packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Query a security group using custom filters
data "aws-security-group" "filtered" {
  region = "us-east-1"

  filter {
    name   = "vpc-id"
    values = ["vpc-12345678"]
  }

  filter {
    name   = "group-name"
    values = ["web-server-sg"]
  }
}

# Query default security group
data "aws-security-group" "default" {
  vpc_id = "vpc-12345678"
  name   = "default"
  region = "us-east-1"
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = [
      "echo Filtered Security Group ID: ${data.aws-security-group.filtered.id}",
      "echo Default Security Group ID: ${data.aws-security-group.default.id}",
    ]
  }
}
