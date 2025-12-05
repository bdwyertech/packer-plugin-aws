package helpers

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// LocateSingleAMI tries to locate a single AMI for the given ID.
func LocateSingleAMI(ctx context.Context, id string, ec2Conn *ec2.Client) (*types.Image, error) {
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
