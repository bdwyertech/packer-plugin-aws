packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Query a security group by ID
data "aws-security-group" "by_id" {
  id     = "sg-12345678"
  region = "us-east-1"
}

# Query a security group by name
data "aws-security-group" "by_name" {
  name   = "default"
  region = "us-east-1"
}

# Query a security group by VPC ID and name
data "aws-security-group" "by_vpc_name" {
  vpc_id = "vpc-12345678"
  name   = "my-security-group"
  region = "us-east-1"
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = [
      "echo Security Group ID: ${data.aws-security-group.by_id.id}",
      "echo Security Group Name: ${data.aws-security-group.by_id.name}",
      "echo Security Group Description: ${data.aws-security-group.by_id.description}",
      "echo Security Group ARN: ${data.aws-security-group.by_id.arn}",
      "echo VPC ID: ${data.aws-security-group.by_id.vpc_id}",
    ]
  }
}
