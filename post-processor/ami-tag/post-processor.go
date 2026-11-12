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
	"github.com/hashicorp/packer-plugin-amazon/builder/chroot"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebssurrogate"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebsvolume"
	"github.com/hashicorp/packer-plugin-amazon/builder/instance"
	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/retry"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	ami_copy "github.com/bdwyertech/packer-plugin-aws/post-processor/ami-copy"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

// BuilderId is the ID of this post processor.
// nolint: golint
const BuilderId = "packer.post-processor.ami-tag"

const (
	maxTags        = 50
	reservedPrefix = "aws:"
	maxKeyLen      = 128
	maxValueLen    = 256
	retryTries     = 11
)

var (
	errMergedTagLimit = errors.New("merged tag count exceeds limit")
	errTagLimitAWS    = errors.New("AWS TagLimitExceeded")
)

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
	amis, err := parseArtifactID(artifact.Id())
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
		ami      ami
		client   tagAPI
		rendered map[string]string
	}
	var results []amiResult
	var allErrors []error
	warnedAMITags := false
	clients := make(map[string]tagAPI)
	for _, a := range amis {
		client, ok := clients[a.region]
		if !ok {
			cfg := awsCfg.Copy()
			cfg.Region = a.region
			if p.newClient != nil {
				client = p.newClient(cfg, a.region)
			} else {
				client = ec2.NewFromConfig(cfg)
			}
			clients[a.region] = client
		}
		existing, err := existingTags(ctx, client, a.id)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf(
				"error reading tags for AMI %s in %s: %w", a.id, a.region, err))
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
				a.id, attempt+1))
			if p.retryDelay != nil {
				time.Sleep(p.retryDelay(attempt))
			} else {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
			}
			re, reErr := existingTags(ctx, client, a.id)
			if reErr != nil {
				errs = []error{fmt.Errorf(
					"error reading tags for AMI %s in %s: %w",
					a.id, a.region, reErr)}
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
				a.id, a.region, len(existing), n, errMergedTagLimit))
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
			r.ami.id, r.ami.region, keys))
		ec2Tags := toEC2Tags(r.rendered)
		if err := retryCreateTags(ctx, r.client, r.ami, ec2Tags, p.retryDelay); err != nil {
			return artifact, true, false, fmt.Errorf(
				"error tagging AMI %s in %s: %w", r.ami.id, r.ami.region, err)
		}
		ui.Say(fmt.Sprintf("Tagged AMI %s in %s", r.ami.id, r.ami.region))
	}

	// All AMIs tagged successfully — pass through the original artifact.
	return artifact, false, false, nil
}

// ami is one parsed "region:ami-id" artifact entry.
type ami struct {
	id, region string
}

// supportedBuilderIDs returns the set of builder IDs
// accepted by the post-processor, for use in error messages.
func supportedBuilderIDs() []string {
	return []string{
		ebs.BuilderId,
		ebssurrogate.BuilderId,
		ebsvolume.BuilderId,
		chroot.BuilderId,
		instance.BuilderId,
	}
}

// isSupportedBuilder reports whether the given builder ID is
// one of the Amazon builders whose artifacts contain AMI IDs.
func isSupportedBuilder(id string) bool {
	switch id {
	case ebs.BuilderId,
		ebssurrogate.BuilderId,
		ebsvolume.BuilderId,
		chroot.BuilderId,
		instance.BuilderId:
		return true
	}
	return false
}

