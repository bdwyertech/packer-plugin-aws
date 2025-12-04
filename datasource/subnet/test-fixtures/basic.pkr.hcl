packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Query a subnet by ID
data "aws-subnet" "by_id" {
  id     = "subnet-12345678"
  region = "us-east-1"
}

# Query a subnet by VPC ID and CIDR block
data "aws-subnet" "by_vpc_cidr" {
  vpc_id     = "vpc-12345678"
  cidr_block = "10.0.1.0/24"
  region     = "us-east-1"
}

# Query a subnet by tags
data "aws-subnet" "by_tags" {
  vpc_id = "vpc-12345678"
  tags = {
    Name        = "my-subnet"
    Environment = "production"
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
      "echo Subnet ID: ${data.aws-subnet.by_id.id}",
      "echo Subnet CIDR: ${data.aws-subnet.by_id.cidr_block}",
      "echo Subnet AZ: ${data.aws-subnet.by_id.availability_zone}",
      "echo VPC ID: ${data.aws-subnet.by_id.vpc_id}",
    ]
  }
}