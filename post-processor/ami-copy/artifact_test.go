package ami_copy

import (
	"strings"
	"testing"
)

// TestArtifactId_GroupsByManifestRegion covers B3: Artifact.Id() must include
// the per-manifest Region in each region:account:image tuple. With the cross
// region attribution fix, manifests across multiple regions appear under their
// own region prefixes (not collapsed under a single source region).
func TestArtifactId_GroupsByManifestRegion(t *testing.T) {
	a := &Artifact{
		CopiedAmis: []*AmiManifest{
			{Region: "us-east-1", AccountID: "111111111111", ImageID: "ami-east"},
			{Region: "us-west-1", AccountID: "222222222222", ImageID: "ami-west"},
		},
	}

	id := a.Id()

	if !strings.Contains(id, "us-east-1:111111111111:ami-east") {
		t.Errorf("B3: expected Artifact.Id() to contain us-east-1:111111111111:ami-east, got %q", id)
	}
	if !strings.Contains(id, "us-west-1:222222222222:ami-west") {
		t.Errorf("B3: expected Artifact.Id() to contain us-west-1:222222222222:ami-west, got %q", id)
	}
}

// TestArtifactString_GroupsAMIsByRegionHeader covers B4: Artifact.String() must
// group AMIs under their per-manifest Region headers in the final summary.
func TestArtifactString_GroupsAMIsByRegionHeader(t *testing.T) {
	a := &Artifact{
		CopiedAmis: []*AmiManifest{
			{Region: "us-east-1", AccountID: "111111111111", ImageID: "ami-east-a"},
			{Region: "us-east-1", AccountID: "111111111111", ImageID: "ami-east-b"},
			{Region: "us-west-1", AccountID: "222222222222", ImageID: "ami-west"},
		},
	}

	out := a.String()

	if !strings.Contains(out, "us-east-1:") {
		t.Errorf("B4: expected us-east-1 region header in output, got:\n%s", out)
	}
	if !strings.Contains(out, "us-west-1:") {
		t.Errorf("B4: expected us-west-1 region header in output, got:\n%s", out)
	}

	westIdx := strings.Index(out, "us-west-1:")
	westAmiIdx := strings.Index(out, "ami-west")
	if westIdx < 0 || westAmiIdx < 0 || westAmiIdx < westIdx {
		t.Errorf("B4: expected ami-west to appear after the us-west-1 header, got:\n%s", out)
	}

	eastIdx := strings.Index(out, "us-east-1:")
	eastAmiAIdx := strings.Index(out, "ami-east-a")
	eastAmiBIdx := strings.Index(out, "ami-east-b")
	if eastIdx < 0 || eastAmiAIdx < eastIdx || eastAmiBIdx < eastIdx {
		t.Errorf("B4: expected ami-east-{a,b} to appear after the us-east-1 header, got:\n%s", out)
	}
}

// TestArtifactString_EmptyManifests covers the regression case noted in the
// design: Artifact.String() returns the empty-case sentinel when there are no
// copied AMIs.
func TestArtifactString_EmptyManifests(t *testing.T) {
	a := &Artifact{}
	if got := a.String(); got != "No AMIs were copied" {
		t.Errorf("expected empty-manifest sentinel, got %q", got)
	}
}
