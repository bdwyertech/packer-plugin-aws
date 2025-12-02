package appstream

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
)

type StepSetGeneratedData struct {
	GeneratedData *packerbuilderdata.GeneratedData
}

func (s *StepSetGeneratedData) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	//
	// TODO: Might need something here?
	//
	//	appstreamConn := state.Get("appstreamv2").(*appstream.Client)
	//
	//	extractBuildInfo(state, s.GeneratedData)

	return multistep.ActionContinue
}

func (s *StepSetGeneratedData) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
