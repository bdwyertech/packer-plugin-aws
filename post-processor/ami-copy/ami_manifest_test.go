package ami_copy

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockDeleteCopyAPI is a copyAPI fake for AmiManifest.delete tests.
type mockDeleteCopyAPI struct {
	describeImagesOut *ec2.DescribeImagesOutput
	describeImagesErr error

	deregisterImageErr error

	deleteSnapshotCalls []string // snapshot IDs passed to DeleteSnapshot
	deleteSnapshotErr   error
}

func (m *mockDeleteCopyAPI) DescribeImages(_ context.Context, params *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if m.describeImagesErr != nil {
		return nil, m.describeImagesErr
	}
	return m.describeImagesOut, nil
}

func (m *mockDeleteCopyAPI) DeregisterImage(_ context.Context, _ *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	if m.deregisterImageErr != nil {
		return nil, m.deregisterImageErr
	}
	return &ec2.DeregisterImageOutput{}, nil
}

func (m *mockDeleteCopyAPI) DeleteSnapshot(_ context.Context, params *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	m.deleteSnapshotCalls = append(m.deleteSnapshotCalls, *params.SnapshotId)
	if m.deleteSnapshotErr != nil {
		return nil, m.deleteSnapshotErr
	}
	return &ec2.DeleteSnapshotOutput{}, nil
}

func (m *mockDeleteCopyAPI) CopyImage(_ context.Context, _ *ec2.CopyImageInput, _ ...func(*ec2.Options)) (*ec2.CopyImageOutput, error) {
	return nil, errors.New("CopyImage not expected in delete tests")
}

func (m *mockDeleteCopyAPI) CreateTags(_ context.Context, _ *ec2.CreateTagsInput, _ ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	return nil, errors.New("CreateTags not expected in delete tests")
}

// Compile-time check: mockDeleteCopyAPI satisfies copyAPI.
var _ copyAPI = (*mockDeleteCopyAPI)(nil)

var _ = Describe("AmiManifest.delete", func() {
	var (
		ctx      context.Context
		mock     *mockDeleteCopyAPI
		manifest *AmiManifest
	)

	Context("DeregisterImage error is lost when snapshots exist", func() {
		deregErr := errors.New("DeregisterImage: access denied")

		BeforeEach(func() {
			ctx = context.Background()

			mock = &mockDeleteCopyAPI{
				describeImagesOut: &ec2.DescribeImagesOutput{
					Images: []types.Image{{
						ImageId: aws.String("ami-test123"),
						OwnerId: aws.String("111111111111"),
						BlockDeviceMappings: []types.BlockDeviceMapping{
							{
								Ebs: &types.EbsBlockDevice{
									SnapshotId: aws.String("snap-aaa111"),
								},
							},
						},
					}},
				},
				deregisterImageErr: deregErr,
			}

			manifest = &AmiManifest{
				Region:  "us-east-1",
				ImageID: "ami-test123",
				client:  mock,
			}
		})

		It("returns the DeregisterImage error", func() {
			err := manifest.delete(ctx)
			Expect(err).To(HaveOccurred(), "delete must propagate DeregisterImage error")
			Expect(err.Error()).To(ContainSubstring("access denied"))
		})

		It("does not call DeleteSnapshot", func() {
			_ = manifest.delete(ctx)
			Expect(mock.deleteSnapshotCalls).To(BeEmpty(),
				"DeleteSnapshot must not be called when DeregisterImage fails")
		})
	})
})
