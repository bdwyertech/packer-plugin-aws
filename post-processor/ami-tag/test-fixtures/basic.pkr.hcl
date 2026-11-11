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

variable "region" {
  default = "us-east-1"
}

data "amazon-ami" "al2023" {
  region = var.region
  filters = {
    name                = "al2023-ami-2023*-x86_64"
    root-device-type    = "ebs"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["amazon"]
}

source "amazon-ebs" "test" {
  region        = var.region
  source_ami    = data.amazon-ami.al2023.id
  instance_type = "t3.micro"
  ssh_username  = "ec2-user"
  ami_name      = "ami-tag-test-{{timestamp}}"
  tags = {
    Name = "ami-tag-acceptance-test"
  }
}

build {
  sources = ["source.amazon-ebs.test"]

  post-processor "aws-ami-tag" {
    tags = {
      "ec2:SharedTag/team"    = "platform"
      "ec2:SharedTag/purpose" = "acceptance-test"
      "test-key"              = "test-value"
    }
  }
}
