//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package ami_delete

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2/hcldec"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
)

// BuilderId is the ID of this post processor.
// nolint: golint
const BuilderId = "packer.post-processor.ami-delete"

// Config is the post-processor configuration with interpolation supported.
// See https://www.packer.io/docs/builders/amazon.html for details.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	awscommon.AMIConfig    `mapstructure:",squash"`

	ctx interpolate.Context
}

// PostProcessor implements Packer's PostProcessor interface.
type PostProcessor struct {
	config Config

	// deleteAMI is a test hook. When nil, defaults to helpers.DeleteAMIAndSnapshots.
	deleteAMI func(ctx context.Context, client helpers.DeleteAMIAPI, image *types.Image, onSnapshot func(string)) error
}

var _ packer.PostProcessor = new(PostProcessor)

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
	errs = packer.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)
	if errs != nil && len(errs.Errors) != 0 {
		return errs
	}

	return nil
}

// PostProcess will delete the AMI
func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	// Ensure we're being called from a supported builder
	if !helpers.IsAMIBuilder(artifact.BuilderId()) {
		return artifact, true, false, fmt.Errorf(
			"unexpected artifact type: %s\nCan only export from Amazon builders",
			artifact.BuilderId(),
		)
	}

	awsCfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, true, false, err
	}

	deletedAmis := make(map[string][]string)
	amis, err := helpers.ParseArtifactID(artifact.Id())
	if err != nil {
		return artifact, true, false, err
	}
	for _, ref := range amis {
		cfg := helpers.RegionalConfig(awsCfg, ref.Region)
		client := ec2.NewFromConfig(cfg)
		img, err := helpers.LocateSingleAMI(ctx, ref.ID, client)
		if err != nil || img == nil {
			return artifact, true, false, err
		}
		ui.Sayf("Deregistering %s", *img.ImageId)
		deleteFn := p.deleteAMI
		if deleteFn == nil {
			deleteFn = helpers.DeleteAMIAndSnapshots
		}
		if err := deleteFn(ctx, client, img, func(snapID string) {
			ui.Sayf("Deleting snapshot %s...", snapID)
		}); err != nil {
			return artifact, true, false, err
		}
		deletedAmis[ref.Region] = append(deletedAmis[ref.Region], ref.ID)
	}

	return &Artifact{
		DeletedAmis:    deletedAmis,
		BuilderIdValue: BuilderId,
		StateData:      make(map[string]any),
	}, true, true, nil
}
