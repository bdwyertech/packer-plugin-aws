//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package imagebuilder

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/zclconf/go-cty/cty"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`
	Name                   string `mapstructure:"name"`
}

type Datasource struct {
	config Config
}

func (d *Datasource) ConfigSpec() hcldec.ObjectSpec {
	return hcldec.ObjectSpec{
		"name": &hcldec.AttrSpec{
			Name:     "name",
			Type:     cty.String,
			Required: true,
		},
		"region": &hcldec.AttrSpec{
			Name:     "region",
			Type:     cty.String,
			Required: false,
		},
	}
}

func (d *Datasource) Configure(raws ...interface{}) error {
	err := config.Decode(&d.config, &config.DecodeOpts{
		PluginType:  "appstream-image-builder",
		Interpolate: true,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	if err != nil {
		return err
	}

	if d.config.Name == "" {
		return fmt.Errorf("name is required")
	}

	return nil
}

func (d *Datasource) Execute() (cty.Value, error) {
	ctx := context.TODO()
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return cty.NilVal, fmt.Errorf("unable to load SDK config, %v", err)
	}
	if d.config.RawRegion != "" {
		cfg.Region = d.config.RawRegion
	}

	svc := appstream.NewFromConfig(cfg)

	resp, err := svc.DescribeImageBuilders(ctx, &appstream.DescribeImageBuildersInput{
		Names: []string{d.config.Name},
	})
	if err != nil {
		return cty.NilVal, fmt.Errorf("error describing image builder: %v", err)
	}

	if len(resp.ImageBuilders) == 0 {
		return cty.NilVal, fmt.Errorf("image builder %s not found", d.config.Name)
	}

	builder := resp.ImageBuilders[0]

	ipAddress := ""
	if builder.NetworkAccessConfiguration != nil && builder.NetworkAccessConfiguration.EniPrivateIpAddress != nil {
		ipAddress = *builder.NetworkAccessConfiguration.EniPrivateIpAddress
	}

	return cty.ObjectVal(map[string]cty.Value{
		"id":         cty.StringVal(d.config.Name),
		"arn":        cty.StringVal(*builder.Arn),
		"state":      cty.StringVal(string(builder.State)),
		"ip_address": cty.StringVal(ipAddress),
	}), nil
}

func (d *Datasource) OutputSpec() hcldec.ObjectSpec {
	return hcldec.ObjectSpec{
		"id": &hcldec.AttrSpec{
			Name: "id",
			Type: cty.String,
		},
		"arn": &hcldec.AttrSpec{
			Name: "arn",
			Type: cty.String,
		},
		"state": &hcldec.AttrSpec{
			Name: "state",
			Type: cty.String,
		},
		"ip_address": &hcldec.AttrSpec{
			Name: "ip_address",
			Type: cty.String,
		},
	}
}
