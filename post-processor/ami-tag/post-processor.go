//go:generate packer-sdc mapstructure-to-hcl2 -type Config
//go:generate packer-sdc struct-markdown

package ami_tag

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
	ami_copy "github.com/bdwyertech/packer-plugin-aws/post-processor/ami-copy"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

// BuilderId is the ID of this post processor.
// nolint: golint
const BuilderId = "packer.post-processor.ami-tag"

// Config is the post-processor configuration with interpolation supported.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// Tags to apply to each AMI (never to snapshots). Min 1, max 50 entries.
	// Keys: 1-128 chars, must not start with `aws:` (any case).
	// Values: 0-256 chars. Keys prefixed with `ec2:SharedTag/` are
	// passed through unchanged.
	Tags map[string]string `mapstructure:"tags"`

	ctx interpolate.Context
}

// PostProcessor implements Packer's PostProcessor interface.
type PostProcessor struct {
	config Config

	// Test-only seams (unexported, so not decoded from HCL and not in
	// the generated spec). Nil means production defaults.
	newClient  func(cfg aws.Config, region string) tagAPI
	retryDelay func(attempt uint) time.Duration
}

var _ packer.PostProcessor = new(PostProcessor)

// ConfigSpec returns the HCL object spec for the configuration.
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
			Exclude: []string{"tags"},
		},
	}, raws...); err != nil {
		return err
	}

	var errs *packer.MultiError

	for _, e := range validateTags(p.config.Tags) {
		errs = packer.MultiErrorAppend(errs, e)
	}

	errs = packer.MultiErrorAppend(errs,
		p.config.AccessConfig.Prepare(&p.config.PackerConfig)...)

	if errs != nil && len(errs.Errors) != 0 {
		return errs
	}
	return nil
}

// PostProcess applies configured tags to each AMI in the artifact.
func (p *PostProcessor) PostProcess(ctx context.Context, ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, bool, error) {
	// Step 1: Builder compatibility check.
	bid := artifact.BuilderId()
	if !isSupportedBuilder(bid) {
		msg := fmt.Sprintf(
			"unsupported builder %q; supported builders: %v",
			bid, supportedBuilderIDs())
		if bid == ami_copy.BuilderId {
			msg += ". Place ami-tag before ami-copy in the post-processor chain, or use the ami-copy tags option"
		}
		return artifact, true, false, fmt.Errorf("%s", msg)
	}

	// Step 2: Parse artifact ID into (region, ami-id) pairs.
	amis, err := helpers.ParseArtifactID(artifact.Id())
	if err != nil {
		return artifact, true, false, err
	}

	// Step 3: Get the base AWS config.
	awsCfg, err := p.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return artifact, true, false, err
	}

	// Step 4: Get generated data.
	data := normalizeData(artifact.State("generated_data"))

	// Pass 1 — per AMI: render and validate.
	type amiResult struct {
		ami      helpers.AMIRef
		client   tagAPI
		rendered map[string]string
	}
	var results []amiResult
	var allErrors []error
	warnedAMITags := false
	clients := make(map[string]tagAPI)
	for _, a := range amis {
		client, ok := clients[a.Region]
		if !ok {
			cfg := helpers.RegionalConfig(awsCfg, a.Region)
			if p.newClient != nil {
				client = p.newClient(cfg, a.Region)
			} else {
				client = ec2.NewFromConfig(cfg)
			}
			clients[a.Region] = client
		}
		existing, err := existingTags(ctx, client, a.ID)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf(
				"error reading tags for AMI %s in %s: %w", a.ID, a.Region, err))
			continue
		}
		d := maps.Clone(data)
		if _, has := data["AMITags"]; has && !warnedAMITags {
			ui.Say(`warning: generated_data key "AMITags" overridden with actual AMI tags`)
			warnedAMITags = true
		}
		d["AMITags"] = existing
		rendered, errs := interpolateTags(p.config.Tags, p.config.ctx, d)

		// AMITags retry: re-read tags up to 4 times if only
		// AMITags keys are missing (eventual consistency).
		for attempt := uint(0); attempt < 4 && isOnlyAMITagsMissing(errs); attempt++ {
			ui.Say(fmt.Sprintf(
				"AMI %s: waiting for tags to propagate, retry %d/4",
				a.ID, attempt+1))
			if p.retryDelay != nil {
				time.Sleep(p.retryDelay(attempt))
			} else {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
			}
			re, reErr := existingTags(ctx, client, a.ID)
			if reErr != nil {
				errs = []error{fmt.Errorf(
					"error reading tags for AMI %s in %s: %w",
					a.ID, a.Region, reErr)}
				break
			}
			existing = re
			d = maps.Clone(data)
			d["AMITags"] = existing
			rendered, errs = interpolateTags(p.config.Tags, p.config.ctx, d)
		}

		allErrors = append(allErrors, errs...)
		allErrors = append(allErrors, validateRenderedTags(rendered)...)
		if n := mergedKeyCount(existing, rendered); n > maxTags {
			allErrors = append(allErrors, fmt.Errorf(
				"AMI %s in %s has %d existing tags; applying would result in %d (limit 50): %w",
				a.ID, a.Region, len(existing), n, errMergedTagLimit))
		}
		results = append(results, amiResult{a, client, rendered})
	}

	if len(allErrors) > 0 {
		return artifact, true, false, &packer.MultiError{Errors: allErrors}
	}

	// Pass 2 — tag each AMI with pre-rendered map.
	for _, r := range results {
		keys := make([]string, 0, len(r.rendered))
		for _, k := range slices.Sorted(maps.Keys(r.rendered)) {
			keys = append(keys, k)
		}
		ui.Say(fmt.Sprintf("Tagging AMI %s in %s with keys %v",
			r.ami.ID, r.ami.Region, keys))
		ec2Tags := toEC2Tags(r.rendered)
		if err := retryCreateTags(ctx, r.client, r.ami, ec2Tags, p.retryDelay); err != nil {
			return artifact, true, false, fmt.Errorf(
				"error tagging AMI %s in %s: %w", r.ami.ID, r.ami.Region, err)
		}
		ui.Say(fmt.Sprintf("Tagged AMI %s in %s", r.ami.ID, r.ami.Region))
	}

	// All AMIs tagged successfully — pass through the original artifact.
	return artifact, false, false, nil
}

