package helpers

import (
	"context"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-amazon/common/awserrors"
	"github.com/hashicorp/packer-plugin-sdk/retry"
)

const (
	// TagRetryTries is the number of CreateTags attempts
	// before giving up on UnauthorizedOperation errors.
	TagRetryTries = 11
)

// ToEC2Tags converts a map[string]string to a sorted []types.Tag.
// nil or empty input yields an empty non-nil slice.
func ToEC2Tags(tags map[string]string) []types.Tag {
	if len(tags) == 0 {
		return []types.Tag{}
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	result := make([]types.Tag, len(keys))
	for i, k := range keys {
		k, v := k, tags[k]
		result[i] = types.Tag{Key: &k, Value: &v}
	}
	return result
}

// CreateTagsWithRetry calls CreateTags on resources, retrying
// UnauthorizedOperation up to TagRetryTries times.
// delay==nil uses Backoff{200ms,30s,x2}.Linear.
// mapErr (may be nil) post-processes each attempt's error
// (e.g. swallow NotFound, wrap TagLimitExceeded).
// RetryExhaustedError is unwrapped so the cause stays
// matchable via errors.Is / errors.As.
func CreateTagsWithRetry(
	ctx context.Context,
	client CreateTagsAPI,
	resources []string,
	tags []types.Tag,
	delay func(attempt uint) time.Duration,
	mapErr func(error) error,
) error {
	if delay == nil {
		b := &retry.Backoff{
			InitialBackoff: 200 * time.Millisecond,
			MaxBackoff:     30 * time.Second,
			Multiplier:     2,
		}
		delay = func(_ uint) time.Duration {
			return b.Linear()
		}
	}

	var lastErr error
	for attempt := uint(0); attempt < TagRetryTries; attempt++ {
		if attempt > 0 {
			time.Sleep(delay(attempt))
		}
		_, err := client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: resources,
			Tags:      tags,
		})
		if err == nil {
			return nil
		}
		lastErr = err
		if !awserrors.Matches(err, "UnauthorizedOperation", "") {
			if mapErr != nil {
				return mapErr(err)
			}
			return err
		}
	}
	if mapErr != nil {
		return mapErr(lastErr)
	}
	return lastErr
}
