//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Target

package ami_copy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2/hcldec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/hashicorp/packer-plugin-amazon/builder/chroot"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebssurrogate"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebsvolume"
	"github.com/hashicorp/packer-plugin-amazon/builder/instance"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	pkrconfig "github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"github.com/bdwyertech/packer-plugin-aws/helpers"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

// BuilderId is the ID of this post processor.
// nolint: golint
const BuilderId = "packer.post-processor.ami-copy"

// Config is the post-processor configuration with interpolation supported.
// See https://www.packer.io/docs/builders/amazon.html for details.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	awscommon.AMIConfig    `mapstructure:",squash"`

	// Variables specific to this post-processor
	RoleName        string `mapstructure:"role_name"`
	CopyConcurrency int    `mapstructure:"copy_concurrency"`
	EnsureAvailable bool   `mapstructure:"ensure_available"`
	KeepArtifact    string `mapstructure:"keep_artifact"`
	ManifestOutput  string `mapstructure:"manifest_output"`
	TagsOnly        bool   `mapstructure:"tags_only"`

	Targets []Target `mapstructure:"targets"`

	ctx interpolate.Context
}

type Target struct {
	awscommon.AccessConfig `mapstructure:",squash"`
	Name                   string `mapstructure:"name"`
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

	if err := pkrconfig.Decode(&p.config, &pkrconfig.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &p.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...); err != nil {
		return err
	}

	if len(p.config.AMIUsers) == 0 && len(p.config.Targets) == 0 {
		return errors.New("ami_users or targets must be set")
	}

	if len(p.config.KeepArtifact) == 0 {
		p.config.KeepArtifact = "true"
	}

	return nil
}

// PostProcess will copy the source AMI to each of the target accounts as
// designated by the mandatory `ami_users` variable. It will optionally
// encrypt the copied AMIs (`encrypt_boot`) with `kms_key_id` if set, or the
// default EBS KMS key if unset. Tags will be copied with the image.
//
// Copies are executed concurrently. This concurrency is unlimited unless
// controller by `copy_concurrency`.
func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {

	keepArtifactBool, err := strconv.ParseBool(p.config.KeepArtifact)
	if err != nil {
		return artifact, keepArtifactBool, false, err
	}

	// Ensure we're being called from a supported builder
	switch artifact.BuilderId() {
	case ebs.BuilderId,
		ebssurrogate.BuilderId,
		ebsvolume.BuilderId,
		chroot.BuilderId,
		instance.BuilderId:
		break
	default:
		return artifact, keepArtifactBool, false,
			fmt.Errorf("Unexpected artifact type: %s\nCan only export from Amazon builders",
				artifact.BuilderId())
	}

	// Get AWS config
	awsCfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, keepArtifactBool, false, err
	}

	// Parse AMIs from artifact
	amis := amisFromArtifactID(artifact.Id())

	// Build list of copy operations
	var copies []*copyOperation
	for _, ami := range amis {
		// Get source image
		cfg := awsCfg.Copy()
		cfg.Region = ami.region
		client := ec2.NewFromConfig(cfg)

		source, err := helpers.LocateSingleAMI(ctx, ami.id, client)
		if err != nil || source == nil {
			return artifact, keepArtifactBool, false, err
		}

		ui.Sayf("Source Tags: %v", source.Tags)

		// Create copy operations for each target
		for _, tgt := range p.config.Targets {
			targetCfg, err := tgt.GetAWSConfig(ctx)
			if err != nil {
				ui.Error(err.Error())
				continue
			}
			targetCfg.Region = ami.region
			targetClient := ec2.NewFromConfig(*targetCfg)

			// Attempt to resolve the target account ID via STS on the target credentials.
			stsClient := sts.NewFromConfig(*targetCfg)
			targetId, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err != nil {
				ui.Error(fmt.Sprintf("unable to resolve target account ID for target (skipping): %v", err))
				continue
			}
			ui.Sayf("Resolved target ARN: %s", *targetId.Arn)

			debugstsClient := sts.NewFromConfig(cfg)
			debugclientId, err := debugstsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err != nil {
				ui.Error(fmt.Sprintf("unable to resolve target account ID for target (skipping): %v", err))
				continue
			}
			ui.Sayf("Resolved source ARN: %s", *debugclientId.Arn)

			// Ensure that the source AMI is shared with the resolved target account
			if err = helpers.EnsureImageSharedWith(ctx, source, targetId.Account, client); err != nil {
				ui.Error(fmt.Sprintf("unable to update AMI launch permissions for account %s: %v", *targetId.Account, err))
				continue
			}
			copy := &copyOperation{
				ctx:             ctx,
				client:          targetClient,
				sourceImage:     source,
				sourceRegion:    ami.region,
				sourceImageID:   ami.id,
				ensureAvailable: p.config.EnsureAvailable,
				tagsOnly:        p.config.TagsOnly,
				tags:            p.config.AMITags,
				encrypted:       p.config.AMIEncryptBootVolume.True(),
				kmsKeyID:        p.config.AMIKmsKeyId,
				targetAccountID: *targetId.Account,
			}
			copies = append(copies, copy)
		}

		// Create copy operations for each user (via role assumption)
		for _, user := range p.config.AMIUsers {
			var targetClient *ec2.Client
			if p.config.RoleName != "" {
				role := fmt.Sprintf("arn:aws:iam::%s:role/%s", user, p.config.RoleName)
				stsClient := sts.NewFromConfig(*awsCfg)
				creds := stscreds.NewAssumeRoleProvider(stsClient, role)

				targetCfg, err := config.LoadDefaultConfig(ctx,
					config.WithRegion(ami.region),
					config.WithCredentialsProvider(aws.NewCredentialsCache(creds)),
				)
				if err != nil {
					ui.Error(err.Error())
					continue
				}
				targetClient = ec2.NewFromConfig(targetCfg)
			} else {
				cfg := awsCfg.Copy()
				cfg.Region = ami.region
				targetClient = ec2.NewFromConfig(cfg)
			}

			copy := &copyOperation{
				ctx:             ctx,
				client:          targetClient,
				sourceImage:     source,
				sourceRegion:    ami.region,
				sourceImageID:   ami.id,
				ensureAvailable: p.config.EnsureAvailable,
				tagsOnly:        p.config.TagsOnly,
				tags:            p.config.AMITags,
				encrypted:       p.config.AMIEncryptBootVolume.True(),
				kmsKeyID:        p.config.AMIKmsKeyId,
				targetAccountID: user,
			}
			copies = append(copies, copy)
		}
	}

	// Execute copies
	copyErrs := p.executeCopies(copies, ui)
	if copyErrCount := len(copyErrs.Errors); copyErrCount > 0 {
		return artifact, true, false, fmt.Errorf(
			"%d/%d AMI copies failed, manual reconciliation may be required", copyErrCount, len(copies))
	}

	return artifact, keepArtifactBool, false, nil
}

// ami encapsulates simplistic details about an AMI.
type ami struct {
	id     string
	region string
}

// amisFromArtifactID returns an AMI slice from a Packer artifact id.
func amisFromArtifactID(artifactID string) (amis []*ami) {
	for _, amiStr := range strings.Split(artifactID, ",") {
		pair := strings.SplitN(amiStr, ":", 2)
		amis = append(amis, &ami{region: pair[0], id: pair[1]})
	}
	return amis
}

func writeManifests(output string, manifests []*AmiManifest) error {
	rawManifest, err := json.Marshal(manifests)
	if err != nil {
		return err
	}
	return os.WriteFile(output, rawManifest, 0644)
}
