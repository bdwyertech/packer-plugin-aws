package ami_copy

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestPostProcessor_ImplementsPostProcessor(t *testing.T) {
	var _ packersdk.PostProcessor = new(PostProcessor)
}

func TestPostProcessor_Impl(t *testing.T) {
	var raw interface{}
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

	errs := p.executeCopies([]*copyOperation{c}, ui)
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
