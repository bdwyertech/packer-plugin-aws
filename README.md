# Packer Plugin for AWS

This plugin provides Packer components for working with AWS services:
- **AppStream 2.0 Builder**: Create AppStream images using Image Builder instances
- **AppStream 2.0 Data Sources**: Query existing Images & Image Builder instances.
- **EC2 Data Sources**: Query VPC subnets and other EC2 resources
- **AppStream 2.0 Post-Processor**: Share and copy AppStream images across accounts and regions
- **AMI Post-Processor**: Delete AMIs and their associated EBS snapshots

## Components

### Builder: `appstream-image-builder`

Creates AWS AppStream 2.0 images by launching an Image Builder instance, provisioning it with your software and configurations, and then creating an image from it.

[Full Builder Documentation](docs/builders/appstream-image-builder.mdx)


### Data Source: `appstream-image`

Fetches information about an existing AppStream Image.

[Full Data Source Documentation](docs/datasources/appstream-image.mdx)


### Data Source: `appstream-image-builder`

Fetches information about an existing AppStream Image Builder instance, including its IP address, ARN, and state.

[Full Data Source Documentation](docs/datasources/appstream-image-builder.mdx)

### Data Source: `security-group`

Fetches information about an AWS Security Group using various criteria like security group ID, name, VPC ID, tags, or custom filters. Compatible with Terraform's `aws_security_group` data source interface.

[Full Data Source Documentation](docs/datasources/security-group.mdx)

### Data Source: `subnet`

Fetches information about an AWS VPC subnet using various criteria like subnet ID, VPC ID, CIDR block, tags, or custom filters. When multiple subnets match, you can select the one with the most free IPs (`most_free`) or a random one (`random`). Compatible with Terraform's `aws_subnet` data source interface.

[Full Data Source Documentation](docs/datasources/subnet.mdx)

### Post-Processor: `appstream-share`

Shares AppStream images with other AWS accounts and optionally copies them to additional regions.

[Full Post-Processor Documentation](docs/post-processors/appstream-share.mdx)

### Post-Processor: `ami-delete`

Deletes AWS AMIs and their associated EBS snapshots. Useful for cleanup operations or removing temporary AMIs created during multi-step build processes.

[Full Post-Processor Documentation](docs/post-processors/ami-delete.mdx)

## Installation

### Using `packer init` (Recommended)

Add the plugin to your Packer template:

```hcl
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}
```

Then run:
```bash
packer init .
```

### Manual Installation

1. **Build the plugin:**
   ```bash
   go build -o packer-plugin-aws .
   ```

2. **Install:**
   ```bash
   mkdir -p ~/.packer.d/plugins
   mv packer-plugin-aws ~/.packer.d/plugins/
   ```

## Quick Start Example

Here's a complete example that demonstrates all three components:

```hcl
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Query an existing Image Builder (optional)
data "aws-appstream-image" "base" {
  name_regex = "ORG-Windows2022-*"
  latest     = true
}

# Query a VPC subnet for network configuration
# If multiple match, use the one with most free IPs
data "aws-subnet" "this" {
  vpc_id    = "vpc-12345678"
  most_free = true
  region    = "us-east-1"
  
  filter {
    name   = "tag:Tier"
    values = ["private"]
  }
}

# Query a security group for network configuration
data "aws-security-group" "this" {
  name   = "appstream-builder-sg"
  region = data.aws-subnet.this.region

  filter {
    name   = "vpc-id"
    values = [data.aws-subnet.this.vpc_id]
  }
}

# Build a new AppStream image
source "aws-appstream-image-builder" "windows" {
  name                = "my-custom-appstream-image"
  builder_name        = "my-image-builder"
  source_image_name   = data.aws-appstream-image.base.name
  instance_type       = "stream.standard.medium"
  region              = "us-east-1"
  
  # Use the subnet and security group from the data sources
  subnet_ids          = [data.aws-subnet.this.id]
  security_group_ids  = [data.aws-security-group.this.id]
  
  tags = {
    Environment = "Production"
    Team        = "DevOps"
  }
  
  communicator        = "winrm"
  winrm_username      = "Administrator"
  winrm_use_ssl       = true
  winrm_insecure      = true
}

build {
  sources = ["source.appstream-image-builder.windows"]
  
  # Provision the image
  provisioner "powershell" {
    inline = [
      "Write-Host 'Installing custom software...'",
      "# Add your software installation commands here"
    ]
  }
  
  # Share the image with other accounts and regions
  post-processor "aws-appstream-share" {
    image_name          = "my-custom-appstream-image"
    account_ids         = ["123456789012", "987654321098"]
    region              = "us-east-1"
    destination_regions = ["us-west-2", "eu-central-1"]
    timeout             = "45m"
    allow_image_builder = true
  }
}
```

## Requirements

- Packer >= 1.7.0
- AWS credentials configured (via environment variables, shared credentials file, or IAM role)
- Appropriate AWS IAM permissions for AppStream 2.0 operations

## Development

### Building from Source

```bash
git clone https://github.com/bdwyertech/packer-plugin-aws.git
cd packer-plugin-aws
go build .
```

### Running Tests

```bash
go test ./...
```

## License

See LICENSE file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.
 