package ami_copy

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type Artifact struct {
	// CopiedAmis holds information about copied AMIs including account, region, and AMI ID
	CopiedAmis []*AmiManifest

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
	parts := make([]string, 0, len(a.CopiedAmis))
	for _, ami := range a.CopiedAmis {
		parts = append(parts, fmt.Sprintf("%s:%s:%s", ami.Region, ami.AccountID, ami.ImageID))
	}

	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (a *Artifact) String() string {
	if len(a.CopiedAmis) == 0 {
		return "No AMIs were copied"
	}

	// Group AMIs by region
	regionMap := make(map[string][]*AmiManifest)
	for _, ami := range a.CopiedAmis {
		regionMap[ami.Region] = append(regionMap[ami.Region], ami)
	}

	// Sort regions
	regions := make([]string, 0, len(regionMap))
	for region := range regionMap {
		regions = append(regions, region)
	}
	sort.Strings(regions)

	// Build output
	var result strings.Builder
	result.WriteString("AMIs were copied to the following accounts/regions:")
	for i, region := range regions {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(fmt.Sprintf("\n%s:", region))
		for j, ami := range regionMap[region] {
			if j == 0 {
				result.WriteString(fmt.Sprintf(" %s:%s", ami.AccountID, ami.ImageID))
			} else {
				result.WriteString(fmt.Sprintf("\n%s %s:%s", strings.Repeat(" ", len(region)+1), ami.AccountID, ami.ImageID))
			}
		}
	}
	return result.String()
}

func (a *Artifact) State(name string) any {
	if _, ok := a.StateData[name]; ok {
		return a.StateData[name]
	}
	return nil
}

func (a *Artifact) Destroy() error {
	for _, ami := range a.CopiedAmis {
		if err := ami.delete(context.TODO()); err != nil {
			return err
		}
	}
	return nil
}
