
## Configuration Reference

**Optional (at least one required)**

### Direct Lookup Parameters

- `id` (string) - ID of the specific subnet to retrieve.

- `vpc_id` (string) - ID of the VPC that the desired subnet belongs to.

- `cidr_block` (string) - CIDR block of the desired subnet.

- `ipv6_cidr_block` (string) - IPv6 CIDR block of the desired subnet.

- `availability_zone` (string) - Availability zone where the subnet must reside.

- `availability_zone_id` (string) - ID of the Availability Zone for the subnet. This argument is not supported in all regions or partitions.

- `default_for_az` (bool) - Whether the desired subnet must be the default subnet for its associated availability zone.

- `state` (string) - State that the desired subnet must have (e.g., `available`, `pending`).

- `tags` (map[string]string) - Map of tags, each pair of which must exactly match a pair on the desired subnet.

### Multiple Match Handling

- `most_free` (bool) - If multiple subnets match the filter criteria, the subnet with the most free IPv4 addresses will be selected. This takes precedence over `random`.

- `random` (bool) - If multiple subnets match the filter criteria, a random subnet will be selected. The `most_free` option takes precedence over this.

### Filter Block

- `filter` (block) - One or more custom filters for complex queries. Each filter block supports:
  - `name` (string, required) - Name of the field to filter by, as defined by the AWS EC2 API.
  - `values` (list of strings, required) - Set of values that are accepted for the given field. A subnet will be selected if any one of the given values matches.

### AWS Configuration

- `access_key` (string) - AWS access key. If not specified, Packer will use the standard AWS credential chain.

- `secret_key` (string) - AWS secret key. If not specified, Packer will use the standard AWS credential chain.

- `region` (string) - AWS region where the subnet exists.

- `profile` (string) - AWS profile to use from your credentials file.

## Output Attributes

- `id` (string) - ID of the subnet.

- `arn` (string) - ARN of the subnet.

- `vpc_id` (string) - ID of the VPC the subnet belongs to.

- `cidr_block` (string) - CIDR block of the subnet.

- `ipv6_cidr_block` (string) - IPv6 CIDR block of the subnet.

- `availability_zone` (string) - Availability zone of the subnet.

- `availability_zone_id` (string) - ID of the availability zone.

- `available_ip_address_count` (int64) - Number of available IP addresses in the subnet.

- `assign_ipv6_address_on_creation` (bool) - Whether IPv6 addresses are assigned on creation.

- `ipv6_cidr_block_association_id` (string) - Association ID of the IPv6 CIDR block.

- `ipv6_native` (bool) - Whether this is an IPv6-only subnet.

- `map_public_ip_on_launch` (bool) - Whether public IP addresses are assigned on instance launch.

- `customer_owned_ipv4_pool` (string) - Identifier of customer owned IPv4 address pool.

- `map_customer_owned_ip_on_launch` (bool) - Whether customer owned IP addresses are assigned on network interface creation.

- `enable_dns64` (bool) - Whether DNS queries return synthetic IPv6 addresses for IPv4-only destinations.

- `enable_resource_name_dns_aaaa_record_on_launch` (bool) - Whether to respond to DNS queries for instance hostnames with DNS AAAA records.

- `enable_resource_name_dns_a_record_on_launch` (bool) - Whether to respond to DNS queries for instance hostnames with DNS A records.

- `private_dns_hostname_type_on_launch` (string) - Type of hostnames assigned to instances in the subnet at launch.

- `enable_lni_at_device_index` (int32) - Device position for local network interfaces in this subnet.

- `default_for_az` (bool) - Whether this is the default subnet for its availability zone.

- `state` (string) - State of the subnet.

- `tags` (map[string]string) - Tags assigned to the subnet.

- `owner_id` (string) - ID of the AWS account that owns the subnet.

- `outpost_arn` (string) - ARN of the Outpost.

- `raw` (string) - Raw JSON response from AWS API.

## Example Usage

### Query by Subnet ID

```hcl
data "aws-subnet" "by_id" {
  id     = "subnet-12345678"
  region = "us-east-1"
}
```

### Query by VPC ID and CIDR Block

```hcl
data "aws-subnet" "private" {
  vpc_id     = "vpc-12345678"
  cidr_block = "10.0.1.0/24"
  region     = "us-east-1"
}
```

### Query by Tags

```hcl
data "aws-subnet" "app_subnet" {
  vpc_id = "vpc-12345678"
  tags = {
    Name        = "app-subnet"
    Environment = "production"
    Tier        = "private"
  }
  region = "us-east-1"
}
```

### Query Using Custom Filters

```hcl
data "aws-subnet" "filtered" {
  region = "us-east-1"
  
  filter {
    name   = "vpc-id"
    values = ["vpc-12345678"]
  }
  
  filter {
    name   = "availability-zone"
    values = ["us-east-1a", "us-east-1b"]
  }
  
  filter {
    name   = "state"
    values = ["available"]
  }
}
```

### Query Default Subnet for Availability Zone

```hcl
data "aws-subnet" "default" {
  default_for_az    = true
  availability_zone = "us-east-1a"
  region            = "us-east-1"
}
```

### Select Subnet with Most Free IPs

```hcl
# When multiple subnets match, select the one with the most available IPs
data "aws-subnet" "least_used" {
  vpc_id    = "vpc-12345678"
  most_free = true
  region    = "us-east-1"
  
  filter {
    name   = "tag:Tier"
    values = ["private"]
  }
}
```

### Select Random Subnet from Matches

```hcl
# When multiple subnets match, select a random one for load distribution
data "aws-subnet" "random_subnet" {
  vpc_id = "vpc-12345678"
  random = true
  region = "us-east-1"
  
  filter {
    name   = "availability-zone"
    values = ["us-east-1a", "us-east-1b", "us-east-1c"]
  }
}
```

### Using Subnet Data in a Build

```hcl
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

data "aws-subnet" "build_subnet" {
  tags = {
    Name = "packer-build-subnet"
  }
  region = "us-east-1"
}

source "amazon-ebs" "example" {
  ami_name      = "packer-example-{{timestamp}}"
  instance_type = "t2.micro"
  region        = "us-east-1"
  source_ami_filter {
    filters = {
      name                = "ubuntu/images/*ubuntu-jammy-22.04-amd64-server-*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["099720109477"]
  }
  ssh_username = "ubuntu"
  
  # Use the subnet from the data source
  subnet_id = data.aws-subnet.build_subnet.id
}

build {
  sources = ["source.amazon-ebs.example"]
  
  provisioner "shell-local" {
    inline = [
      "echo 'Building in subnet: ${data.aws-subnet.build_subnet.id}'",
      "echo 'Subnet CIDR: ${data.aws-subnet.build_subnet.cidr_block}'",
      "echo 'Availability Zone: ${data.aws-subnet.build_subnet.availability_zone}'"
    ]
  }
}
```

## Notes

- At least one search criterion must be provided.
- If multiple subnets match the specified criteria without `most_free` or `random` set, the data source will return an error asking you to refine your search.
- When `most_free = true`, the subnet with the highest number of available IP addresses will be selected from multiple matches.
- When `random = true`, a random subnet will be selected from multiple matches, useful for distributing load across subnets.
- The `most_free` option takes precedence over `random` if both are set to true.
- The `filter` block allows for complex queries using any field supported by the AWS EC2 DescribeSubnets API.
- For a complete list of available filter names, see the [AWS EC2 DescribeSubnets API documentation](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeSubnets.html).
- This data source is compatible with Terraform's `aws_subnet` data source and extends it with Packer's `most_free` and `random` subnet selection features.
