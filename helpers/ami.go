package helpers

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

// LocateSingleAMI tries to locate a single AMI for the given ID.
func LocateSingleAMI(ctx context.Context, id string, ec2Conn DescribeImagesAPI) (*types.Image, error) {
	if output, err := ec2Conn.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("image-id"),
				Values: []string{id},
			},
		},
	}); err != nil {
		return nil, err
	} else if len(output.Images) != 1 {
		return nil, fmt.Errorf("single source image not located (found: %d images)", len(output.Images))
	} else {
		return &output.Images[0], nil
	}
}

// IsImageSharedWith checks whether the given AMI (imageID) in the source account
// is shared with the specified target accountID. It returns true if the AMI is
// explicitly shared with the account, or if it is public.
func IsImageSharedWith(ctx context.Context, image *types.Image, accountID string, ec2Conn ShareImageAPI) (bool, error) {
	out, err := ec2Conn.DescribeImageAttribute(ctx, &ec2.DescribeImageAttributeInput{
		ImageId:   image.ImageId,
		Attribute: types.ImageAttributeNameLaunchPermission,
	})
	if err != nil {
		return false, err
	}

	// If no launch permissions are present, image is private to owner.
	// Check each launch permission entry for a matching account ID or public group.
	for _, lp := range out.LaunchPermissions {
		if lp.UserId != nil && *lp.UserId == accountID {
			return true, nil
		}
		if lp.Group != "" && lp.Group == "all" {
			return true, nil
		}
	}

	return false, nil
}

// EnsureImageSharedWith ensures the given AMI (imageID) in the source account
// is shared with the target accountID.
func EnsureImageSharedWith(ctx context.Context, image *types.Image, accountID *string, ec2Conn ShareImageAPI) error {
	if shared, err := IsImageSharedWith(ctx, image, *accountID, ec2Conn); err != nil {
		return err
	} else if shared {
		return nil
	}
	log.Println("Modifying LaunchPermissions for AMI", *image.ImageId, "with account", *accountID)
	_, err := ec2Conn.ModifyImageAttribute(ctx, &ec2.ModifyImageAttributeInput{
		ImageId: image.ImageId,
		LaunchPermission: &types.LaunchPermissionModifications{
			Add: []types.LaunchPermission{
				{
					UserId: accountID,
				},
			},
		},
	})
	if err != nil {
		return err
	}

	var errs *packer.MultiError
	for _, bdm := range image.BlockDeviceMappings {
		if bdm.Ebs != nil && bdm.Ebs.SnapshotId != nil {
			log.Printf("Modifying CreateVolumePermission for AMI %s with account %s", *image.ImageId, *accountID)
			_, err := ec2Conn.ModifySnapshotAttribute(ctx, &ec2.ModifySnapshotAttributeInput{
				SnapshotId: bdm.Ebs.SnapshotId,
				CreateVolumePermission: &types.CreateVolumePermissionModifications{
					Add: []types.CreateVolumePermission{
						{
							UserId: accountID,
						},
					},
				},
			})
			if err != nil {
				errs = packer.MultiErrorAppend(errs, err)
			}
		}
	}
	if errs != nil && len(errs.Errors) != 0 {
		return errs
	}
	return nil
}

// DeleteAMIAndSnapshots deregisters an AMI and deletes its EBS snapshots.
// onSnapshot (may be nil) is called before each snapshot delete.
func DeleteAMIAndSnapshots(ctx context.Context, client DeleteAMIAPI, image *types.Image, onSnapshot func(string)) error {
	if image == nil {
		return errors.New("image must not be nil")
	}
	if image.ImageId == nil {
		return errors.New("image.ImageId must not be nil")
	}

	// Deregister the AMI
	_, err := client.DeregisterImage(ctx, &ec2.DeregisterImageInput{
		ImageId: image.ImageId,
	})
	if err != nil {
		return fmt.Errorf("deregister image %s: %w", *image.ImageId, err)
	}

	// Delete associated snapshots
	for _, bdm := range image.BlockDeviceMappings {
		if bdm.Ebs == nil || bdm.Ebs.SnapshotId == nil {
			continue
		}
		snapID := *bdm.Ebs.SnapshotId
		if onSnapshot != nil {
			onSnapshot(snapID)
		}
		_, err := client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
			SnapshotId: &snapID,
		})
		if err != nil {
			return fmt.Errorf("delete snapshot %s: %w", snapID, err)
		}
	}

	return nil
}
