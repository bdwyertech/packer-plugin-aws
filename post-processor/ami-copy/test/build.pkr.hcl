packer {
  required_plugins {
    amazon = {
      version = ">= 1.2.8"
      source  = "github.com/hashicorp/amazon"
    }
  }
}

variable "Profile" {
  type    = string
  default = "bfs21_tss_amifactory_pro"
}

variable "Region" {
  type    = string
  default = env("AWS_REGION")
}

data "amazon-ami" "el9" {
  decode_authorization_messages = true
  filters = {
    architecture        = "x86_64"
    is-public           = "false"
    name                = "csg-rocky-9.*"
    state               = "available"
    "tag:BuildID"       = "*"
    virtualization-type = "hvm"
  }
  most_recent = true
  owners      = ["self"]
  profile     = var.Profile
  region      = var.Region
}
# The "legacy_isotime" function has been provided for backwards compatability, but we recommend switching to the timestamp and formatdate functions.

locals {
  BuildID   = uuidv4()
  BuildTime = legacy_isotime("02-Jan-2006__1504MST")
  Name      = "packer-test-el-9.7-${legacy_isotime("0601.02.1504")}"
}

source "amazon-ebs" "el9" {
  profile = var.Profile
  region  = var.Region

  vpc_filter {
    filters = {
      "tag:Name" = "br_*_vpc"
    }
  }
  subnet_filter {
    filters = {
      "tag:Name" = "br_*_app_sbt"
    }
    most_free = true
    random    = false
  }
  security_group_filter {
    filters = {
      "tag:Name" = "TestKitchen"
    }
  }

  ami_description               = "Packer Test - DELETEME!!!"
  ami_name                      = local.Name
  associate_public_ip_address   = false
  decode_authorization_messages = true
  ena_support                   = true
  encrypt_boot                  = false
  iam_instance_profile          = "BR_SSM_Role"
  instance_type                 = "t3.medium"
  source_ami                    = data.amazon-ami.el9.id
  sriov_support                 = true
  ssh_keep_alive_interval       = "15s"
  ssh_username                  = "ec2-user"
  temporary_key_pair_name       = "packer_${local.Name}_${local.BuildID}"

  // IMDSv2 Only
  imds_support = "v2.0"
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  // Deprecate 100 days post-creation
  deprecate_at = timeadd(timestamp(), "2400h")

  tags = {
    BuildRegion   = "{{ .BuildRegion }}"
    ChargeCode    = "DVADM-5225-550-00130-000-XC AWS CSG"
    Name          = "${local.Name}"
    SBU           = "CSG"
    SELinux       = true
    SourceAMI     = "{{ .SourceAMI }}"
    SourceAMIName = "{{ .SourceAMIName }}"
    RootAMI       = "{{ .SourceAMITags.SourceAMI }}"
    RootAMIName   = "{{ .SourceAMITags.SourceAMIName }}"
  }
}

build {
  sources = ["source.amazon-ebs.el9"]

  post-processor "manifest" {
    strip_path = true
    custom_data = {
      BuildID   = local.BuildID
      BuildTime = local.BuildTime
      Name      = local.Name
    }
  }

  post-processor "aws-ami-copy" {
    profile = var.Profile
    region  = var.Region
    targets {
      profile = "tss_dev"
    }
    targets {
      profile = "shared_services"
    }
    targets {
      profile = "tss_dev"
      region  = "us-west-1"
    }

    encrypt_boot     = true
    ensure_available = true
    keep_artifact    = false

    copy_duration_minutes = 15
    copy_timeout_minutes  = 30
  }

  post-processor "aws-ami-delete" {
    profile = var.Profile
    // region  = var.Region
  }
}
