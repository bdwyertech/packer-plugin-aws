package ami_delete

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type Artifact struct {
	// A map of regions to deleted AMI IDs.
	DeletedAmis map[string][]string

	// BuilderId is the unique ID for the builder that created this AMI
	BuilderIdValue string

	// StateData should store data such as GeneratedData
	// to be shared with post-processors
	StateData map[string]any
}

var _ packer.Artifact = new(Artifact)

func (a *Artifact) BuilderId() string {
	return a.BuilderIdValue
}

func (*Artifact) Files() []string {
	// We have no files
	return nil
}

func (a *Artifact) Id() string {
	parts := make([]string, 0)
	for region, amiIds := range a.DeletedAmis {
		for _, amiId := range amiIds {
			parts = append(parts, fmt.Sprintf("%s:%s", region, amiId))
		}
	}

	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (a *Artifact) String() string {
	if len(a.DeletedAmis) == 0 {
		return "No AMIs were deleted"
	}

	amiStrings := make([]string, 0)
	for region, amiIds := range a.DeletedAmis {
		for _, id := range amiIds {
			amiStrings = append(amiStrings, fmt.Sprintf("%s: %s", region, id))
		}
	}

	sort.Strings(amiStrings)
	return fmt.Sprintf("AMIs were deleted:\n%s", strings.Join(amiStrings, "\n"))
}

func (a *Artifact) State(name string) any {
	if _, ok := a.StateData[name]; ok {
		return a.StateData[name]
	}
	return nil
}

func (a *Artifact) Destroy() error {
	return nil
}
