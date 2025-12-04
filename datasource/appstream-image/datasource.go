//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput

package image

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/aws/aws-sdk-go-v2/service/appstream/types"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/hcl2helper"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/zclconf/go-cty/cty"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	// The name of the image-builder you want to query.
	Name string `mapstructure:"name" required:"false"`
	// The regular expression name of the image-builder you want to query.
	NameRegex string `mapstructure:"name_regex" required:"false"`
	// The type of image which must be (PUBLIC, PRIVATE, or SHARED).
	Type   string `mapstructure:"type" required:"false"`
	Latest bool   `mapstructure:"latest" required:"false"`
}

type Datasource struct {
	config Config
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

type DatasourceOutput struct {
	ID           string  `mapstructure:"id"`
	ARN          string  `mapstructure:"arn"`
	Name         string  `mapstructure:"name"`
	Region       string  `mapstructure:"region"`
	BaseImageArn *string `mapstructure:"base_image_arn"`
	CreatedTime  string  `mapstructure:"created_time"`
	Platform     string  `mapstructure:"platform"`
	Visibility   string  `mapstructure:"visibility"`
	Raw          string  `mapstructure:"raw"`
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return (&DatasourceOutput{}).FlatMapstructure().HCL2Spec()
}

func (d *Datasource) Configure(raws ...any) error {
	err := config.Decode(&d.config, nil, raws...)
	var errs *packersdk.MultiError
	errs = packersdk.MultiErrorAppend(errs, d.config.AccessConfig.Prepare(&d.config.PackerConfig)...)
	if err != nil {
		return err
	}

	if d.config.Name == "" && d.config.NameRegex == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("a 'name' or 'name_regex' must be provided"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	cfg, err := d.config.AccessConfig.GetAWSConfig(ctx)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), err
	}

	svc := appstream.NewFromConfig(*cfg)

	// If latest is set, get the latest image
	if d.config.NameRegex != "" {
		var images []types.Image
		paginator := appstream.NewDescribeImagesPaginator(svc, &appstream.DescribeImagesInput{Type: types.VisibilityType(d.config.Type)})
		for paginator.HasMorePages() {
			resp, err := paginator.NextPage(ctx)
			if err != nil {
				return cty.NullVal(cty.EmptyObject), fmt.Errorf("error describing images: %v", err)
			}
			images = append(images, resp.Images...)
		}

		var matchingImages []types.Image
		re := regexp.MustCompile(d.config.NameRegex)
		for _, image := range images {
			if image.State != types.ImageStateAvailable {
				continue
			}
			if re.MatchString(*image.Name) {
				matchingImages = append(matchingImages, image)
			}
		}

		if len(matchingImages) == 0 {
			return cty.NullVal(cty.EmptyObject), fmt.Errorf("no image matching %s found", d.config.NameRegex)
		}

		// Find the latest image
		latestImage := matchingImages[0]
		if d.config.Latest {
			for _, image := range matchingImages {
				if image.CreatedTime.After(*latestImage.CreatedTime) {
					latestImage = image
				}
			}
		}

		raw, err := json.Marshal(latestImage)
		if err != nil {
			return cty.NullVal(cty.EmptyObject), fmt.Errorf("error marshaling image: %v", err)
		}

		output := &DatasourceOutput{
			ID:           *latestImage.Arn,
			ARN:          *latestImage.Arn,
			Name:         *latestImage.Name,
			Region:       cfg.Region,
			BaseImageArn: latestImage.BaseImageArn,
			Platform:     string(latestImage.Platform),
			Visibility:   string(latestImage.Visibility),
			CreatedTime:  (*latestImage.CreatedTime).Format(time.RFC822),
			Raw:          string(raw),
		}

		return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
	}

	resp, err := svc.DescribeImages(ctx, &appstream.DescribeImagesInput{Names: []string{d.config.Name}})

	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error describing images: %v", err)
	}

	if len(resp.Images) == 0 {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("image %s not found", d.config.Name)
	}

	raw, err := json.Marshal(resp.Images[0])
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error marshaling image: %v", err)
	}

	output := DatasourceOutput{
		ID:           *resp.Images[0].Arn,
		ARN:          *resp.Images[0].Arn,
		Region:       cfg.Region,
		Name:         *resp.Images[0].Name,
		BaseImageArn: resp.Images[0].BaseImageArn,
		Platform:     string(resp.Images[0].Platform),
		Visibility:   string(resp.Images[0].Visibility),
		CreatedTime:  resp.Images[0].CreatedTime.Format(time.RFC822),
		Raw:          string(raw),
	}

	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
