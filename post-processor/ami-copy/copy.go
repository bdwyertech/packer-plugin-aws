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
	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

// copyAPI is the subset of the EC2 client used by copyOperation and AmiManifest.
type copyAPI interface {
	CopyImage(ctx context.Context, params *ec2.CopyImageInput, optFns ...func(*ec2.Options)) (*ec2.CopyImageOutput, error)
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

// Compile-time check: *ec2.Client satisfies copyAPI.
var _ copyAPI = (*ec2.Client)(nil)

// AmiManifest holds the data about the resulting copied image
type AmiManifest struct {
	AccountID string  `json:"account_id"`
	Region    string  `json:"region"`
	ImageID   string  `json:"image_id"`
	client    copyAPI `json:"-"`
}

// copyOperation holds data and methods related to copying an image.
type copyOperation struct {
	ctx                 context.Context
	client              copyAPI
	region              string
	sourceImage         *types.Image
	sourceRegion        string
	targetRegion        string
	sourceImageID       string
	copiedImageID       string
	ensureAvailable     bool
	tagsOnly            bool
	tags                map[string]string
	encrypted           bool
	kmsKeyID            string
	targetAccountID     string
	copyDurationMinutes *int64
	timeoutMinutes      int
	retryDelay          func(uint) time.Duration
	waiterOptions       []func(*ec2.ImageAvailableWaiterOptions)
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
			Name:                                  aws.String(name),
			Description:                           aws.String(description),
			SourceImageId:                         aws.String(c.sourceImageID),
			SourceRegion:                          aws.String(c.sourceRegion),
			Encrypted:                             aws.Bool(c.encrypted),
			CopyImageTags:                         aws.Bool(true),
			SnapshotCopyCompletionDurationMinutes: c.copyDurationMinutes,
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
	tags := helpers.ToEC2Tags(c.tags)
	if len(tags) == 0 {
		return nil
	}

	ui.Say(fmt.Sprintf("Adding tags [%s]", c.tags))

	mapErr := func(err error) error {
		if awserrors.Matches(err, "InvalidAMIID.NotFound", "") ||
			awserrors.Matches(err, "InvalidSnapshot.NotFound", "") {
			return nil
		}
		return err
	}

	return helpers.CreateTagsWithRetry(c.ctx, c.client, []string{c.copiedImageID}, tags, c.retryDelay, mapErr)
}

// waitForAvailable waits for the copied image to become available
// using the SDK v2 ec2.NewImageAvailableWaiter.
func (c *copyOperation) waitForAvailable(ui packer.Ui) error {
	ui.Say("Waiting for image to be in available state")

	maxWait := time.Duration(c.timeoutMinutes) * time.Minute
	if maxWait == 0 {
		maxWait = 30 * time.Minute
	}

	waiter := ec2.NewImageAvailableWaiter(c.client, c.waiterOptions...)
	err := waiter.Wait(c.ctx, &ec2.DescribeImagesInput{
		ImageIds: []string{c.copiedImageID},
	}, maxWait)
	if err != nil {
		return fmt.Errorf("waiting for image %s: %w", c.copiedImageID, err)
	}
	return nil
}

func (ami *AmiManifest) delete(ctx context.Context) error {
	image, err := helpers.LocateSingleAMI(ctx, ami.ImageID, ami.client)
	if err != nil {
		return err
	}
	return helpers.DeleteAMIAndSnapshots(ctx, ami.client, image, nil)
}

// executeCopies runs all copy operations concurrently.
func (p *PostProcessor) executeCopies(copies []*copyOperation, ui packer.Ui) ([]*AmiManifest, packer.MultiError) {
	copyCount := len(copies)
	amiManifests := make(chan *AmiManifest, copyCount)
	var errs packer.MultiError

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
					"[%s:%s] Copying to %s:%s (encrypted: %t)",
					copy.sourceRegion,
					copy.sourceImageID,
					copy.targetRegion,
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
				Region:    copy.targetRegion,
				ImageID:   copy.copiedImageID,
			}
			amiManifests <- manifest

			ui.Say(
				fmt.Sprintf(
					"[%s:%s] Finished copying to %s:%s (copied id: %s)",
					copy.sourceRegion,
					copy.sourceImageID,
					copy.targetRegion,
					copy.targetAccountID,
					copy.copiedImageID,
				),
			)
		})
	}
	pool.Wait()

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
	close(amiManifests)

	if p.config.ManifestOutput != "" {
		err := writeManifests(p.config.ManifestOutput, manifests)
		if err != nil {
			ui.Say(fmt.Sprintf("Unable to write out manifest to %s: %s", p.config.ManifestOutput, err))
		}
	}

	return manifests, errs
}
