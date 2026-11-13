package helpers_test

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/bdwyertech/packer-plugin-aws/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockDescribeImagesAPI is a minimal fake satisfying DescribeImagesAPI.
type mockDescribeImagesAPI struct {
	output *ec2.DescribeImagesOutput
	err    error
}

func (m *mockDescribeImagesAPI) DescribeImages(_ context.Context, _ *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return m.output, m.err
}

// mockShareImageAPI is a minimal fake satisfying ShareImageAPI.
type mockShareImageAPI struct {
	describeOutput *ec2.DescribeImageAttributeOutput
	describeErr    error
	modifyImageErr error
	modifySnapErr  error
}

func (m *mockShareImageAPI) DescribeImageAttribute(_ context.Context, _ *ec2.DescribeImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.DescribeImageAttributeOutput, error) {
	return m.describeOutput, m.describeErr
}

func (m *mockShareImageAPI) ModifyImageAttribute(_ context.Context, _ *ec2.ModifyImageAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error) {
	return &ec2.ModifyImageAttributeOutput{}, m.modifyImageErr
}

func (m *mockShareImageAPI) ModifySnapshotAttribute(_ context.Context, _ *ec2.ModifySnapshotAttributeInput, _ ...func(*ec2.Options)) (*ec2.ModifySnapshotAttributeOutput, error) {
	return &ec2.ModifySnapshotAttributeOutput{}, m.modifySnapErr
}

// mockDeleteAMIAPI is a minimal fake satisfying DeleteAMIAPI.
type mockDeleteAMIAPI struct {
	deregisterErr error
	deleteSnapErr error
}

func (m *mockDeleteAMIAPI) DeregisterImage(_ context.Context, _ *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	return &ec2.DeregisterImageOutput{}, m.deregisterErr
}

func (m *mockDeleteAMIAPI) DeleteSnapshot(_ context.Context, _ *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	return &ec2.DeleteSnapshotOutput{}, m.deleteSnapErr
}

// mockCreateTagsAPI is a minimal fake satisfying CreateTagsAPI.
type mockCreateTagsAPI struct {
	err error
}

func (m *mockCreateTagsAPI) CreateTags(_ context.Context, _ *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	return &ec2.CreateTagsOutput{}, m.err
}

var _ = Describe("Narrow EC2 interfaces", func() {

	Context("compile-time satisfaction checks", func() {
		// These mirror the var _ checks in api.go. If any interface
		// method signature drifts from *ec2.Client, both the source
		// file and these specs will fail to compile.

		It("*ec2.Client satisfies DescribeImagesAPI", func() {
			var _ helpers.DescribeImagesAPI = (*ec2.Client)(nil)
			Succeed()
		})

		It("*ec2.Client satisfies DeleteAMIAPI", func() {
			var _ helpers.DeleteAMIAPI = (*ec2.Client)(nil)
			Succeed()
		})

		It("*ec2.Client satisfies CreateTagsAPI", func() {
			var _ helpers.CreateTagsAPI = (*ec2.Client)(nil)
			Succeed()
		})

		It("*ec2.Client satisfies ShareImageAPI", func() {
			var _ helpers.ShareImageAPI = (*ec2.Client)(nil)
			Succeed()
		})
	})

	Context("mock implementations satisfy interfaces", func() {
		It("mockDescribeImagesAPI satisfies DescribeImagesAPI", func() {
			var api helpers.DescribeImagesAPI = &mockDescribeImagesAPI{}
			Expect(api).NotTo(BeNil())
		})

		It("mockDeleteAMIAPI satisfies DeleteAMIAPI", func() {
			var api helpers.DeleteAMIAPI = &mockDeleteAMIAPI{}
			Expect(api).NotTo(BeNil())
		})

		It("mockCreateTagsAPI satisfies CreateTagsAPI", func() {
			var api helpers.CreateTagsAPI = &mockCreateTagsAPI{}
			Expect(api).NotTo(BeNil())
		})

		It("mockShareImageAPI satisfies ShareImageAPI", func() {
			var api helpers.ShareImageAPI = &mockShareImageAPI{}
			Expect(api).NotTo(BeNil())
		})
	})

	Context("mock implementations are callable", func() {
		It("mockDescribeImagesAPI returns configured output", func() {
			expected := &ec2.DescribeImagesOutput{}
			mock := &mockDescribeImagesAPI{output: expected}
			out, err := mock.DescribeImages(context.Background(), &ec2.DescribeImagesInput{})
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal(expected))
		})

		It("mockDescribeImagesAPI returns configured error", func() {
			mock := &mockDescribeImagesAPI{err: errFake}
			_, err := mock.DescribeImages(context.Background(), &ec2.DescribeImagesInput{})
			Expect(err).To(MatchError(errFake))
		})

		It("mockShareImageAPI returns configured error on DescribeImageAttribute", func() {
			mock := &mockShareImageAPI{describeErr: errFake}
			_, err := mock.DescribeImageAttribute(context.Background(), &ec2.DescribeImageAttributeInput{})
			Expect(err).To(MatchError(errFake))
		})

		It("mockDeleteAMIAPI returns configured error on DeregisterImage", func() {
			mock := &mockDeleteAMIAPI{deregisterErr: errFake}
			_, err := mock.DeregisterImage(context.Background(), &ec2.DeregisterImageInput{})
			Expect(err).To(MatchError(errFake))
		})

		It("mockCreateTagsAPI returns configured error", func() {
			mock := &mockCreateTagsAPI{err: errFake}
			_, err := mock.CreateTags(context.Background(), &ec2.CreateTagsInput{})
			Expect(err).To(MatchError(errFake))
		})
	})
})

var errFake = fmt.Errorf("fake error")
