package helpers

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// Compile-time checks: *ec2.Client satisfies every narrow interface.
var _ DescribeImagesAPI = (*ec2.Client)(nil)
var _ DeleteAMIAPI = (*ec2.Client)(nil)
var _ CreateTagsAPI = (*ec2.Client)(nil)
var _ ShareImageAPI = (*ec2.Client)(nil)

// DescribeImagesAPI is the subset of the EC2 client used for locating AMIs.
type DescribeImagesAPI interface {
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

// DeleteAMIAPI is the subset of the EC2 client used for deregistering AMIs and deleting snapshots.
type DeleteAMIAPI interface {
	DeregisterImage(ctx context.Context, params *ec2.DeregisterImageInput, optFns ...func(*ec2.Options)) (*ec2.DeregisterImageOutput, error)
	DeleteSnapshot(ctx context.Context, params *ec2.DeleteSnapshotInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSnapshotOutput, error)
}

// CreateTagsAPI is the subset of the EC2 client used for tagging.
type CreateTagsAPI interface {
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
}

// ShareImageAPI is the subset of the EC2 client used for sharing AMIs.
type ShareImageAPI interface {
	DescribeImageAttribute(ctx context.Context, params *ec2.DescribeImageAttributeInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImageAttributeOutput, error)
	ModifyImageAttribute(ctx context.Context, params *ec2.ModifyImageAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyImageAttributeOutput, error)
	ModifySnapshotAttribute(ctx context.Context, params *ec2.ModifySnapshotAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifySnapshotAttributeOutput, error)
}
