// TODO: This acceptance test fixture uses hardcoded placeholder IDs
// (vpc-12345678) and tags that do not match any real security group.
// To make this pass, either replace them with real resource IDs/tags or use a
// dynamic lookup (e.g. the amazon-ami datasource pattern in ami-tag/test-fixtures/basic.pkr.hcl).
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Query a security group by tags
data "aws-security-group" "by_tags" {
  vpc_id = "vpc-12345678"
  tags = {
    Name        = "web-server-sg"
    Environment = "production"
    Team        = "platform"
  }
  region = "us-east-1"
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = [
      "echo Security Group ID: ${data.aws-security-group.by_tags.id}",
      "echo Security Group Name: ${data.aws-security-group.by_tags.name}",
    ]
  }
}
