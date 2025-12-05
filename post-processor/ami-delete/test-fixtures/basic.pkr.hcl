packer {
  required_plugins {
    amazon = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/amazon"
    }
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  post-processor "aws-ami-delete" {
    region = "us-east-1"
  }
}
