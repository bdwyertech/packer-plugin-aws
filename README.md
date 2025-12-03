# Packer Plugin for AWS AppStream 2.0

This plugin provides a complete set of Packer components for working with AWS AppStream 2.0:
- **Builder**: Create AppStream images using Image Builder instances
- **Data Source**: Query existing Image Builder instances
- **Post-Processor**: Share and copy AppStream images across accounts and regions

## Components

### Builder: `appstream-image-builder`

Creates AWS AppStream 2.0 images by launching an Image Builder instance, provisioning it with your software and configurations, and then creating an image from it.

[Full Builder Documentation](docs/builders/appstream-image-builder.mdx)

### Data Source: `appstream-image-builder`

Fetches information about an existing AppStream Image Builder instance, including its IP address, ARN, and state.

[Full Data Source Documentation](docs/datasources/appstream-image-builder.mdx)

### Post-Processor: `appstream-share`

Shares AppStream images with other AWS accounts and optionally copies them to additional regions.

[Full Post-Processor Documentation](docs/post-processors/appstream-share.mdx)

## Installation

### Using `packer init` (Recommended)

Add the plugin to your Packer template:

```hcl
packer {
  required_plugins {
    aws-appstream = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyer/aws"
    }
  }
}
```

Then run:
```bash
packer init .
```

### Manual Installation

1. **Build the plugin:**
   ```bash
   go build -o packer-plugin-aws-appstream .
   ```

2. **Install:**
   ```bash
   mkdir -p ~/.packer.d/plugins
   mv packer-plugin-aws-appstream ~/.packer.d/plugins/
   ```

## Quick Start Example

Here's a complete example that demonstrates all three components:

```hcl
packer {
  required_plugins {
    aws-appstream = {
      version = ">= 0.0.1"
      source  = "github.com/bdwyer/aws"
    }
  }
}

# Query an existing Image Builder (optional)
data "appstream-image-builder" "existing" {
  name   = "my-existing-builder"
  region = "us-east-1"
}

# Build a new AppStream image
source "appstream-image-builder" "windows" {
  name                = "my-custom-appstream-image"
  builder_name        = "my-image-builder"
  source_image_name   = "AppStream-WinServer2019-12-05-2024"
  instance_type       = "stream.standard.medium"
  region              = "us-east-1"
  
  subnet_ids          = ["subnet-12345678"]
  security_group_ids  = ["sg-12345678"]
  
  tags = {
    Environment = "Production"
    Team        = "DevOps"
  }
  
  communicator        = "winrm"
  winrm_username      = "Administrator"
  winrm_use_ssl       = true
  winrm_insecure      = true
}

build {
  sources = ["source.appstream-image-builder.windows"]
  
  # Provision the image
  provisioner "powershell" {
    inline = [
      "Write-Host 'Installing custom software...'",
      "# Add your software installation commands here"
    ]
  }
  
  # Share the image with other accounts and regions
  post-processor "appstream-share" {
    image_name          = "my-custom-appstream-image"
    account_ids         = ["123456789012", "987654321098"]
    region              = "us-east-1"
    destination_regions = ["us-west-2", "eu-central-1"]
    timeout             = "45m"
    allow_image_builder = true
  }
}
```

## Requirements

- Packer >= 1.7.0
- AWS credentials configured (via environment variables, shared credentials file, or IAM role)
- Appropriate AWS IAM permissions for AppStream 2.0 operations

## Development

### Building from Source

```bash
git clone https://github.com/bdwyer/packer-plugin-aws-appstream.git
cd packer-plugin-aws-appstream
go build .
```

### Running Tests

```bash
go test ./...
```

## License

See LICENSE file for details.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.
