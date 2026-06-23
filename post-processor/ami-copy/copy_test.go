package ami_copy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestPostProcessor_ImplementsPostProcessor(t *testing.T) {
	var _ packersdk.PostProcessor = new(PostProcessor)
}

func TestPostProcessor_Impl(t *testing.T) {
	var raw any
	raw = &PostProcessor{}
	if _, ok := raw.(packersdk.PostProcessor); !ok {
		t.Fatalf("must be a post processor")
	}
}

func TestAmisFromArtifactID(t *testing.T) {
	artifact := "us-east-1:ami-123,us-west-2:ami-456"
	amis := amisFromArtifactID(artifact)

	if len(amis) != 2 {
		t.Fatalf("expected 2 amis, got %d", len(amis))
	}

	if amis[0].region != "us-east-1" || amis[0].id != "ami-123" {
		t.Fatalf("first ami mismatch: %+v", amis[0])
	}

	if amis[1].region != "us-west-2" || amis[1].id != "ami-456" {
		t.Fatalf("second ami mismatch: %+v", amis[1])
	}
}

func TestWriteManifestsWritesJSON(t *testing.T) {
	tmp := t.TempDir() + "/manifest.json"
	manifests := []*AmiManifest{
		{AccountID: "111111111111", Region: "us-east-1", ImageID: "ami-abc"},
	}

	if err := writeManifests(tmp, manifests); err != nil {
		t.Fatalf("writeManifests failed: %v", err)
	}

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("reading manifest file failed: %v", err)
	}

	var out []*AmiManifest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(out) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(out))
	}
	if out[0].ImageID != "ami-abc" || out[0].AccountID != "111111111111" || out[0].Region != "us-east-1" {
		t.Fatalf("manifest content mismatch: %+v", out[0])
	}
}

func TestExecuteCopies_TagsOnly_NoClientNeeded(t *testing.T) {
	ui := packersdk.TestUi(t)

	srcImage := &types.Image{
		ImageId: aws.String("ami-xyz"),
		Tags:    []types.Tag{}, // no tags -> tagImage will be no-op
	}

	c := &copyOperation{
		ctx:             context.Background(),
		client:          nil, // should not be used because tagsOnly == true and no tags
		sourceImage:     srcImage,
		sourceRegion:    "us-east-1",
		sourceImageID:   "ami-xyz",
		ensureAvailable: false,
		tagsOnly:        true,
		tags:            map[string]string{},
		encrypted:       false,
		targetAccountID: "000000000000",
	}

	p := PostProcessor{
		config: Config{
			CopyConcurrency: 1,
			ManifestOutput:  "",
		},
	}

	_, errs := p.executeCopies([]*copyOperation{c}, ui)
	if len(errs.Errors) != 0 {
		t.Fatalf("expected no errors from executeCopies, got: %v", errs)
	}

	if c.copiedImageID != "ami-xyz" {
		t.Fatalf("expected copiedImageID to be source image id, got %q", c.copiedImageID)
	}
}

func TestCopyExecute_SetsCopiedImageWhenTagsOnly(t *testing.T) {
	ui := packersdk.TestUi(t)

	srcImage := &types.Image{
		ImageId: aws.String("ami-foo"),
		Tags:    []types.Tag{},
	}

	c := &copyOperation{
		ctx:           context.Background(),
		client:        nil,
		sourceImage:   srcImage,
		sourceRegion:  "us-east-1",
		sourceImageID: "ami-foo",
		tagsOnly:      true,
		tags:          map[string]string{},
	}

	if err := c.execute(ui); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if c.copiedImageID != "ami-foo" {
		t.Fatalf("expected copiedImageID 'ami-foo', got %q", c.copiedImageID)
	}
}

// capturingUi captures everything written through the Ui so tests can assert
// against log output.
func capturingUi() (*packersdk.BasicUi, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return &packersdk.BasicUi{
		Reader:      &bytes.Buffer{},
		Writer:      buf,
		ErrorWriter: buf,
		PB:          &packersdk.NoopProgressTracker{},
	}, buf
}

