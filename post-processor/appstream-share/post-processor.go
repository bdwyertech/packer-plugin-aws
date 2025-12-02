//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package appstream

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/aws/aws-sdk-go-v2/service/appstream/types"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"github.com/hashicorp/packer-plugin-sdk/common"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

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

type PostProcessor struct {
	config Config
}

func (p *PostProcessor) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

func (p *PostProcessor) Configure(raws ...any) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         "appstream-share",
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)
	if err != nil {
		return err
	}

	if p.config.ImageName == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("image_name is required"))
	}
	if len(p.config.AccountIDs) == 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("account_ids is required"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	ui.Say("Sharing AppStream image...")

	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, false, false, fmt.Errorf("unable to load SDK config, %v", err)
	}
	if p.config.RawRegion != "" {
		cfg.Region = p.config.RawRegion
	}

	svc := appstream.NewFromConfig(cfg)

	// Parse timeout
	timeout := 30 * time.Minute
	if p.config.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(p.config.Timeout)
		if err != nil {
			return nil, false, false, fmt.Errorf("invalid timeout format: %v", err)
		}
	}

	ui.Say(fmt.Sprintf("Waiting for image %s to be available (timeout: %s)...", p.config.ImageName, timeout))

	// Wait for image to be available in source region
	err = p.waitForImage(ctx, svc, p.config.ImageName, timeout)
	if err != nil {
		return nil, false, false, err
	}

	// Share in the source region
	if len(p.config.AccountIDs) > 0 {
		err = p.shareImage(ctx, svc, p.config.ImageName, p.config.AccountIDs)
		if err != nil {
			return nil, false, false, err
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
				return nil, false, false, fmt.Errorf("error copying image to %s: %v", destRegion, err)
			}

			// Create a client for the destination region
			destCfg := cfg.Copy()
			destCfg.Region = destRegion
			destSvc := appstream.NewFromConfig(destCfg)

			// Wait for image in destination region
			ui.Say(fmt.Sprintf("Waiting for image %s to be available in %s...", p.config.ImageName, destRegion))
			err = p.waitForImage(ctx, destSvc, p.config.ImageName, timeout)
			if err != nil {
				return nil, false, false, fmt.Errorf("error waiting for image in %s: %v", destRegion, err)
			}

			// Share in destination region
			if len(p.config.AccountIDs) > 0 {
				err = p.shareImage(ctx, destSvc, p.config.ImageName, p.config.AccountIDs)
				if err != nil {
					return nil, false, false, fmt.Errorf("error sharing image in %s: %v", destRegion, err)
				}
				ui.Say(fmt.Sprintf("Image shared successfully in %s!", destRegion))
			}
		}
	}

	return artifact, true, false, nil
}

func (p *PostProcessor) shareImage(ctx context.Context, svc *appstream.Client, imageName string, accountIDs []string) error {
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
			return fmt.Errorf("error sharing image with account %s: %v", accountID, err)
		}
	}
	return nil
}

func (p *PostProcessor) waitForImage(ctx context.Context, svc *appstream.Client, imageName string, timeout time.Duration) error {
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
			return fmt.Errorf("error describing image: %v", err)
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
