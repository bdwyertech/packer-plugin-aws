//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput,Filter

package subnet

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"

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

// Filter represents a custom filter for subnet lookup
type Filter struct {
	// Name of the field to filter by, as defined by the AWS API
	Name string `mapstructure:"name" required:"true"`
	// Set of values that are accepted for the given field
	Values []string `mapstructure:"values" required:"true"`
}

// Config is the configuration structure for the subnet datasource
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// ID of the specific subnet to retrieve
	ID string `mapstructure:"id" required:"false"`
	// ID of the VPC that the desired subnet belongs to
	VpcID string `mapstructure:"vpc_id" required:"false"`
	// CIDR block of the desired subnet
	CidrBlock string `mapstructure:"cidr_block" required:"false"`
	// IPv6 CIDR block of the desired subnet
	IPv6CidrBlock string `mapstructure:"ipv6_cidr_block" required:"false"`
	// Availability zone where the subnet must reside
	AvailabilityZone string `mapstructure:"availability_zone" required:"false"`
	// ID of the Availability Zone for the subnet
	AvailabilityZoneID string `mapstructure:"availability_zone_id" required:"false"`
	// Whether the desired subnet must be the default subnet for its associated availability zone
	DefaultForAz *bool `mapstructure:"default_for_az" required:"false"`
	// State that the desired subnet must have
	State string `mapstructure:"state" required:"false"`
	// Map of tags, each pair of which must exactly match a pair on the desired subnet
	Tags map[string]string `mapstructure:"tags" required:"false"`
	// Custom filters for more complex queries
	Filters []Filter `mapstructure:"filter" required:"false"`
	// The Subnet with the most free IPv4 addresses will be used if multiple Subnets match the filter
	MostFree bool `mapstructure:"most_free" required:"false"`
	// A random Subnet will be used if multiple Subnets match the filter. most_free has precedence over this
	Random bool `mapstructure:"random" required:"false"`
}

// Datasource implements the subnet datasource
type Datasource struct {
	config Config
}

// ConfigSpec returns the HCL object spec for the config
func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

