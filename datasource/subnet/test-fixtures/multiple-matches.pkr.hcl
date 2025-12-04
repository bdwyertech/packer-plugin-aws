packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

# Select subnet with most free IPs when multiple match
data "aws-subnet" "most_free" {
  vpc_id    = "vpc-12345678"
  most_free = true
  region    = "us-east-1"

  filter {
    name   = "tag:Tier"
    values = ["private"]
  }
}

# Select random subnet when multiple match
data "aws-subnet" "random" {
  vpc_id = "vpc-12345678"
  random = true
  region = "us-east-1"

  filter {
    name   = "availability-zone"
    values = ["us-east-1a", "us-east-1b"]
  }
}

source "null" "basic" {
  communicator = "none"
}

build {
  sources = ["source.null.basic"]

  provisioner "shell-local" {
    inline = [
      "echo Most Free Subnet ID: ${data.aws-subnet.most_free.id}",
      "echo Most Free Subnet Available IPs: ${data.aws-subnet.most_free.available_ip_address_count}",
      "echo Random Subnet ID: ${data.aws-subnet.random.id}",
      "echo Random Subnet AZ: ${data.aws-subnet.random.availability_zone}",
    ]
  }
}
