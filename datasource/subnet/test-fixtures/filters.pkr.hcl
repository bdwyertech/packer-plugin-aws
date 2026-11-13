// TODO: This acceptance test fixture uses hardcoded placeholder IDs
// (vpc-12345678) that do not exist in any real AWS account.
// To make this pass, either replace them with real resource IDs or use a
// dynamic lookup (e.g. the amazon-ami datasource pattern in ami-tag/test-fixtures/basic.pkr.hcl).
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Query a subnet using custom filters
data "aws-subnet" "with_filters" {
  region = "us-east-1"

  filter {
    name   = "vpc-id"
    values = ["vpc-12345678"]
  }

  filter {
    name   = "availability-zone"
    values = ["us-east-1a"]
  }

  filter {
    name   = "state"
    values = ["available"]
  }
}

# Query the default subnet for an availability zone
data "aws-subnet" "default_subnet" {
  default_for_az    = true
  availability_zone = "us-east-1a"
  region            = "us-east-1"
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = [
      "echo Filtered Subnet ID: ${data.aws-subnet.with_filters.id}",
      "echo Default Subnet ID: ${data.aws-subnet.default_subnet.id}",
      "echo Default Subnet CIDR: ${data.aws-subnet.default_subnet.cidr_block}",
    ]
  }
}
