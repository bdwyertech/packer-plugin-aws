package ami_copy

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockWaiterCopyAPI is a copyAPI fake for waitForAvailable tests.
type mockWaiterCopyAPI struct {
	describeImagesFn func(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

func (m *mockWaiterCopyAPI) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return m.describeImagesFn(ctx, in, opts...)
}

func (m *mockWaiterCopyAPI) CopyImage(context.Context, *ec2.CopyImageInput, ...func(*ec2.Options)) (*ec2.CopyImageOutput, error) {
	return nil, errors.New("CopyImage not expected in waiter tests")
}

func (m *mockWaiterCopyAPI) CreateTags(context.Context, *ec2.CreateTagsInput, ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	return nil, errors.New("CreateTags not expected in waiter tests")
}

func (m *mockWaiterCopyAPI) DeregisterImage(context.Context, *ec2.DeregisterImageInput, ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	return nil, errors.New("DeregisterImage not expected in waiter tests")
}

func (m *mockWaiterCopyAPI) DeleteSnapshot(context.Context, *ec2.DeleteSnapshotInput, ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	return nil, errors.New("DeleteSnapshot not expected in waiter tests")
}

var _ copyAPI = (*mockWaiterCopyAPI)(nil)

// noopUi implements packer.Ui with no-op methods for testing.
type noopUi struct{}

func (u *noopUi) Ask(string) (string, error)          { return "", nil }
func (u *noopUi) Askf(string, ...any) (string, error) { return "", nil }
func (u *noopUi) Say(string)                          {}
func (u *noopUi) Sayf(string, ...any)                 {}
func (u *noopUi) Message(string)                      {}
func (u *noopUi) Error(string)                        {}
func (u *noopUi) Errorf(string, ...any)               {}
func (u *noopUi) Machine(string, ...string)           {}

func (u *noopUi) TrackProgress(_ string, _, _ int64, stream io.ReadCloser) io.ReadCloser {
	return stream
}

var _ packer.Ui = (*noopUi)(nil)

var _ = Describe("waitForAvailable", func() {
	var (
		ctx context.Context
		ui  packer.Ui
	)

	BeforeEach(func() {
		ctx = context.Background()
		ui = &noopUi{}
	})

	Context("DescribeImages error propagation", func() {
		It("returns the DescribeImages error", func() {
			apiErr := errors.New("DescribeImages: throttled")
			mock := &mockWaiterCopyAPI{
				describeImagesFn: func(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
					return nil, apiErr
				},
			}
			op := &copyOperation{
				ctx:            ctx,
				client:         mock,
				region:         "us-east-1",
				copiedImageID:  "ami-test123",
				timeoutMinutes: 1,
				waiterOptions: []func(*ec2.ImageAvailableWaiterOptions){
					func(o *ec2.ImageAvailableWaiterOptions) {
						o.MinDelay = 10 * time.Millisecond
						o.MaxDelay = 10 * time.Millisecond
					},
				},
			}
			err := op.waitForAvailable(ui)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("throttled"))
		})
	})

	Context("failed state returns error without nil dereference", func() {
		It("returns an error about the failed state", func() {
			mock := &mockWaiterCopyAPI{
				describeImagesFn: func(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
					return &ec2.DescribeImagesOutput{
						Images: []types.Image{{
							ImageId:     aws.String("ami-test123"),
							State:       types.ImageStateFailed,
							OwnerId:     nil,
							StateReason: nil,
						}},
					}, nil
				},
			}
			op := &copyOperation{
				ctx:            ctx,
				client:         mock,
				region:         "us-east-1",
				copiedImageID:  "ami-test123",
				timeoutMinutes: 1,
				waiterOptions: []func(*ec2.ImageAvailableWaiterOptions){
					func(o *ec2.ImageAvailableWaiterOptions) {
						o.MinDelay = 10 * time.Millisecond
						o.MaxDelay = 10 * time.Millisecond
					},
				},
			}
			err := op.waitForAvailable(ui)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(MatchRegexp("(?i)fail"))
		})
	})
})
