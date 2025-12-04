# Packer Plugin for AWS

This plugin provides Packer components for working with AWS services, with a focus on AppStream 2.0 image building and AWS resource data sources.

### Installation

To install this plugin, copy and paste this code into your Packer configuration, then run [`packer init`](https://www.packer.io/docs/commands/init).

```hcl
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}
```

Alternatively, you can use `packer plugins install` to manage installation of this plugin.

```sh
$ packer plugins install github.com/bdwyertech/aws
```

### Components

#### Builders

- [appstream-image-builder](builders/appstream-image-builder.mdx) - Creates AWS AppStream 2.0 images by launching an Image Builder instance, provisioning it, and creating an image from it.

#### Data Sources

- [appstream-image](datasources/appstream-image.mdx) - Fetches information about existing AppStream 2.0 images. Supports exact name matching or regex patterns, with the ability to find the latest matching image.
- [appstream-image-builder](datasources/appstream-image-builder.mdx) - Fetches information about an existing AppStream Image Builder instance, including its IP address, ARN, and state.
- [security-group](datasources/security-group.mdx) - Fetches information about an AWS Security Group using various criteria like security group ID, name, VPC ID, tags, or custom filters.
- [subnet](datasources/subnet.mdx) - Fetches information about an AWS VPC subnet using various criteria. When multiple subnets match, you can select the one with the most free IPs or a random one.

#### Post-processors

- [appstream-share](post-processors/appstream-share.mdx) - Shares AppStream images with other AWS accounts and optionally copies them to additional regions.

