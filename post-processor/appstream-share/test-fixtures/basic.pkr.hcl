packer {
  required_plugins {
    appstream-share = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyer/packer-plugin-aws-appstream"
    }
  }
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  post-processor "appstream-share" {
    image_name  = "test-image-does-not-exist"
    account_ids = ["123456789012"]
    region      = "us-east-1"
    timeout     = "10s"
  }
}
