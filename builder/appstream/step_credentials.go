package appstream

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// StepCredentials writes the credentials to AWS Secrets Manager
type StepCredentials struct {
	Debug     bool
	Comm      *communicator.Config
	Timeout   time.Duration
	BuildName string
}

func (s *StepCredentials) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	//
	// TODO: Use our password or generate a random one and write it into AWS Secrets Manager
	//

	// In debug-mode, we output the password
	if s.Debug {
		ui.Say(fmt.Sprintf(
			"Password (since debug is enabled): %s", s.Comm.WinRMPassword))
	}
	// store so that we can access this later during provisioning
	state.Put("winrm_password", s.Comm.WinRMPassword)
	packersdk.LogSecretFilter.Set(s.Comm.WinRMPassword)

	return multistep.ActionContinue
}

func (s *StepCredentials) Cleanup(multistep.StateBag) {
	//
	// TODO: Perhaps we need to put this here to delete the credentials?
	//
}
