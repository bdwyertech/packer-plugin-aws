package appstream

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/aws/aws-sdk-go-v2/service/appstream/types"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepImageBuilderSnapshot struct {
	config Config
}

var _ multistep.Step = new(StepImageBuilderSnapshot)

func (s *StepImageBuilderSnapshot) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
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
	comm, ok := state.Get("communicator").(packersdk.Communicator)
	if !ok {
		state.Put("error", fmt.Errorf("communicator not found"))
		return multistep.ActionHalt
	}

	ui.Say("Launching an AppStream ImageBuilder...")

	// Construct the command to create the image
	cmdString := fmt.Sprintf("image-assistant.exe create-image --name %s", s.config.Name)
	if len(s.config.Tags) > 0 {
		cmdString += " --tags"
		for k, v := range s.config.Tags {
			cmdString += fmt.Sprintf(" %q %q", k, v)
		}
	}

	ui.Say(fmt.Sprintf("Executing command: %s", cmdString))

	cmd := &packersdk.RemoteCmd{
		Command: cmdString,
	}

	if err := cmd.RunWithUi(ctx, comm, ui); err != nil {
		state.Put("error", fmt.Errorf("failed to run image creation command: %w", err))
		return multistep.ActionHalt
	}

	if cmd.ExitStatus() != 0 {
		state.Put("error", fmt.Errorf("image creation command failed with exit status: %d", cmd.ExitStatus()))
		return multistep.ActionHalt
	}

	// Wait for image to become available
	var elapsed time.Duration
	for {
		images, err := svc.DescribeImages(ctx, &appstream.DescribeImagesInput{
			Names: []string{s.config.Name},
			Type:  types.VisibilityTypePrivate,
		})

		if err != nil {
			state.Put("error", fmt.Errorf("failed to describe images: %w", err))
			return multistep.ActionHalt
		}

		if len(images.Images) == 0 {
			// Image might not be immediately available after command returns
			ui.Say(fmt.Sprintf("Waiting for image %s to appear... (elapsed: %s)", s.config.Name, elapsed))
			wait := 10 * time.Second
			elapsed += wait
			time.Sleep(wait)
			continue
		}

		switch images.Images[0].State {
		case types.ImageStateAvailable:
			state.Put("images", map[string]string{
				s.config.RawRegion: s.config.Name,
			})
			return multistep.ActionContinue
		case types.ImageStateFailed:
			state.Put("error", fmt.Errorf("ImageBuilder failed"))
			return multistep.ActionHalt
		case types.ImageStatePending:
			ui.Say(fmt.Sprintf("Waiting for ImageBuilder to become available (elapsed: %s)", elapsed))
			wait := 10 * time.Second
			elapsed += wait
			time.Sleep(wait)
			continue
		default:
			// Handle other states if necessary, or just wait
			ui.Say(fmt.Sprintf("Image state is %s, waiting... (elapsed: %s)", images.Images[0].State, elapsed))
			wait := 10 * time.Second
			elapsed += wait
			time.Sleep(wait)
			continue
		}
	}
}

func (s *StepImageBuilderSnapshot) Cleanup(multistep.StateBag) {
	// Nothing to do
}
