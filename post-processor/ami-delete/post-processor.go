//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package ami_delete

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/hcldec"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/hashicorp/packer-plugin-amazon/builder/chroot"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebssurrogate"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebsvolume"
	"github.com/hashicorp/packer-plugin-amazon/builder/instance"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
)

// BuilderId is the ID of this post processor.
// nolint: golint
const BuilderId = "packer.post-processor.aws-ami-delete"

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
	switch artifact.BuilderId() {
	case ebs.BuilderId,
		ebssurrogate.BuilderId,
		ebsvolume.BuilderId,
		chroot.BuilderId,
		instance.BuilderId:
		break
	default:
		return artifact, false, false, fmt.Errorf("unexpected artifact type: %s\nCan only export from Amazon builders", artifact.BuilderId())
	}

	awsCfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, false, false, err
	}

	amis := amisFromArtifactID(artifact.Id())
	for _, ami := range amis {
		var img *types.Image
		cfg := awsCfg.Copy()
		cfg.Region = ami.region
		client := ec2.NewFromConfig(cfg)
		if img, err = helpers.LocateSingleAMI(ctx, ami.id, client); err != nil || img == nil {
			return artifact, false, false, err
		}
		ui.Sayf("Deregistering %s", *img.ImageId)
		if _, err = client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
			ImageId: img.ImageId,
		}); err != nil {
			return artifact, false, false, err
		}
		for _, bdm := range img.BlockDeviceMappings {
			if bdm.Ebs != nil && bdm.Ebs.SnapshotId != nil {
				ui.Sayf("Deleting %s", *bdm.Ebs.SnapshotId)
				if _, err = client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: bdm.Ebs.SnapshotId,
				}); err != nil {
					return artifact, false, false, err
				}
			}
		}
	}

	return artifact, true, true, nil
}

// ami encapsulates simplistic details about an AMI.
type ami struct {
	id, region string
}

// amisFromArtifactID returns an AMI slice from a Packer artifact id.
func amisFromArtifactID(artifactID string) (amis []*ami) {
	for amiStr := range strings.SplitSeq(artifactID, ",") {
		pair := strings.SplitN(amiStr, ":", 2)
		amis = append(amis, &ami{region: pair[0], id: pair[1]})
	}
	return amis
}