// TestExecuteCopies_CrossRegion_ReportsTargetRegion covers B1 and B2: when a
// copyOperation is run with sourceRegion != targetRegion, the resulting
// AmiManifest must record the target region and log lines must reference the
// target region as the destination — not collapse it under the source region.
func TestExecuteCopies_CrossRegion_ReportsTargetRegion(t *testing.T) {
	ui, buf := capturingUi()

	srcImage := &types.Image{
		ImageId: aws.String("ami-src"),
		Tags:    []types.Tag{},
	}

	c := &copyOperation{
		ctx:             context.Background(),
		client:          nil,
		sourceImage:     srcImage,
		sourceRegion:    "us-east-1",
		targetRegion:    "us-west-1",
		sourceImageID:   "ami-src",
		ensureAvailable: false,
		tagsOnly:        true,
		tags:            map[string]string{},
		encrypted:       false,
		targetAccountID: "000000000000",
	}

	p := PostProcessor{
		config: Config{
			CopyConcurrency: 1,
			ManifestOutput:  "",
		},
	}

	manifests, errs := p.executeCopies([]*copyOperation{c}, ui)
	if len(errs.Errors) != 0 {
		t.Fatalf("expected no errors from executeCopies, got: %v", errs)
	}

	if len(manifests) != 1 {
		t.Fatalf("expected exactly 1 manifest, got %d", len(manifests))
	}

	// B1: AmiManifest.Region must be the target region.
	if manifests[0].Region != "us-west-1" {
		t.Errorf("B1: expected AmiManifest.Region=us-west-1, got %q", manifests[0].Region)
	}

	// B2: log output must reference the cross-region destination. Current
	// format is "[sourceRegion:sourceImageID] ... to targetRegion:targetAccountID".
	out := buf.String()
	if !strings.Contains(out, "[us-east-1:ami-src]") {
		t.Errorf("B2: expected log output to contain source prefix [us-east-1:ami-src], got:\n%s", out)
	}
	if !strings.Contains(out, "to us-west-1:000000000000") {
		t.Errorf("B2: expected log output to reference destination us-west-1:000000000000, got:\n%s", out)
	}
}

// TestExecuteCopies_SameRegion_StillReportsRegion is the regression guard for
// the existing same-region path: when sourceRegion == targetRegion, reporting
// continues to use that region for both prefix and destination.
func TestExecuteCopies_SameRegion_StillReportsRegion(t *testing.T) {
	ui, buf := capturingUi()

	srcImage := &types.Image{
		ImageId: aws.String("ami-src"),
		Tags:    []types.Tag{},
	}

	c := &copyOperation{
		ctx:             context.Background(),
		client:          nil,
		sourceImage:     srcImage,
		sourceRegion:    "us-east-1",
		targetRegion:    "us-east-1",
		sourceImageID:   "ami-src",
		tagsOnly:        true,
		tags:            map[string]string{},
		targetAccountID: "000000000000",
	}

	p := PostProcessor{config: Config{CopyConcurrency: 1}}

	manifests, errs := p.executeCopies([]*copyOperation{c}, ui)
	if len(errs.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(manifests) != 1 || manifests[0].Region != "us-east-1" {
		t.Fatalf("expected single manifest with Region=us-east-1, got %+v", manifests)
	}
	out := buf.String()
	if !strings.Contains(out, "[us-east-1:ami-src]") {
		t.Errorf("expected log output to contain source prefix [us-east-1:ami-src], got:\n%s", out)
	}
	if !strings.Contains(out, "to us-east-1:000000000000") {
		t.Errorf("expected log output to reference destination us-east-1:000000000000, got:\n%s", out)
	}
}

// _ keeps io imported even when other helpers below don't reference it.
var _ = io.Discard
