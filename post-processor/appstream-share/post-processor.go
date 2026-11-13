//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package appstream

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/aws/aws-sdk-go-v2/service/appstream/types"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"github.com/hashicorp/packer-plugin-sdk/common"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
)

// BuilderId is the unique identifier for the appstream-share post-processor.
const BuilderId = "appstream-share"

// appstreamAPI is the subset of the AppStream client used by this post-processor.
type appstreamAPI interface {
	CopyImage(ctx context.Context, params *appstream.CopyImageInput, optFns ...func(*appstream.Options)) (*appstream.CopyImageOutput, error)
	DescribeImages(ctx context.Context, params *appstream.DescribeImagesInput, optFns ...func(*appstream.Options)) (*appstream.DescribeImagesOutput, error)
	UpdateImagePermissions(ctx context.Context, params *appstream.UpdateImagePermissionsInput, optFns ...func(*appstream.Options)) (*appstream.UpdateImagePermissionsOutput, error)
}

var _ appstreamAPI = (*appstream.Client)(nil)

// Config holds the configuration for the appstream-share post-processor.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	ImageName          string   `mapstructure:"image_name"`
	AccountIDs         []string `mapstructure:"account_ids"`
	DestinationRegions []string `mapstructure:"destination_regions"`
	AllowImageBuilder  bool     `mapstructure:"allow_image_builder"`
	NoAllowFleet       bool     `mapstructure:"no_allow_fleet"`
	Timeout            string   `mapstructure:"timeout"`

	ctx interpolate.Context
}

// PostProcessor implements the packer.PostProcessor interface for sharing
// AppStream images across accounts and regions.
type PostProcessor struct {
	config          Config
	appstreamClient func(aws.Config) appstreamAPI // nil → appstream.NewFromConfig
}

var _ packer.PostProcessor = new(PostProcessor)

// ConfigSpec returns the HCL object spec for the appstream-share post-processor configuration.
func (p *PostProcessor) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

// Configure decodes and validates the post-processor configuration.
func (p *PostProcessor) Configure(raws ...any) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	if err != nil {
		return err
	}

	var errs *packer.MultiError
	errs = packer.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)

	if p.config.ImageName == "" {
		errs = packer.MultiErrorAppend(errs, fmt.Errorf("image_name is required"))
	}
	if len(p.config.AccountIDs) == 0 {
		errs = packer.MultiErrorAppend(errs, fmt.Errorf("account_ids is required"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

// PostProcess shares the AppStream image with the configured accounts and
// optionally copies it to additional regions.
func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	ui.Say("Sharing AppStream image...")

	cfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, true, false, err
	}

	svc := appstreamAPI(appstream.NewFromConfig(*cfg))
	if p.appstreamClient != nil {
		svc = p.appstreamClient(*cfg)
	}

	// Parse timeout
	timeout := 30 * time.Minute
	if p.config.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(p.config.Timeout)
		if err != nil {
			return artifact, true, false, fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	ui.Say(fmt.Sprintf("Waiting for image %s to be available (timeout: %s)...", p.config.ImageName, timeout))

	// Wait for image to be available in source region
	err = p.waitForImage(ctx, svc, p.config.ImageName, timeout)
	if err != nil {
		return artifact, true, false, err
	}

	// Share in the source region
	if len(p.config.AccountIDs) > 0 {
		err = p.shareImage(ctx, svc, p.config.ImageName, p.config.AccountIDs)
		if err != nil {
			return artifact, true, false, err
		}
		ui.Say("Image shared successfully in source region!")
	}

	// Process destination regions
	if len(p.config.DestinationRegions) > 0 {
		for _, destRegion := range p.config.DestinationRegions {
			ui.Say(fmt.Sprintf("Copying image %s to %s...", p.config.ImageName, destRegion))

			// Copy image
			_, err = svc.CopyImage(ctx, &appstream.CopyImageInput{
				SourceImageName:      &p.config.ImageName,
				DestinationImageName: &p.config.ImageName,
				DestinationRegion:    &destRegion,
			})
			if err != nil {
				return artifact, true, false, fmt.Errorf("error copying image to %s: %w", destRegion, err)
			}

			// Create a client for the destination region
			destCfg := helpers.RegionalConfig(cfg, destRegion)
			destSvc := appstreamAPI(appstream.NewFromConfig(destCfg))
			if p.appstreamClient != nil {
				destSvc = p.appstreamClient(destCfg)
			}

			// Wait for image in destination region
			ui.Say(fmt.Sprintf("Waiting for image %s to be available in %s...", p.config.ImageName, destRegion))
			err = p.waitForImage(ctx, destSvc, p.config.ImageName, timeout)
			if err != nil {
				return artifact, true, false, fmt.Errorf("error waiting for image in %s: %w", destRegion, err)
			}

			// Share in destination region
			if len(p.config.AccountIDs) > 0 {
				err = p.shareImage(ctx, destSvc, p.config.ImageName, p.config.AccountIDs)
				if err != nil {
					return artifact, true, false, fmt.Errorf("error sharing image in %s: %w", destRegion, err)
				}
				ui.Say(fmt.Sprintf("Image shared successfully in %s!", destRegion))
			}
		}
	}

	return artifact, true, false, nil
}

func (p *PostProcessor) shareImage(ctx context.Context, svc appstreamAPI, imageName string, accountIDs []string) error {
	for _, accountID := range accountIDs {
		_, err := svc.UpdateImagePermissions(ctx, &appstream.UpdateImagePermissionsInput{
			Name:            &imageName,
			SharedAccountId: &accountID,
			ImagePermissions: &types.ImagePermissions{
				AllowFleet:        aws.Bool(!p.config.NoAllowFleet),
				AllowImageBuilder: aws.Bool(p.config.AllowImageBuilder),
			},
		})
		if err != nil {
			return fmt.Errorf("error sharing image with account %s: %w", accountID, err)
		}
	}
	return nil
}

func (p *PostProcessor) waitForImage(ctx context.Context, svc appstreamAPI, imageName string, timeout time.Duration) error {
	waitStart := time.Now()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		if time.Since(waitStart) > timeout {
			return fmt.Errorf("timeout waiting for image %s to be available", imageName)
		}

		resp, err := svc.DescribeImages(ctx, &appstream.DescribeImagesInput{
			Names: []string{imageName},
		})
		if err != nil {
			return fmt.Errorf("error describing image: %w", err)
		}

		if len(resp.Images) == 0 {
			// Image might not be created yet
		} else {
			image := resp.Images[0]
			if image.State == types.ImageStateAvailable {
				return nil
			}
			if image.State == types.ImageStateFailed {
				msg := "unknown reason"
				if image.StateChangeReason != nil && image.StateChangeReason.Message != nil {
					msg = *image.StateChangeReason.Message
				}
				return fmt.Errorf("image creation failed: %s", msg)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}
