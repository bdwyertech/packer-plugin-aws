packer {
  required_plugins {
    appstream-share = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws-appstream"
    }
  }
}

data "aws-appstream-image-builder" "basic" {
  // name = "test-builder-does-not-exist"
  name = "packer-appstream-11-12-25"
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = ["echo Builder IP: ${data.aws-appstream-image-builder.basic.ip_address}"]
  }

  provisioner "shell-local" {
    inline = ["echo Builder State: ${data.aws-appstream-image-builder.basic.raw}"]
  }
}
