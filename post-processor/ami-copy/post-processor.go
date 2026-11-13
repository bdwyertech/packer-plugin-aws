//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Target

package ami_copy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/hcl/v2/hcldec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

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
	RoleName string `mapstructure:"role_name"`
	// Limit the number of copies executed in parallel (default: unlimited).
	CopyConcurrency int `mapstructure:"copy_concurrency"`
	// Specify a completion duration, in 15 minute increments, to initiate a
	// time-based AMI copy. The specified completion duration applies to each of the
	// snapshots associated with the AMI. Each snapshot associated with the AMI will be
	// completed within the specified completion duration, with copy throughput
	// automatically adjusted for each snapshot based on its size to meet the timing
	// target.
	//
	// If you do not specify a value, the AMI copy operation is completed on a
	// best-effort basis.
	//
	// If you initiate two time-based copy operations for the same snapshot or AMI,
	// the second copy operation's completion duration starts only after the first
	// copy operation completes.
	//
	// This parameter is not supported when copying an AMI to or from a Local Zone, or
	// to an Outpost.
	//
	// For more information, see [Time-based copies for Amazon EBS snapshots and EBS-backed AMIs].
	//
	// [Time-based copies for Amazon EBS snapshots and EBS-backed AMIs]: https://docs.aws.amazon.com/ebs/latest/userguide/time-based-copies.html
	CopyDurationMinutes *int64 `mapstructure:"copy_duration_minutes"`
	// Maximum time, in minutes, to wait for the copied AMI to become available. (default: 30)
	CopyTimeoutMinutes int `mapstructure:"copy_timeout_minutes"`
	// Wait for the copied AMI to become available before returning.
	EnsureAvailable bool `mapstructure:"ensure_available"`
	// remove the original generated AMI after copy
	KeepArtifact pkrconfig.Trilean `mapstructure:"keep_artifact"`
	// the name of the file we output AMI IDs to, in JSON format (default: no manifest file is written)
	ManifestOutput string `mapstructure:"manifest_output"`

	// if set to true, then the AMI won't be copied, but the tags will be duplicated on the shared AMI in the destination account.
	TagsOnly bool `mapstructure:"tags_only"`

	Targets []Target `mapstructure:"targets"`

	ctx interpolate.Context
}

type Target struct {
	awscommon.AccessConfig `mapstructure:",squash"`
}

