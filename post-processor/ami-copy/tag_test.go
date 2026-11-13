package ami_copy

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockTagCopyAPI is a copyAPI fake for tagImage tests.
type mockTagCopyAPI struct {
	createTagsCalls int
	createTagsErr   error
}

func (m *mockTagCopyAPI) CopyImage(_ context.Context, _ *ec2.CopyImageInput, _ ...func(*ec2.Options)) (*ec2.CopyImageOutput, error) {
	return nil, errors.New("CopyImage not expected in tag tests")
}

func (m *mockTagCopyAPI) CreateTags(_ context.Context, _ *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	m.createTagsCalls++
	return &ec2.CreateTagsOutput{}, m.createTagsErr
}

func (m *mockTagCopyAPI) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return nil, errors.New("DescribeImages not expected in tag tests")
}

func (m *mockTagCopyAPI) DeregisterImage(_ context.Context, _ *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	return nil, errors.New("DeregisterImage not expected in tag tests")
}

func (m *mockTagCopyAPI) DeleteSnapshot(_ context.Context, _ *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	return nil, errors.New("DeleteSnapshot not expected in tag tests")
}

// Compile-time check: mockTagCopyAPI satisfies copyAPI.
var _ copyAPI = (*mockTagCopyAPI)(nil)

var _ = Describe("tagImage", func() {
	var (
		ctx  context.Context
		mock *mockTagCopyAPI
		ui   *packer.MockUi
		op   *copyOperation
	)

	noDelay := func(_ uint) time.Duration { return 0 }

	BeforeEach(func() {
		ctx = context.Background()
		mock = &mockTagCopyAPI{}
		ui = &packer.MockUi{}
		op = &copyOperation{
			ctx:           ctx,
			client:        mock,
			copiedImageID: "ami-test123",
			tags:          map[string]string{"Name": "test"},
			retryDelay:    noDelay,
		}
	})

	Context("InvalidAMIID.NotFound is swallowed", func() {
		BeforeEach(func() {
			mock.createTagsErr = &smithy.GenericAPIError{
				Code:    "InvalidAMIID.NotFound",
				Message: "AMI not found",
			}
		})

		It("returns nil", func() {
			err := op.tagImage(ui)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("InvalidSnapshot.NotFound is swallowed", func() {
		BeforeEach(func() {
			mock.createTagsErr = &smithy.GenericAPIError{
				Code:    "InvalidSnapshot.NotFound",
				Message: "Snapshot not found",
			}
		})

		It("returns nil", func() {
			err := op.tagImage(ui)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("other errors are propagated", func() {
		BeforeEach(func() {
			mock.createTagsErr = &smithy.GenericAPIError{
				Code:    "InternalError",
				Message: "something went wrong",
			}
		})

		It("returns the error", func() {
			err := op.tagImage(ui)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("InternalError"))
		})
	})

	Context("no tags to apply", func() {
		BeforeEach(func() {
			op.tags = nil
		})

		It("returns nil without calling CreateTags", func() {
			err := op.tagImage(ui)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.createTagsCalls).To(Equal(0))
		})
	})
})
