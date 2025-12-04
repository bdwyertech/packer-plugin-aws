
## Configuration Reference

**Required (one of)**

- `name` (string) - The exact name of the AppStream image to query.

- `name_regex` (string) - A regular expression pattern to match image names. When used with `latest: true`, returns the most recently created matching image.

**Optional**

### Query Options

- `type` (string) - The type of image to query. Must be one of: `PUBLIC`, `PRIVATE`, or `SHARED`. If not specified, searches all types.

- `latest` (bool) - When using `name_regex`, set to `true` to return the most recently created image that matches the pattern. Default is `false`.

### AWS Configuration

- `access_key` (string) - AWS access key. If not specified, Packer will use the standard AWS credential chain.

- `secret_key` (string) - AWS secret key. If not specified, Packer will use the standard AWS credential chain.

- `region` (string) - AWS region where the image exists.

- `profile` (string) - AWS profile to use from your credentials file.

## Output

- `id` (string) - The ARN of the image.

- `arn` (string) - The ARN of the image.

- `name` (string) - The name of the image.

- `region` (string) - The AWS region where the image exists.

- `base_image_arn` (string) - The ARN of the base image that was used to create this image (if applicable).

- `created_time` (string) - The timestamp when the image was created (RFC822 format).

- `platform` (string) - The operating system platform of the image (e.g., `WINDOWS`, `WINDOWS_SERVER_2016`, `WINDOWS_SERVER_2019`).

- `visibility` (string) - The visibility of the image (`PUBLIC`, `PRIVATE`, or `SHARED`).

- `raw` (string) - The raw JSON representation of the image object from the AWS API.

## Example Usage

### Query a specific image by name

```hcl
packer {
  required_plugins {
    aws = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyertech/aws"
    }
  }
}

data "aws-appstream-image" "base" {
  name   = "AppStream-WinServer2019-12-05-2024"
  region = "us-east-1"
}

source "aws-appstream-image-builder" "windows" {
  name              = "my-custom-image"
  builder_name      = "my-builder"
  source_image_name = data.aws-appstream-image.base.name
  instance_type     = "stream.standard.medium"
  region            = "us-east-1"
}

build {
  sources = ["source.appstream-image-builder.windows"]
  
  provisioner "powershell" {
    inline = [
      "Write-Host 'Base Image: ${data.aws-appstream-image.base.name}'",
      "Write-Host 'Platform: ${data.aws-appstream-image.base.platform}'",
      "Write-Host 'Created: ${data.aws-appstream-image.base.created_time}'"
    ]
  }
}
```

### Find the latest image matching a pattern

```hcl
data "aws-appstream-image" "latest_windows" {
  name_regex = "AppStream-WinServer2019-.*"
  type       = "PUBLIC"
  latest     = true
  region     = "us-east-1"
}

source "aws-appstream-image-builder" "windows" {
  name              = "my-custom-image"
  builder_name      = "my-builder"
  source_image_name = data.aws-appstream-image.latest_windows.name
  instance_type     = "stream.standard.medium"
  region            = "us-east-1"
}

build {
  sources = ["source.appstream-image-builder.windows"]
  
  provisioner "powershell" {
    inline = [
      "Write-Host 'Using latest image: ${data.aws-appstream-image.latest_windows.name}'"
    ]
  }
}
```

## Notes

- This data source queries the AWS AppStream API to retrieve information about existing images.
- When using `name_regex` with `latest: true`, the data source will find all images matching the pattern and return the one with the most recent `created_time`.
- Only images in the `AVAILABLE` state are considered when using `name_regex`.
- The `raw` output contains the complete image object as JSON, which can be useful for debugging or accessing additional fields not exposed as dedicated outputs.
- You must specify either `name` or `name_regex`, but not both.