// toEC2Tags converts a string map to a sorted slice of EC2 Tag structs.
func toEC2Tags(tags map[string]string) []ec2types.Tag {
	result := make([]ec2types.Tag, 0, len(tags))
	for _, k := range slices.Sorted(maps.Keys(tags)) {
		result = append(result, ec2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(tags[k]),
		})
	}
	return result
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
// TagLimitExceeded is not retried. RetryExhaustedError is unwrapped so
// the underlying AWS error remains matchable by awserrors.Matches.
func retryCreateTags(ctx context.Context, api tagAPI, a ami,
	tags []ec2types.Tag, delay func(uint) time.Duration) error {
	var retryDelay func() time.Duration
	if delay == nil {
		retryDelay = (&retry.Backoff{
			InitialBackoff: 200 * time.Millisecond,
			MaxBackoff:     30 * time.Second,
			Multiplier:     2,
		}).Linear
	} else {
		var attempt uint
		retryDelay = func() time.Duration {
			d := delay(attempt)
			attempt++
			return d
		}
	}
	err := retry.Config{
		Tries: retryTries,
		ShouldRetry: func(e error) bool {
			return awserrors.Matches(e, "UnauthorizedOperation", "")
		},
		RetryDelay: retryDelay,
	}.Run(ctx, func(ctx context.Context) error {
		_, e := api.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{a.id},
			Tags:      tags,
		})
		if awserrors.Matches(e, "TagLimitExceeded", "") {
			return fmt.Errorf(
				"AMI %s in %s would exceed the 50-tag limit: %w: %w",
				a.id, a.region, errTagLimitAWS, e)
		}
		return e
	})

	// RetryExhaustedError (packer-plugin-sdk v0.6.13) has no Unwrap
	// method, so errors.As/awserrors.Matches can't see the inner AWS
	// error. Unwrap .Err so the cause is preserved in the chain.
	if re := (*retry.RetryExhaustedError)(nil); errors.As(err, &re) {
		err = fmt.Errorf("retries exhausted after %d attempts: %w",
			retryTries, re.Err)
	}
	return err
}

// tagAMI orchestrates the tagging of a single AMI:
// log, read existing tags, check merged count, call
// CreateTags (with retry), and report the result.
func tagAMI(ctx context.Context, ui packer.Ui,
	api tagAPI, a ami, tags map[string]string,
	delay func(uint) time.Duration) error {

	// Step 1: Log the start with sorted keys (never values).
	keys := make([]string, 0, len(tags))
	for _, k := range slices.Sorted(maps.Keys(tags)) {
		keys = append(keys, k)
	}
	ui.Say(fmt.Sprintf(
		"Tagging AMI %s in %s with keys %v",
		a.id, a.region, keys))

	// Step 2: Read existing tags.
	existing, err := existingTags(ctx, api, a.id)
	if err != nil {
		wrapped := fmt.Errorf(
			"error reading tags for AMI %s in %s: %w",
			a.id, a.region, err)
		ui.Error(wrapped.Error())
		return wrapped
	}

	// Step 3: Check merged count.
	n := mergedKeyCount(existing, tags)
	if n > maxTags {
		wrapped := fmt.Errorf(
			"AMI %s in %s has %d existing tags; "+
				"applying would result in %d (limit 50): %w",
			a.id, a.region, len(existing), n,
			errMergedTagLimit)
		ui.Error(wrapped.Error())
		return wrapped
	}

	// Step 4: Convert to EC2 tags for the API call.
	ec2Tags := toEC2Tags(tags)

	// Retry block: CreateTags with retry
	// (implemented in task 3.4).
	err = retryCreateTags(ctx, api, a, ec2Tags, delay)

	// Step 5: Handle result.
	if err != nil {
		wrapped := fmt.Errorf(
			"error tagging AMI %s in %s: %w",
			a.id, a.region, err)
		ui.Error(wrapped.Error())
		return wrapped
	}
	ui.Say(fmt.Sprintf("Tagged AMI %s in %s", a.id, a.region))
	return nil
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

// parseArtifactID splits a comma-separated list of
// "region:ami-id" entries into an ami slice.
// It returns a descriptive error for empty input, entries
// without exactly one ':', or entries with empty parts.
func parseArtifactID(id string) ([]ami, error) {
	if id == "" {
		return nil, fmt.Errorf("artifact ID is empty")
	}
	entries := strings.Split(id, ",")
	amis := make([]ami, 0, len(entries))
	for _, entry := range entries {
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(
				"artifact entry %q: expected region:ami-id", entry)
		}
		if parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf(
				"artifact entry %q: region and ami-id must not be empty",
				entry)
		}
		amis = append(amis, ami{region: parts[0], id: parts[1]})
	}
	return amis, nil
}
