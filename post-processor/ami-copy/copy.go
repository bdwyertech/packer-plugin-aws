package ami_copy

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/sourcegraph/conc/pool"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
	"github.com/hashicorp/packer-plugin-amazon/builder/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

// AmiManifest holds the data about the resulting copied image
type AmiManifest struct {
	AccountID string `json:"account_id"`
	Region    string `json:"region"`
	ImageID   string `json:"image_id"`
}

// copyOperation holds data and methods related to copying an image.
type copyOperation struct {
	ctx             context.Context
	client          *ec2.Client
	sourceImage     *types.Image
	sourceRegion    string
	sourceImageID   string
	copiedImageID   string
	ensureAvailable bool
	tagsOnly        bool
	tags            map[string]string
	encrypted       bool
	kmsKeyID        string
	targetAccountID string
}

// execute performs the EC2 copy and tags the result.
func (c *copyOperation) execute(ui packer.Ui) error {
	var name, description string
	if c.sourceImage.Name != nil {
		name = *c.sourceImage.Name
	}
	if c.sourceImage.Description != nil {
		description = *c.sourceImage.Description
	}

	if !c.tagsOnly {
		// Perform the copy
		input := &ec2.CopyImageInput{
			Name:          aws.String(name),
			Description:   aws.String(description),
			SourceImageId: aws.String(c.sourceImageID),
			SourceRegion:  aws.String(c.sourceRegion),
			Encrypted:     aws.Bool(c.encrypted),
		}

		if c.kmsKeyID != "" {
			input.KmsKeyId = aws.String(c.kmsKeyID)
		}

		output, err := c.client.CopyImage(c.ctx, input)
		if err != nil {
			return err
		}
		c.copiedImageID = *output.ImageId
	} else {
		ui.Say(fmt.Sprintf("Only copying tags in %s as tags_only=true", c.targetAccountID))
		c.copiedImageID = c.sourceImageID
	}

	// Tag the copied image
	if err := c.tagImage(ui); err != nil {
		return err
	}

	// Wait for image to be available if requested
	if c.ensureAvailable {
		if err := c.waitForAvailable(ui); err != nil {
			return err
		}
	}

	return nil
}

// tagImage copies tags from the source image to the target.
func (c *copyOperation) tagImage(ui packer.Ui) error {
	tags := make([]types.Tag, 0, len(c.sourceImage.Tags)+len(c.tags))

	// Copy source tags
	for _, tag := range c.sourceImage.Tags {
		tags = append(tags, types.Tag{
			Key:   tag.Key,
			Value: tag.Value,
		})
	}

	// Add additional tags
	for k, v := range c.tags {
		tags = append(tags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	if len(tags) == 0 {
		return nil
	}

	ui.Say(fmt.Sprintf("Adding tags %v", tags))

	// Retry creating tags for about 2.5 minutes
	return retry.Config{
		Tries: 11,
		ShouldRetry: func(err error) bool {
			return awserrors.Matches(err, "UnauthorizedOperation", "")
		},
		RetryDelay: (&retry.Backoff{InitialBackoff: 200 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}).Linear,
	}.Run(c.ctx, func(ctx context.Context) error {
		_, err := c.client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{c.copiedImageID},
			Tags:      tags,
		})

		if awserrors.Matches(err, "InvalidAMIID.NotFound", "") ||
			awserrors.Matches(err, "InvalidSnapshot.NotFound", "") {
			return nil
		}

		return err
	})
}

// waitForAvailable waits for the copied image to become available.
func (c *copyOperation) waitForAvailable(ui packer.Ui) error {
	ui.Say("Going to wait for image to be in available state")

	for i := 1; i <= 30; i++ {
		image, err := helpers.LocateSingleAMI(c.ctx, c.copiedImageID, c.client)
		if err != nil && image == nil {
			return err
		}

		switch image.State {
		case types.ImageStateAvailable:
			return nil
		case types.ImageStateFailed:
			return fmt.Errorf("AMI copy failed: image %s transitioned to failed state on account %s", *image.ImageId, c.targetAccountID)
		}

		ui.Say(fmt.Sprintf("Waiting one minute (%d/30) for AMI to become available, current state: %s for image %s on account %s",
			i, image.State, *image.ImageId, c.targetAccountID))
		time.Sleep(time.Duration(1) * time.Minute)
	}

	return fmt.Errorf("Timed out waiting for image %s to copy to account %s", c.copiedImageID, c.targetAccountID)
}

// executeCopies runs all copy operations concurrently.
func (p PostProcessor) executeCopies(copies []*copyOperation, ui packer.Ui) (errs packer.MultiError) {
	copyCount := len(copies)
	amiManifests := make(chan *AmiManifest, copyCount)

	concurrencyCount := p.config.CopyConcurrency
	if concurrencyCount == 0 { // Unlimited
		concurrencyCount = copyCount
	}

	pool := pool.New().WithMaxGoroutines(concurrencyCount)
	for _, c := range copies {
		copy := c // capture loop variable
		pool.Go(func() {
			ui.Say(
				fmt.Sprintf(
					"[%s] Copying %s to account %s (encrypted: %t)",
					copy.sourceRegion,
					copy.sourceImageID,
					copy.targetAccountID,
					copy.encrypted,
				),
			)

			if err := copy.execute(ui); err != nil {
				ui.Error(err.Error())
				packer.MultiErrorAppend(&errs, err)
				return
			}

			manifest := &AmiManifest{
				AccountID: copy.targetAccountID,
				Region:    copy.sourceRegion,
				ImageID:   copy.copiedImageID,
			}
			amiManifests <- manifest

			ui.Say(
				fmt.Sprintf(
					"[%s] Finished copying %s to %s (copied id: %s)",
					copy.sourceRegion,
					copy.sourceImageID,
					copy.targetAccountID,
					copy.copiedImageID,
				),
			)
		})
	}
	pool.Wait()

	if p.config.ManifestOutput != "" {
		manifests := []*AmiManifest{}
	LOOP:
		for {
			select {
			case m := <-amiManifests:
				manifests = append(manifests, m)
			default:
				break LOOP
			}
		}
		err := writeManifests(p.config.ManifestOutput, manifests)
		if err != nil {
			ui.Say(fmt.Sprintf("Unable to write out manifest to %s: %s", p.config.ManifestOutput, err))
		}
	}
	close(amiManifests)

	return
}
