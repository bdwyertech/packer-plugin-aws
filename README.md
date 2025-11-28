# Packer Plugin for AWS AppStream Sharing

This plugin provides a Packer post-processor to share AWS AppStream images with other AWS accounts.

## Features

-   Waits for an AppStream image to become `AVAILABLE`.
-   Shares the image with a list of AWS Account IDs.
-   Configurable timeout.

## Installation

1.  **Build the plugin:**
    ```bash
    go build .
    ```

2.  **Install:**
    Move the binary `packer-plugin-aws-appstream` to your Packer plugins directory (e.g., `~/.packer.d/plugins/`).

## Usage

Add the post-processor to your `packer.pkr.hcl`:

```hcl
packer {
  required_plugins {
    appstream-share = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyer/packer-plugin-aws-appstream"
    }
  }
}

build {
  sources = ["..."]

  post-processor "appstream-share" {
    image_name          = "my-appstream-image"
    account_ids         = ["123456789012", "987654321098"]
    region              = "us-east-1"
    destination_regions = ["us-west-2", "eu-central-1"]
    timeout             = "45m"
  }
}
```

## Configuration

-   `image_name` (string, required): The name of the AppStream image to share.
-   `account_ids` (list of strings, required): List of AWS Account IDs to share with.
-   `region` (string, optional): AWS Region. Defaults to environment or shared config.
-   `destination_regions` (list of strings, optional): List of AWS Regions to copy the image to and share.
-   `timeout` (string, optional): Timeout for waiting for the image. Default: `30m`.
