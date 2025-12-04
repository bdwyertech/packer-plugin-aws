package appstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/aws/aws-sdk-go-v2/service/appstream/types"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepImageBuilderCreate struct {
	config Config
	name   string
}

var _ multistep.Step = new(StepImageBuilderCreate)

func (s *StepImageBuilderCreate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	svc, ok := state.Get("appstreamv2").(*appstream.Client)
	if !ok {
		state.Put("error", fmt.Errorf("appstreamv2 client not found"))
		return multistep.ActionHalt
	}
	ui, ok := state.Get("ui").(packersdk.Ui)
	if !ok {
		state.Put("error", fmt.Errorf("ui not found"))
		return multistep.ActionHalt
	}

	ui.Say("Launching an AppStream ImageBuilder...")

	out, err := svc.CreateImageBuilder(ctx, &appstream.CreateImageBuilderInput{
		Name:                        &s.config.BuilderName,
		Description:                 &s.config.Description,
		DisplayName:                 &s.config.DisplayName,
		InstanceType:                &s.config.InstanceType,
		IamRoleArn:                  &s.config.IamRoleArn,
		ImageName:                   &s.config.SourceImageName,
		EnableDefaultInternetAccess: &s.config.EnableDefaultInternetAccess,
		AppstreamAgentVersion:       &s.config.AppstreamAgentVersion,
		DomainJoinInfo: &types.DomainJoinInfo{
			DirectoryName:                       s.config.DirectoryName,
			OrganizationalUnitDistinguishedName: s.config.OrganizationalUnitDistinguishedName,
		},
		VpcConfig: &types.VpcConfig{
			SecurityGroupIds: s.config.SecurityGroupIds,
			SubnetIds:        s.config.SubnetIds,
		},
		Tags:                 s.config.BuilderTags,
		SoftwaresToInstall:   s.config.SoftwaresToInstall,
		SoftwaresToUninstall: s.config.SoftwaresToUninstall,
	})
	if err != nil {
		state.Put("error", err)
		return multistep.ActionHalt
	}

	builder := out.ImageBuilder

	s.name = *builder.Name

	// Wait for image to become available
	var elapsed time.Duration
	for {
		status, err := svc.DescribeImageBuilders(ctx, &appstream.DescribeImageBuildersInput{
			Names: []string{s.name},
		})
		if err != nil {
			state.Put("error", err)
			return multistep.ActionHalt
		}

		if len(status.ImageBuilders) == 0 {
			state.Put("error", fmt.Errorf("image builder not found"))
			return multistep.ActionHalt
		}

		imageBuilder := status.ImageBuilders[0]

		switch imageBuilder.State {
		case types.ImageBuilderStateRunning:
			builder = &imageBuilder
		case types.ImageBuilderStatePending:
			ui.Say(fmt.Sprintf("Waiting for ImageBuilder (%s) to become available (elapsed: %s)", s.name, elapsed))
			wait := 5 * time.Second
			elapsed += wait
			time.Sleep(wait)
			continue
		default:
			state.Put("error", fmt.Errorf("bad imagebuilder state: %s", imageBuilder.State))
			return multistep.ActionHalt
		}
		break
	}
	state.Put("image_builder", builder)
	if builder.NetworkAccessConfiguration != nil && builder.NetworkAccessConfiguration.EniPrivateIpAddress != nil {
		state.Put("ip", *builder.NetworkAccessConfiguration.EniPrivateIpAddress)
	} else {
		state.Put("error", errors.New("failed to fetch address for ImageBuilder"))
		return multistep.ActionHalt
	}

	ui.Say(fmt.Sprintf("ImageBuilder has IP: %s.", state.Get("ip")))

	return multistep.ActionContinue
}

func (s *StepImageBuilderCreate) Cleanup(state multistep.StateBag) {
	svc := state.Get("appstreamv2").(*appstream.Client)
	ui := state.Get("ui").(packersdk.Ui)
	ctx := context.TODO()

	if s.name == "" {
		// Never created -- nothing to do
		return
	}

	// We need to first wait for the image builder to be in a stoppable state
	var elapsed time.Duration
	for {
		status, err := svc.DescribeImageBuilders(ctx, &appstream.DescribeImageBuildersInput{
			Names: []string{s.name},
		})
		if err != nil {
			ui.Error(fmt.Sprintf("Error describing ImageBuilder during cleanup: %s", err))
			return
		}

		if len(status.ImageBuilders) == 0 {
			ui.Say("ImageBuilder already terminated")
			return
		}

		imageBuilder := status.ImageBuilders[0]

		switch imageBuilder.State {
		case types.ImageBuilderStateStopped, types.ImageBuilderStateFailed:
			if _, err := svc.DeleteImageBuilder(ctx, &appstream.DeleteImageBuilderInput{Name: &s.name}); err != nil {
				ui.Error(fmt.Sprintf("Error terminating ImageBuilder, may still be around: %s", err))
			}
			return
		case types.ImageBuilderStatePending, types.ImageBuilderStateStopping, types.ImageBuilderStateSnapshotting:
			// We cannot Delete a builder while pending
			ui.Say(fmt.Sprintf("Waiting for ImageBuilder to exit %s state (elapsed: %s)", imageBuilder.State, elapsed))
		case types.ImageBuilderStateRunning:
			// We cannot Delete a builder while running
			if _, err := svc.StopImageBuilder(ctx, &appstream.StopImageBuilderInput{Name: &s.name}); err != nil {
				ui.Error(fmt.Sprintf("Error stopping ImageBuilder, may still be around: %s", err))
			}
		default:
			// Unknown?  Need to stop it first so...
			ui.Error(fmt.Sprintf("Unexpected ImageBuilder state during cleanup: %s", imageBuilder.State))
			if _, err := svc.StopImageBuilder(ctx, &appstream.StopImageBuilderInput{Name: &s.name}); err != nil {
				ui.Error(fmt.Sprintf("Error stopping ImageBuilder, may still be around: %s", err))
			}
		}
		wait := 5 * time.Second
		elapsed += wait
		time.Sleep(wait)
	}
}
