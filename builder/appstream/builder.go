//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package appstream

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	awscommon "github.com/hashicorp/packer-plugin-amazon/builder/common"
)

// The unique ID for this builder
const BuilderId = "bdwyertech.amazonappstream"

type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	awscommon.AccessConfig `mapstructure:",squash"`

	// Domain Join Configuration
	DirectoryName                       *string `mapstructure:"directory_name" required:"false"`
	OrganizationalUnitDistinguishedName *string `mapstructure:"organizational_unit_distinguished_name" required:"false"`

	// VPC Configuration
	SecurityGroupIds []string `mapstructure:"security_group_ids" required:"false"`
	SubnetIds        []string `mapstructure:"subnet_ids" required:"false"`

	// Volume Configuration
	VolumeSizeInGb *int32 `mapstructure:"volume_size_in_gb" required:"false"`

	// Communicator
	Comm communicator.Config `mapstructure:",squash"`

	// If true, Packer will not create the AppStream Image. Useful for setting to `true`
	// during a build test stage. Default `false`.
	SkipCreateImage bool `mapstructure:"skip_create_image" required:"false"`

	Name                        string `mapstructure:"name" required:"true"`
	Description                 string `mapstructure:"description" required:"false"`
	DisplayName                 string `mapstructure:"display_name" required:"false"`
	EnableDefaultInternetAccess bool   `mapstructure:"enable_default_internet_access" required:"false"`
	SourceImageName             string `mapstructure:"source_image_name" required:"true"`
	// SourceImageArn              *string `mapstructure:"source_image_arn" required:"false"`
	InstanceType string `mapstructure:"instance_type" required:"true"`
	IamRoleArn   string `mapstructure:"iam_role_arn" required:"false"`

	AppstreamAgentVersion string `mapstructure:"appstream_agent_version" required:"false"`

	SoftwaresToInstall   []string `mapstructure:"softwares_to_install" required:"false"`
	SoftwaresToUninstall []string `mapstructure:"softwares_to_uninstall" required:"false"`

	// Username string

	// AccessEndpoints []types.AccessEndpoint `mapstructure:"access_endpoints" required:"false"`

	Tags map[string]string `mapstructure:"tags" required:"false"`

	ctx interpolate.Context
}

type Builder struct {
	config Config
	runner multistep.Runner
}

var _ packersdk.Builder = new(Builder)

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...any) ([]string, []string, error) {
	b.config.ctx.Funcs = awscommon.TemplateFuncs
	err := config.Decode(&b.config, &config.DecodeOpts{
		PluginType:         BuilderId,
		Interpolate:        true,
		InterpolateContext: &b.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	if err != nil {
		return nil, nil, err
	}
	var errs *packersdk.MultiError
	var warns []string
	errs = packersdk.MultiErrorAppend(errs, b.config.AccessConfig.Prepare(&b.config.PackerConfig)...)

	if errs != nil && len(errs.Errors) != 0 {
		return nil, warns, errs
	}

	return nil, warns, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config, %v", err)
	}
	if b.config.RawRegion != "" {
		cfg.Region = b.config.RawRegion
	}

	svc := appstream.NewFromConfig(cfg)

	// Setup the state bag and initial state for the steps
	state := new(multistep.BasicStateBag)
	state.Put("config", &b.config)
	state.Put("hook", hook)
	state.Put("ui", ui)
	state.Put("appstreamv2", svc)
	state.Put("aws_config", cfg)
	state.Put("region", b.config.RawRegion)

	generatedData := &packerbuilderdata.GeneratedData{State: state}

	steps := []multistep.Step{
		&StepImageBuilderCreate{
			config: b.config,
		},
		&communicator.StepConnect{
			// StepConnect is provided settings for WinRM and SSH, but
			// the communicator will ultimately determine which port to use.
			Config:    &b.config.Comm,
			Host:      communicator.CommHost(b.config.Comm.Host(), "ip"),
			SSHConfig: b.config.Comm.SSHConfigFunc(),
		},
		&awscommon.StepSetGeneratedData{
			GeneratedData: generatedData,
		},
		// &commonsteps.StepProvision{},
	}

	// Run!
	b.runner = commonsteps.NewRunnerWithPauseFn(steps, b.config.PackerConfig, ui, state)
	b.runner.Run(ctx, state)
	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// If we were interrupted or cancelled, then just exit.
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("build was cancelled")
	}

	if _, ok := state.GetOk(multistep.StateHalted); ok {
		return nil, errors.New("build was halted")
	}

	// Build the artifact and return it
	artifact := &Artifact{
		Images:         state.Get("images").(map[string]string),
		BuilderIdValue: BuilderId,
		StateData:      map[string]any{"generated_data": state.Get("generated_data")},
		Config:         cfg,
	}

	return artifact, nil
}

type Artifact struct {
	// A map of regions to Image IDs.
	Images map[string]string

	// BuilderId is the unique ID for the builder that created this Image
	BuilderIdValue string

	// StateData should store data such as GeneratedData
	// to be shared with post-processors
	StateData map[string]any

	// EC2 connection for performing API stuff.
	Config aws.Config
}

func (a *Artifact) BuilderId() string {
	return a.BuilderIdValue
}

func (a *Artifact) Destroy() error {
	// TODO: Implement Destroy
	return nil
}

func (a *Artifact) Files() []string {
	// TODO: Implement Files
	return nil
}

func (a *Artifact) Id() string {
	// TODO: Implement Id
	return ""
}

func (a *Artifact) State(name string) any {
	if data, ok := a.StateData[name]; ok {
		return data
	}
	return nil
}

func (a *Artifact) String() string {
	imageStrings := make([]string, 0, len(a.Images))
	for region, id := range a.Images {
		single := fmt.Sprintf("%s: %s", region, id)
		imageStrings = append(imageStrings, single)
	}

	sort.Strings(imageStrings)
	return fmt.Sprintf("Images were created:\n%s\n", strings.Join(imageStrings, "\n"))
}