// PostProcessor implements Packer's PostProcessor interface.
type PostProcessor struct {
	config Config
	// locateAMI is a test hook for LocateSingleAMI. nil means helpers.LocateSingleAMI.
	locateAMI func(ctx context.Context, id string, client helpers.DescribeImagesAPI) (*types.Image, error)
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

	if p.config.KeepArtifact == pkrconfig.TriUnset {
		p.config.KeepArtifact = pkrconfig.TriTrue
	}

	var errs *packer.MultiError
	errs = packer.MultiErrorAppend(errs, p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)
	for i := range p.config.Targets {
		errs = packer.MultiErrorAppend(errs, p.config.Targets[i].AccessConfig.Prepare(&p.config.PackerConfig)...)
	}
	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

// PostProcess will copy the source AMI to each of the target accounts as
// designated by the mandatory `ami_users` variable. It will optionally
// encrypt the copied AMIs (`encrypt_boot`) with `kms_key_id` if set, or the
// default EBS KMS key if unset. Tags will be copied with the image.
//
// Copies are executed concurrently. This concurrency is unlimited unless
// controlled by `copy_concurrency`.
func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	keepArtifactBool := p.config.KeepArtifact.True()

	// Ensure we're being called from a supported builder
	if !helpers.IsAMIBuilder(artifact.BuilderId()) {
		return artifact, true, false, fmt.Errorf("unexpected artifact type: %s\nCan only export from Amazon builders", artifact.BuilderId())
	}

	// Get AWS config
	awsCfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, true, false, err
	}

	// Parse AMIs from artifact
	amis, err := helpers.ParseArtifactID(artifact.Id())
	if err != nil {
		return artifact, true, false, fmt.Errorf("parsing artifact ID: %w", err)
	}

	// Build list of copy operations
	var copies []*copyOperation
	for _, ref := range amis {
		// Get source image
		cfg := helpers.RegionalConfig(awsCfg, ref.Region)
		client := ec2.NewFromConfig(cfg)

		locateFn := p.locateAMI
		if locateFn == nil {
			locateFn = helpers.LocateSingleAMI
		}
		source, err := locateFn(ctx, ref.ID, client)
		if err != nil || source == nil {
			return artifact, true, false, err
		}
		// Create copy operations for each target
		for _, tgt := range p.config.Targets {
			targetCfg, err := tgt.GetAWSConfig(ctx)
			if err != nil {
				ui.Error(err.Error())
				continue
			}

			targetClient := ec2.NewFromConfig(*targetCfg)

			// Attempt to resolve the target account ID via STS on the target credentials.
			stsClient := sts.NewFromConfig(*targetCfg)
			targetId, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
			if err != nil {
				ui.Error(fmt.Sprintf("unable to resolve target account ID for target (skipping): %v", err))
				continue
			}

			// Ensure that the source AMI is shared with the resolved target account
			if err = helpers.EnsureImageSharedWith(ctx, source, targetId.Account, client); err != nil {
				ui.Error(fmt.Sprintf("unable to update AMI launch permissions for account %s: %v", *targetId.Account, err))
				continue
			}
			copy := &copyOperation{
				ctx:                 ctx,
				client:              targetClient,
				region:              targetCfg.Region,
				sourceImage:         source,
				sourceRegion:        ref.Region,
				targetRegion:        targetCfg.Region,
				sourceImageID:       ref.ID,
				ensureAvailable:     p.config.EnsureAvailable,
				tagsOnly:            p.config.TagsOnly,
				tags:                p.config.AMITags,
				encrypted:           p.config.AMIEncryptBootVolume.True(),
				kmsKeyID:            p.config.AMIKmsKeyId,
				copyDurationMinutes: p.config.CopyDurationMinutes,
				timeoutMinutes:      p.config.CopyTimeoutMinutes,
				targetAccountID:     *targetId.Account,
			}
			copies = append(copies, copy)
		}

		// Create copy operations for each user (via role assumption)
		for _, user := range p.config.AMIUsers {
			targetCfg := awsCfg.Copy()
			if p.config.RoleName != "" {
				role := fmt.Sprintf("arn:aws:iam::%s:role/%s", user, p.config.RoleName)
				stsClient := sts.NewFromConfig(*awsCfg)
				creds := stscreds.NewAssumeRoleProvider(stsClient, role)
				targetCfg.Credentials = aws.NewCredentialsCache(creds)
			}
			targetClient := ec2.NewFromConfig(targetCfg)

			copy := &copyOperation{
				ctx:                 ctx,
				client:              targetClient,
				region:              targetCfg.Region,
				sourceImage:         source,
				sourceRegion:        ref.Region,
				targetRegion:        targetCfg.Region,
				sourceImageID:       ref.ID,
				ensureAvailable:     p.config.EnsureAvailable,
				tagsOnly:            p.config.TagsOnly,
				tags:                p.config.AMITags,
				encrypted:           p.config.AMIEncryptBootVolume.True(),
				kmsKeyID:            p.config.AMIKmsKeyId,
				copyDurationMinutes: p.config.CopyDurationMinutes,
				timeoutMinutes:      p.config.CopyTimeoutMinutes,
				targetAccountID:     user,
			}
			copies = append(copies, copy)
		}
	}

	// Execute copies
	copiedAmis, copyErrs := p.executeCopies(copies, ui)
	if copyErrCount := len(copyErrs.Errors); copyErrCount > 0 {
		return artifact, true, false, fmt.Errorf(
			"%d/%d AMI copies failed, manual reconciliation may be required", copyErrCount, len(copies))
	}

	return &Artifact{
		CopiedAmis:     copiedAmis,
		BuilderIdValue: BuilderId,
		StateData:      make(map[string]any),
	}, keepArtifactBool, false, nil
}

func writeManifests(output string, manifests []*AmiManifest) error {
	rawManifest, err := json.Marshal(manifests)
	if err != nil {
		return err
	}
	return os.WriteFile(output, rawManifest, 0644)
}
