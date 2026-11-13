//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package ami_watermark

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
	"github.com/hashicorp/hcl/v2/hcldec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// BuilderId is the ID of this post processor.
// nolint: golint
const BuilderId = "packer.post-processor.ami-watermark"

// Config is the post-processor configuration with interpolation supported.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// A list of watermark names to attach to each AMI. Minimum 1, maximum 5.
	// Each name must be 3–128 characters and contain only alphanumeric characters,
	// parentheses, square brackets, spaces, periods, slashes, dashes, single quotes,
	// at-signs, or underscores.
	WatermarkNames []string `mapstructure:"watermark_names"`

	ctx interpolate.Context
}

// stsAPI is the subset of the STS client used by this post-processor.
type stsAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// watermarkEC2API is the subset of the EC2 client used by this post-processor.
type watermarkEC2API interface {
	AttachImageWatermark(ctx context.Context, params *ec2.AttachImageWatermarkInput, optFns ...func(*ec2.Options)) (*ec2.AttachImageWatermarkOutput, error)
}

var _ stsAPI = (*sts.Client)(nil)
var _ watermarkEC2API = (*ec2.Client)(nil)

// PostProcessor implements Packer's PostProcessor interface.
type PostProcessor struct {
	config    Config
	stsClient func(aws.Config) stsAPI          // nil → sts.NewFromConfig
	ec2Client func(aws.Config) watermarkEC2API // nil → ec2.NewFromConfig
}

var _ packer.PostProcessor = new(PostProcessor)

// ConfigSpec returns the HCL object spec for the ami-watermark post-processor configuration.
func (p *PostProcessor) ConfigSpec() hcldec.ObjectSpec {
	return p.config.FlatMapstructure().HCL2Spec()
}

// Configure interpolates and validates requisite vars for the PostProcessor.
func (p *PostProcessor) Configure(raws ...any) error {
	p.config.ctx.Funcs = awscommon.TemplateFuncs

	if err := config.Decode(&p.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...); err != nil {
		return err
	}

	var errs *packer.MultiError

	// Validate watermark_names count
	if len(p.config.WatermarkNames) == 0 {
		errs = packer.MultiErrorAppend(errs, errors.New("watermark_names must contain at least one entry"))
	}
	if len(p.config.WatermarkNames) > 5 {
		errs = packer.MultiErrorAppend(errs, errors.New("watermark_names must not contain more than five entries"))
	}

	// Validate each watermark name
	for _, name := range p.config.WatermarkNames {
		if err := validateWatermarkName(name); err != nil {
			errs = packer.MultiErrorAppend(errs, err)
		}
	}

	// Validate AWS access config
	errs = packer.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)

	if errs != nil && len(errs.Errors) != 0 {
		return errs
	}
	return nil
}

// PostProcess attaches watermarks to AMIs in the artifact.
func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	// Builder compatibility check
	if !helpers.IsAMIBuilder(artifact.BuilderId()) {
		return artifact, true, false, fmt.Errorf(
			"unexpected artifact type: %s\nCan only attach watermarks from Amazon builders (ebs, ebssurrogate, ebsvolume, chroot, instance)",
			artifact.BuilderId(),
		)
	}

	// Get AWS base config
	awsCfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, true, false, err
	}

	// Parse AMIs from artifact
	if artifact.Id() == "" {
		return artifact, false, false, nil
	}
	amis, err := helpers.ParseArtifactID(artifact.Id())
	if err != nil {
		return artifact, true, false, err
	}

	// Attach watermarks
	for _, ref := range amis {
		cfg := helpers.RegionalConfig(awsCfg, ref.Region)

		var client watermarkEC2API = ec2.NewFromConfig(cfg)
		if p.ec2Client != nil {
			client = p.ec2Client(cfg)
		}

		var stsSvc stsAPI = sts.NewFromConfig(cfg)
		if p.stsClient != nil {
			stsSvc = p.stsClient(cfg)
		}

		whoami, err := stsSvc.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return artifact, true, false, fmt.Errorf("GetCallerIdentity: %w", err)
		}
		ui.Say(*whoami.Arn)

		id := ref.ID
		for _, watermarkName := range p.config.WatermarkNames {
			ui.Sayf("Attaching watermark %q to %s in %s", watermarkName, ref.ID, ref.Region)
			resp, err := client.AttachImageWatermark(ctx, &ec2.AttachImageWatermarkInput{
				ImageId:       &id,
				WatermarkName: &watermarkName,
			})
			if err != nil {
				ui.Error(err.Error())
				return artifact, true, false, fmt.Errorf(
					"error attaching watermark %q to AMI %s in %s: %w",
					watermarkName, ref.ID, ref.Region, err,
				)
			}

			ui.Sayf("Watermark attached: key=%s", *resp.WatermarkKey)
		}
	}

	// Pass through original artifact
	return artifact, false, false, nil
}

// watermarkNameRegex defines the allowed characters for a watermark name.
var watermarkNameRegex = regexp.MustCompile(`^[a-zA-Z0-9()\[\] .\/\-'@_]+$`)

// validateWatermarkName checks that a watermark name meets length and character constraints.
func validateWatermarkName(name string) error {
	if len(name) < 3 {
		return fmt.Errorf("watermark name %q must be at least 3 characters", name)
	}
	if len(name) > 128 {
		return fmt.Errorf("watermark name %q must not exceed 128 characters", name)
	}
	if !watermarkNameRegex.MatchString(name) {
		return fmt.Errorf(
			"watermark name %q contains invalid characters; allowed: alphanumeric, parentheses, square brackets, spaces, periods, slashes, dashes, single quotes, at-signs, underscores",
			name,
		)
	}
	return nil
}
