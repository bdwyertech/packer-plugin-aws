package helpers_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/bdwyertech/packer-plugin-aws/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// spyDeleteAMIAPI records every call for assertions.
type spyDeleteAMIAPI struct {
	deregisterCalls []string // ImageIds passed to DeregisterImage
	deregisterErr   error

	deletedSnapshots []string // SnapshotIds passed to DeleteSnapshot
	// deleteSnapErrs maps snapshot ID to the error returned.
	// A nil map or missing key means success.
	deleteSnapErrs map[string]error
}

func (s *spyDeleteAMIAPI) DeregisterImage(_ context.Context, in *ec2.DeregisterImageInput, _ ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error) {
	s.deregisterCalls = append(s.deregisterCalls, aws.ToString(in.ImageId))
	return &ec2.DeregisterImageOutput{}, s.deregisterErr
}

func (s *spyDeleteAMIAPI) DeleteSnapshot(_ context.Context, in *ec2.DeleteSnapshotInput, _ ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error) {
	sid := aws.ToString(in.SnapshotId)
	s.deletedSnapshots = append(s.deletedSnapshots, sid)
	if s.deleteSnapErrs != nil {
		if err, ok := s.deleteSnapErrs[sid]; ok {
			return &ec2.DeleteSnapshotOutput{}, err
		}
	}
	return &ec2.DeleteSnapshotOutput{}, nil
}

// imageWith builds a types.Image with the given snapshot IDs as EBS mappings.
func imageWith(imageID string, snapIDs ...string) *types.Image {
	var bdms []types.BlockDeviceMapping
	for _, sid := range snapIDs {
		bdms = append(bdms, types.BlockDeviceMapping{
			Ebs: &types.EbsBlockDevice{
				SnapshotId: aws.String(sid),
			},
		})
	}
	return &types.Image{
		ImageId:             aws.String(imageID),
		BlockDeviceMappings: bdms,
	}
}

var _ = Describe("DeleteAMIAndSnapshots", func() {
	var (
		ctx context.Context
		spy *spyDeleteAMIAPI
	)

	BeforeEach(func() {
		ctx = context.Background()
		spy = &spyDeleteAMIAPI{}
	})

	Context("when image is nil", func() {
		It("returns an error", func() {
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil"))
		})

		It("makes no API calls", func() {
			_ = helpers.DeleteAMIAndSnapshots(ctx, spy, nil, nil)
			Expect(spy.deregisterCalls).To(BeEmpty())
			Expect(spy.deletedSnapshots).To(BeEmpty())
		})
	})

	Context("when image.ImageId is nil", func() {
		It("returns an error", func() {
			img := &types.Image{ImageId: nil}
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nil"))
		})

		It("makes no API calls", func() {
			img := &types.Image{ImageId: nil}
			_ = helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(spy.deregisterCalls).To(BeEmpty())
			Expect(spy.deletedSnapshots).To(BeEmpty())
		})
	})

	Context("when DeregisterImage fails", func() {
		var deregErr error

		BeforeEach(func() {
			deregErr = fmt.Errorf("access denied")
			spy.deregisterErr = deregErr
		})

		It("returns the error wrapped with %%w", func() {
			img := imageWith("ami-fail", "snap-1", "snap-2")
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, deregErr)).To(BeTrue(),
				"returned error should wrap the original via %%w")
		})

		It("does not call DeleteSnapshot", func() {
			img := imageWith("ami-fail", "snap-1", "snap-2")
			_ = helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(spy.deletedSnapshots).To(BeEmpty())
		})
	})

	Context("when DeregisterImage succeeds", func() {
		It("deletes snapshots from block device mappings in order", func() {
			img := imageWith("ami-ok", "snap-a", "snap-b", "snap-c")
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.deregisterCalls).To(Equal([]string{"ami-ok"}))
			Expect(spy.deletedSnapshots).To(Equal(
				[]string{"snap-a", "snap-b", "snap-c"}))
		})

		It("calls onSnapshot callback before each delete", func() {
			var callbackOrder []string
			onSnap := func(sid string) {
				callbackOrder = append(callbackOrder, sid)
			}
			img := imageWith("ami-ok", "snap-x", "snap-y")
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, img, onSnap)
			Expect(err).NotTo(HaveOccurred())
			Expect(callbackOrder).To(Equal([]string{"snap-x", "snap-y"}))
		})

		It("stops at first snapshot deletion failure", func() {
			spy.deleteSnapErrs = map[string]error{
				"snap-b": fmt.Errorf("snap delete failed"),
			}
			img := imageWith("ami-ok", "snap-a", "snap-b", "snap-c")
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("snap-b"))
			// snap-c must NOT have been attempted
			Expect(spy.deletedSnapshots).To(Equal(
				[]string{"snap-a", "snap-b"}))
		})

		It("skips non-EBS mappings", func() {
			img := &types.Image{
				ImageId: aws.String("ami-mixed"),
				BlockDeviceMappings: []types.BlockDeviceMapping{
					{Ebs: &types.EbsBlockDevice{
						SnapshotId: aws.String("snap-1")}},
					{DeviceName: aws.String("/dev/sdb")}, // no Ebs
					{Ebs: &types.EbsBlockDevice{
						SnapshotId: aws.String("snap-3")}},
				},
			}
			err := helpers.DeleteAMIAndSnapshots(ctx, spy, img, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(spy.deletedSnapshots).To(Equal(
				[]string{"snap-1", "snap-3"}))
		})
	})
})