const (
	maxTags        = 50
	reservedPrefix = "aws:"
	maxKeyLen      = 128
	maxValueLen    = 256
)

// retryTries is kept as a package-level alias for backward compatibility
// with tests that reference it.
var retryTries = helpers.TagRetryTries

var (
	errMergedTagLimit = errors.New("merged tag count exceeds limit")
	errTagLimitAWS    = errors.New("AWS TagLimitExceeded")
)

// supportedBuilderIDs returns the set of builder IDs
// accepted by the post-processor, for use in error messages.
func supportedBuilderIDs() []string {
	return slices.Clone(helpers.AMIBuilderIDs)
}

// isSupportedBuilder reports whether the given builder ID is
// one of the Amazon builders whose artifacts contain AMI IDs.
func isSupportedBuilder(id string) bool {
	return helpers.IsAMIBuilder(id)
}

// toEC2Tags converts a string map to a sorted slice of EC2 Tag structs.
func toEC2Tags(tags map[string]string) []ec2types.Tag {
	return helpers.ToEC2Tags(tags)
}

// tagAPI is the subset of the EC2 client used by the post-processor.
type tagAPI interface {
	CreateTags(ctx context.Context, in *ec2.CreateTagsInput,
		optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	DescribeTags(ctx context.Context, in *ec2.DescribeTagsInput,
		optFns ...func(*ec2.Options)) (*ec2.DescribeTagsOutput, error)
}

// mergedKeyCount returns the number of unique keys in the union of
// existing tag keys and the configured tag map. Overwritten keys
// count once.
func mergedKeyCount(existing map[string]string, tags map[string]string) int {
	seen := make(map[string]struct{}, len(existing)+len(tags))
	for k := range existing {
		seen[k] = struct{}{}
	}
	for k := range tags {
		seen[k] = struct{}{}
	}
	return len(seen)
}

// existingTags returns the tags already present on the given AMI
// as a key→value map.
func existingTags(ctx context.Context, api tagAPI, id string) (map[string]string, error) {
	paginator := ec2.NewDescribeTagsPaginator(api, &ec2.DescribeTagsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("resource-id"), Values: []string{id}},
			{Name: aws.String("resource-type"), Values: []string{"image"}},
		},
	})
	tags := make(map[string]string)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("DescribeTags for %s: %w", id, err)
		}
		for _, t := range page.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}
	return tags, nil
}

// retryCreateTags calls CreateTags with retry on UnauthorizedOperation.
// TagLimitExceeded is not retried but is wrapped with errTagLimitAWS.
// Delegates to helpers.CreateTagsWithRetry.
func retryCreateTags(ctx context.Context, api tagAPI, a helpers.AMIRef,
	tags []ec2types.Tag, delay func(uint) time.Duration) error {
	mapErr := func(err error) error {
		if awserrors.Matches(err, "TagLimitExceeded", "") {
			return fmt.Errorf(
				"AMI %s in %s would exceed the 50-tag limit: %w: %w",
				a.ID, a.Region, errTagLimitAWS, err)
		}
		return err
	}
	return helpers.CreateTagsWithRetry(ctx, api, []string{a.ID}, tags, delay, mapErr)
}

// validateTags checks that the tag map meets AWS constraints.
// Returns a slice of errors; each violating key is named.
func validateTags(tags map[string]string) []error {
	if len(tags) == 0 {
		return []error{fmt.Errorf("at least one tag required")}
	}

	var errs []error
	if len(tags) > maxTags {
		errs = append(errs, fmt.Errorf(
			"tag count %d exceeds maximum of %d", len(tags), maxTags))
	}

	for k, v := range tags {
		if k == "" {
			errs = append(errs, fmt.Errorf("empty tag key not permitted"))
			continue
		}
		if utf8.RuneCountInString(k) > maxKeyLen {
			errs = append(errs, fmt.Errorf(
				"tag key %q exceeds maximum length of %d characters",
				k, maxKeyLen))
		}
		if !strings.Contains(v, "{{") && utf8.RuneCountInString(v) > maxValueLen {
			errs = append(errs, fmt.Errorf(
				"tag key %q: value exceeds maximum length of %d characters",
				k, maxValueLen))
		}
		if len(k) >= len(reservedPrefix) &&
			strings.EqualFold(k[:len(reservedPrefix)], reservedPrefix) {
			errs = append(errs, fmt.Errorf(
				"tag key %q starts with reserved prefix %q",
				k, reservedPrefix))
		}
	}
	return errs
}

// isOnlyAMITagsMissing reports whether every error in errs is a
// missing-variable error that references an AMITags key. This is
// used to decide whether re-reading DescribeTags might resolve
// the failure (eventual consistency after AMI creation).
func isOnlyAMITagsMissing(errs []error) bool {
	for _, e := range errs {
		if !strings.Contains(e.Error(), "AMITags") {
			return false
		}
	}
	return len(errs) > 0
}
