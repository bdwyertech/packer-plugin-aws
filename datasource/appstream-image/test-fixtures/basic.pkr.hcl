// TODO: This acceptance test fixture references a specific AppStream image
// builder ("packer-appstream-11-12-25") that is ephemeral and likely no longer
// exists. To make this pass, create a matching builder in the target account
// or parameterize the name via a variable/env var.
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
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
