//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,DatasourceOutput

package imagebuilder

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/appstream"
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
	Name string `mapstructure:"name" required:"true"`
}

type Datasource struct {
	config Config
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return d.config.FlatMapstructure().HCL2Spec()
}

type DatasourceOutput struct {
	ID        string `mapstructure:"id"`
	ARN       string `mapstructure:"arn"`
	State     string `mapstructure:"state"`
	IPAddress string `mapstructure:"ip_address"`
	Raw       string `mapstructure:"raw"`
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

	if d.config.Name == "" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("a 'name' must be provided"))
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

	resp, err := svc.DescribeImageBuilders(ctx, &appstream.DescribeImageBuildersInput{
		Names: []string{d.config.Name},
	})

	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error describing image builder: %v", err)
	}

	if len(resp.ImageBuilders) == 0 {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("image builder %s not found", d.config.Name)
	}

	builder := resp.ImageBuilders[0]

	ipAddress := ""
	if builder.NetworkAccessConfiguration != nil && builder.NetworkAccessConfiguration.EniPrivateIpAddress != nil {
		ipAddress = *builder.NetworkAccessConfiguration.EniPrivateIpAddress
	}

	raw, err := json.Marshal(builder)
	if err != nil {
		return cty.NullVal(cty.EmptyObject), fmt.Errorf("error marshaling image builder: %v", err)
	}

	output := DatasourceOutput{
		ID:        d.config.Name,
		ARN:       *builder.Arn,
		State:     string(builder.State),
		IPAddress: ipAddress,
		Raw:       string(raw),
	}

	return hcl2helper.HCL2ValueFromConfig(output, d.OutputSpec()), nil
}
