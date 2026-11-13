package helpers_test

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"github.com/bdwyertech/packer-plugin-aws/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ToEC2Tags", func() {
	DescribeTable("converts map to sorted EC2 tags",
		func(input map[string]string, expected []types.Tag) {
			result := helpers.ToEC2Tags(input)
			Expect(result).To(Equal(expected))
		},
		Entry("nil input yields empty non-nil slice",
			nil, []types.Tag{}),
		Entry("empty map yields empty non-nil slice",
			map[string]string{}, []types.Tag{}),
		Entry("single entry",
			map[string]string{"Name": "test"},
			[]types.Tag{
				{Key: aws.String("Name"), Value: aws.String("test")},
			}),
		Entry("multi-entry sorted by key",
			map[string]string{
				"Env": "prod", "App": "web",
			},
			[]types.Tag{
				{Key: aws.String("App"), Value: aws.String("web")},
				{Key: aws.String("Env"), Value: aws.String("prod")},
			}),
	)

	It("key-value set equals input", func() {
		input := map[string]string{"a": "1", "b": "2", "c": "3"}
		result := helpers.ToEC2Tags(input)
		Expect(result).To(HaveLen(3))
		for _, t := range result {
			Expect(input).To(HaveKeyWithValue(*t.Key, *t.Value))
		}
	})
})

// retryTagMock counts calls and returns a configurable error.
type retryTagMock struct {
	calls int
	err   error
}

func (m *retryTagMock) CreateTags(
	ctx context.Context, params *ec2.CreateTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.CreateTagsOutput, error) {
	m.calls++
	return &ec2.CreateTagsOutput{}, m.err
}

var _ = Describe("CreateTagsWithRetry", func() {
	var (
		ctx     context.Context
		mock    *retryTagMock
		tags    []types.Tag
		res     []string
		noDelay func(uint) time.Duration
	)

	BeforeEach(func() {
		ctx = context.Background()
		mock = &retryTagMock{}
		tags = []types.Tag{
			{Key: aws.String("k"), Value: aws.String("v")},
		}
		res = []string{"ami-123"}
		noDelay = func(_ uint) time.Duration { return 0 }
	})

	Context("success on first try", func() {
		It("returns nil", func() {
			err := helpers.CreateTagsWithRetry(
				ctx, mock, res, tags, noDelay, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.calls).To(Equal(1))
		})
	})

	Context("retries UnauthorizedOperation", func() {
		It("retries up to TagRetryTries times", func() {
			mock.err = &smithy.GenericAPIError{
				Code:    "UnauthorizedOperation",
				Message: "test",
			}
			err := helpers.CreateTagsWithRetry(
				ctx, mock, res, tags, noDelay, nil)
			Expect(err).To(HaveOccurred())
			Expect(mock.calls).To(Equal(helpers.TagRetryTries))
		})
	})

	Context("mapErr transforms errors", func() {
		It("applies mapErr to the final error", func() {
			sentinel := errors.New("mapped")
			mock.err = &smithy.GenericAPIError{
				Code:    "SomeNonRetryable",
				Message: "test",
			}
			mapper := func(_ error) error { return sentinel }
			err := helpers.CreateTagsWithRetry(
				ctx, mock, res, tags, noDelay, mapper)
			Expect(errors.Is(err, sentinel)).To(BeTrue())
			Expect(mock.calls).To(Equal(1))
		})
	})

	Context("nil mapErr passes errors through", func() {
		It("returns the original error", func() {
			cause := &smithy.GenericAPIError{
				Code:    "SomeNonRetryable",
				Message: "test",
			}
			mock.err = cause
			err := helpers.CreateTagsWithRetry(
				ctx, mock, res, tags, noDelay, nil)
			Expect(err).To(HaveOccurred())
			var apiErr smithy.APIError
			Expect(errors.As(err, &apiErr)).To(BeTrue())
			Expect(apiErr.ErrorCode()).To(Equal("SomeNonRetryable"))
		})
	})

	Context("exhaustion preserves cause", func() {
		It("allows errors.Is to match the inner cause", func() {
			cause := &smithy.GenericAPIError{
				Code:    "UnauthorizedOperation",
				Message: "exhaustion test",
			}
			mock.err = cause
			err := helpers.CreateTagsWithRetry(
				ctx, mock, res, tags, noDelay, nil)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, cause)).To(BeTrue())
		})
	})
})
