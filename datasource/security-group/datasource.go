//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput,Filter

package securitygroup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

// Filter represents a custom filter for security group lookup
type Filter struct {
	// Name of the field to filter by, as defined by the AWS API
	Name string `mapstructure:"name" required:"true"`
	// Set of values that are accepted for the given field
	Values []string `mapstructure:"values" required:"true"`
}

// Config is the configuration structure for the security group datasource
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// ID of the specific security group to retrieve
	ID string `mapstructure:"id" required:"false"`
	// Name that the desired security group must have
	Name string `mapstructure:"name" required:"false"`
	// ID of the VPC that the desired security group belongs to
	VpcID string `mapstructure:"vpc_id" required:"false"`
	// Map of tags, each pair of which must exactly match a pair on the desired security group
	Tags map[string]string `mapstructure:"tags" required:"false"`
	// Custom filters for more complex queries
	Filters []Filter `mapstructure:"filter" required:"false"`
}

// Datasource implements the security group datasource
type Datasource struct {
	config Config
}

// ConfigSpec returns the HCL object spec for the config
func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

// DatasourceOutput contains all the output attributes for the security group datasource
type DatasourceOutput struct {
	// ID of the security group
	ID string `mapstructure:"id"`
	// ARN of the security group
	ARN string `mapstructure:"arn"`
	// Name of the security group
	Name string `mapstructure:"name"`
	// AWS region where the security group is located
	Region string `mapstructure:"region"`
	// Description of the security group
	Description string `mapstructure:"description"`
	// ID of the VPC the security group belongs to
	VpcID string `mapstructure:"vpc_id"`
	// ID of the AWS account that owns the security group
	OwnerID string `mapstructure:"owner_id"`
	// Tags assigned to the security group
	Tags map[string]string `mapstructure:"tags"`
	// Raw JSON response from AWS API
	Raw string `mapstructure:"raw"`
}

// OutputSpec returns the HCL object spec for the output
func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

// Configure configures the datasource
func (d *Datasource) Configure(raws ...any) error {
	err := config.Decode(&d.config, nil, raws...)
	if err != nil {
		return err
	}

	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, d.config.AccessConfig.Prepare(&d.config.PackerConfig)...)

	// Validate that at least one search criterion is provided
	hasSearchCriteria := d.config.ID != "" ||
		d.config.Name != "" ||
		d.config.VpcID != "" ||
		len(d.config.Tags) > 0 ||
		len(d.config.Filters) > 0

	if !hasSearchCriteria {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("at least one search criterion must be provided (id, name, vpc_id, tags, or filters)"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

// Execute executes the datasource and returns security group information
func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	cfg, err := d.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	svc := ec2.NewFromConfig(*cfg)

	// Build filters from configuration
	filters := d.buildFilters()

	// Query security groups
	input := &ec2.DescribeSecurityGroupsInput{
		Filters: filters,
	}

	// If ID is specified, use it directly
	if d.config.ID != "" {
		input.GroupIds = []string{d.config.ID}
	}

	// If Name is specified without ID, use it as a filter (unless already added)
	if d.config.Name != "" && d.config.ID == "" {
		// Check if name filter already exists
		hasNameFilter := false
		for _, f := range filters {
			if f.Name != nil && *f.Name == "group-name" {
				hasNameFilter = true
				break
			}
		}
		if !hasNameFilter {
			filters = append(filters, types.Filter{
				Name:   aws.String("group-name"),
				Values: []string{d.config.Name},
			})
			input.Filters = filters
		}
	}

	resp, err := svc.DescribeSecurityGroups(ctx, input)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error describing security groups: %v", err)
	}

	if len(resp.SecurityGroups) == 0 {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("no security group found matching the specified criteria")
	}

	if len(resp.SecurityGroups) > 1 {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("multiple security groups matched the criteria (%d found); please refine your search to match a single security group", len(resp.SecurityGroups))
	}

	sg := resp.SecurityGroups[0]

	// Marshal the raw response
	raw, err := json.Marshal(sg)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error marshaling security group: %v", err)
	}

	// Extract tags into a map
	tags := make(map[string]string)
	for _, tag := range sg.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	// Build output
	output := DatasourceOutput{
		ID:          aws.ToString(sg.GroupId),
		ARN:         aws.ToString(sg.SecurityGroupArn),
		Name:        aws.ToString(sg.GroupName),
		Description: aws.ToString(sg.Description),
		VpcID:       aws.ToString(sg.VpcId),
		OwnerID:     aws.ToString(sg.OwnerId),
		Region:      cfg.Region,
		Tags:        tags,
		Raw:         string(raw),
	}

	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}

// buildFilters constructs EC2 filters from the datasource configuration
func (d *Datasource) buildFilters() []types.Filter {
	var filters []types.Filter

	// Add filters for each configured parameter
	if d.config.VpcID != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("vpc-id"),
			Values: []string{d.config.VpcID},
		})
	}

	// Add tag filters
	for key, value := range d.config.Tags {
		filters = append(filters, types.Filter{
			Name:   aws.String(fmt.Sprintf("tag:%s", key)),
			Values: []string{value},
		})
	}

	// Add custom filters
	for _, filter := range d.config.Filters {
		filters = append(filters, types.Filter{
			Name:   aws.String(filter.Name),
			Values: filter.Values,
		})
	}

	return filters
}
