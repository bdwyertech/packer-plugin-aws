packer {
  required_plugins {
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

  post-processor "aws-appstream-share" {
    image_name  = "test-image-does-not-exist"
    account_ids = ["123456789012"]
    region      = "us-east-1"
    timeout     = "10s"
  }
}