// DatasourceOutput contains all the output attributes for the subnet datasource
type DatasourceOutput struct {
	// ID of the subnet
	ID string `mapstructure:"id"`
	// ARN of the subnet
	ARN string `mapstructure:"arn"`
	// AWS region where the subnet is located
	Region string `mapstructure:"region"`
	// ID of the VPC the subnet belongs to
	VpcID string `mapstructure:"vpc_id"`
	// CIDR block of the subnet
	CidrBlock string `mapstructure:"cidr_block"`
	// IPv6 CIDR block of the subnet
	IPv6CidrBlock string `mapstructure:"ipv6_cidr_block"`
	// Availability zone of the subnet
	AvailabilityZone string `mapstructure:"availability_zone"`
	// ID of the availability zone
	AvailabilityZoneID string `mapstructure:"availability_zone_id"`
	// Number of available IP addresses in the subnet
	AvailableIPAddressCount int32 `mapstructure:"available_ip_address_count"`
	// Whether IPv6 addresses are assigned on creation
	AssignIPv6AddressOnCreation bool `mapstructure:"assign_ipv6_address_on_creation"`
	// Association ID of the IPv6 CIDR block
	IPv6CidrBlockAssociationID string `mapstructure:"ipv6_cidr_block_association_id"`
	// Whether this is an IPv6-only subnet
	IPv6Native bool `mapstructure:"ipv6_native"`
	// Whether public IP addresses are assigned on instance launch
	MapPublicIPOnLaunch bool `mapstructure:"map_public_ip_on_launch"`
	// Identifier of customer owned IPv4 address pool
	CustomerOwnedIPv4Pool string `mapstructure:"customer_owned_ipv4_pool"`
	// Whether customer owned IP addresses are assigned on network interface creation
	MapCustomerOwnedIPOnLaunch bool `mapstructure:"map_customer_owned_ip_on_launch"`
	// Whether DNS queries return synthetic IPv6 addresses for IPv4-only destinations
	EnableDNS64 bool `mapstructure:"enable_dns64"`
	// Whether to respond to DNS queries for instance hostnames with DNS AAAA records
	EnableResourceNameDNSAAAARecordOnLaunch bool `mapstructure:"enable_resource_name_dns_aaaa_record_on_launch"`
	// Whether to respond to DNS queries for instance hostnames with DNS A records
	EnableResourceNameDNSARecordOnLaunch bool `mapstructure:"enable_resource_name_dns_a_record_on_launch"`
	// Type of hostnames assigned to instances in the subnet at launch
	PrivateDNSHostnameTypeOnLaunch string `mapstructure:"private_dns_hostname_type_on_launch"`
	// Device position for local network interfaces in this subnet
	EnableLniAtDeviceIndex int32 `mapstructure:"enable_lni_at_device_index"`
	// Whether this is the default subnet for its availability zone
	DefaultForAz bool `mapstructure:"default_for_az"`
	// State of the subnet
	State string `mapstructure:"state"`
	// Tags assigned to the subnet
	Tags map[string]string `mapstructure:"tags"`
	// ID of the AWS account that owns the subnet
	OwnerID string `mapstructure:"owner_id"`
	// ARN of the Outpost
	OutpostARN string `mapstructure:"outpost_arn"`
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
		d.config.VpcID != "" ||
		d.config.CidrBlock != "" ||
		d.config.IPv6CidrBlock != "" ||
		d.config.AvailabilityZone != "" ||
		d.config.AvailabilityZoneID != "" ||
		d.config.DefaultForAz != nil ||
		d.config.State != "" ||
		len(d.config.Tags) > 0 ||
		len(d.config.Filters) > 0

	if !hasSearchCriteria {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("at least one search criterion must be provided (id, vpc_id, cidr_block, filters, tags, etc.)"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

// Execute executes the datasource and returns subnet information
func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	cfg, err := d.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("unable to load SDK config, %v", err)
	}

	svc := ec2.NewFromConfig(*cfg)

	// Build filters from configuration
	filters := d.buildFilters()

	// Query subnets
	input := &ec2.DescribeSubnetsInput{
		Filters: filters,
	}

	// If ID is specified, use it directly
	if d.config.ID != "" {
		input.SubnetIds = []string{d.config.ID}
	}

	resp, err := svc.DescribeSubnets(ctx, input)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error describing subnets: %v", err)
	}

	if len(resp.Subnets) == 0 {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("no subnet found matching the specified criteria")
	}

	// Handle multiple matches with most_free or random selection
	subnet := resp.Subnets[0]
	if len(resp.Subnets) > 1 {
		if d.config.MostFree {
			// Select subnet with most available IP addresses
			subnet = selectSubnetWithMostFreeIPs(resp.Subnets)
		} else if d.config.Random {
			// Select a random subnet
			subnet = selectRandomSubnet(resp.Subnets)
		} else {
			return cty.NullVal(cty.EmptyObject), fmt.Errorf("multiple subnets matched the criteria (%d found); please refine your search, use 'most_free = true', or use 'random = true' to select from multiple matches", len(resp.Subnets))
		}
	}

	// Marshal the raw response
	raw, err := json.Marshal(subnet)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error marshaling subnet: %v", err)
	}

	// Extract tags into a map
	tags := make(map[string]string)
	for _, tag := range subnet.Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	// Extract IPv6 CIDR block information
	ipv6CidrBlock := ""
	ipv6CidrBlockAssociationID := ""
	if len(subnet.Ipv6CidrBlockAssociationSet) > 0 {
		if subnet.Ipv6CidrBlockAssociationSet[0].Ipv6CidrBlock != nil {
			ipv6CidrBlock = *subnet.Ipv6CidrBlockAssociationSet[0].Ipv6CidrBlock
		}
		if subnet.Ipv6CidrBlockAssociationSet[0].AssociationId != nil {
			ipv6CidrBlockAssociationID = *subnet.Ipv6CidrBlockAssociationSet[0].AssociationId
		}
	}

	// Build output
	output := DatasourceOutput{
		ID:                                      aws.ToString(subnet.SubnetId),
		ARN:                                     aws.ToString(subnet.SubnetArn),
		Region:                                  cfg.Region,
		VpcID:                                   aws.ToString(subnet.VpcId),
		CidrBlock:                               aws.ToString(subnet.CidrBlock),
		IPv6CidrBlock:                           ipv6CidrBlock,
		AvailabilityZone:                        aws.ToString(subnet.AvailabilityZone),
		AvailabilityZoneID:                      aws.ToString(subnet.AvailabilityZoneId),
		AvailableIPAddressCount:                 aws.ToInt32(subnet.AvailableIpAddressCount),
		AssignIPv6AddressOnCreation:             aws.ToBool(subnet.AssignIpv6AddressOnCreation),
		IPv6CidrBlockAssociationID:              ipv6CidrBlockAssociationID,
		IPv6Native:                              aws.ToBool(subnet.Ipv6Native),
		MapPublicIPOnLaunch:                     aws.ToBool(subnet.MapPublicIpOnLaunch),
		CustomerOwnedIPv4Pool:                   aws.ToString(subnet.CustomerOwnedIpv4Pool),
		MapCustomerOwnedIPOnLaunch:              aws.ToBool(subnet.MapCustomerOwnedIpOnLaunch),
		EnableDNS64:                             aws.ToBool(subnet.EnableDns64),
		EnableResourceNameDNSAAAARecordOnLaunch: aws.ToBool(subnet.PrivateDnsNameOptionsOnLaunch.EnableResourceNameDnsAAAARecord),
		EnableResourceNameDNSARecordOnLaunch:    aws.ToBool(subnet.PrivateDnsNameOptionsOnLaunch.EnableResourceNameDnsARecord),
		PrivateDNSHostnameTypeOnLaunch:          string(subnet.PrivateDnsNameOptionsOnLaunch.HostnameType),
		EnableLniAtDeviceIndex:                  aws.ToInt32(subnet.EnableLniAtDeviceIndex),
		DefaultForAz:                            aws.ToBool(subnet.DefaultForAz),
		State:                                   string(subnet.State),
		Tags:                                    tags,
		OwnerID:                                 aws.ToString(subnet.OwnerId),
		OutpostARN:                              aws.ToString(subnet.OutpostArn),
		Raw:                                     string(raw),
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

	if d.config.CidrBlock != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("cidr-block"),
			Values: []string{d.config.CidrBlock},
		})
	}

	if d.config.IPv6CidrBlock != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("ipv6-cidr-block-association.ipv6-cidr-block"),
			Values: []string{d.config.IPv6CidrBlock},
		})
	}

	if d.config.AvailabilityZone != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("availability-zone"),
			Values: []string{d.config.AvailabilityZone},
		})
	}

	if d.config.AvailabilityZoneID != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("availability-zone-id"),
			Values: []string{d.config.AvailabilityZoneID},
		})
	}

	if d.config.DefaultForAz != nil {
		filters = append(filters, types.Filter{
			Name:   aws.String("default-for-az"),
			Values: []string{fmt.Sprintf("%t", *d.config.DefaultForAz)},
		})
	}

	if d.config.State != "" {
		filters = append(filters, types.Filter{
			Name:   aws.String("state"),
			Values: []string{d.config.State},
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

// selectSubnetWithMostFreeIPs selects the subnet with the most available IP addresses
func selectSubnetWithMostFreeIPs(subnets []types.Subnet) types.Subnet {
	if len(subnets) == 0 {
		return types.Subnet{}
	}

	// Sort subnets by available IP address count (descending)
	sort.Slice(subnets, func(i, j int) bool {
		iCount := int32(0)
		jCount := int32(0)
		if subnets[i].AvailableIpAddressCount != nil {
			iCount = *subnets[i].AvailableIpAddressCount
		}
		if subnets[j].AvailableIpAddressCount != nil {
			jCount = *subnets[j].AvailableIpAddressCount
		}
		return iCount > jCount
	})

	return subnets[0]
}

// selectRandomSubnet selects a random subnet from the list
func selectRandomSubnet(subnets []types.Subnet) types.Subnet {
	if len(subnets) == 0 {
		return types.Subnet{}
	}

	// Select a random index
	return subnets[rand.Intn(len(subnets))]
}
