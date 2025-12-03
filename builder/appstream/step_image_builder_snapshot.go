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

	_ = s.config.Tags
	cmd := &packersdk.RemoteCmd{}
	comm.Start(ctx, cmd)
	cmd.Wait()
	// TODO: Somehow have powershell implement something similar to the following:
	// provisioner "windows-shell" {
	//   inline = [
	//     "start \"\" /B \"image-assistant.exe\" \"create-image\" \"--name=${s.name}\" \"--tags=${join(" ", [for k, v in s.config.Tags : "\"${k}\" \"${v}\""])}\""
	//   ]
	// }

	// Wait for image to become available
	var elapsed time.Duration
	for {
		images, err := svc.DescribeImages(ctx, &appstream.DescribeImagesInput{
			Names: []string{"TODO-IMPLEMENT-NAME"},
			Type:  types.VisibilityTypePrivate,
		})

		if err != nil {
			state.Put("error", fmt.Errorf("failed to describe images: %w", err))
			return multistep.ActionHalt
		}

		switch images.Images[0].State {
		case types.ImageStateAvailable:
			return multistep.ActionContinue
		case types.ImageStateFailed:
			state.Put("error", fmt.Errorf("ImageBuilder failed"))
			return multistep.ActionHalt
		case types.ImageStatePending:
			ui.Say(fmt.Sprintf("Waiting for ImageBuilder to become available (elapsed: %s)", elapsed))
			wait := 5 * time.Second
			elapsed += wait
			time.Sleep(wait)
			continue
		}
	}
}

func (s *StepImageBuilderSnapshot) Cleanup(multistep.StateBag) {
	// Nothing to do
}
