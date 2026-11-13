package ami_copy

import (
	"bytes"
	"context"
	"encoding/json"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

var _ = Describe("PostProcessor", func() {
	It("implements packer.PostProcessor (typed)", func() {
		var _ packersdk.PostProcessor = new(PostProcessor)
	})

	It("implements packer.PostProcessor (interface cast)", func() {
		var raw any = &PostProcessor{}
		_, ok := raw.(packersdk.PostProcessor)
		Expect(ok).To(BeTrue(), "must be a post processor")
	})
})

var _ = Describe("writeManifests", func() {
	It("writes valid JSON with correct content", func() {
		tmp := GinkgoT().TempDir() + "/manifest.json"
		manifests := []*AmiManifest{
			{AccountID: "111111111111", Region: "us-east-1", ImageID: "ami-abc"},
		}

		Expect(writeManifests(tmp, manifests)).To(Succeed())

		data, err := os.ReadFile(tmp)
		Expect(err).NotTo(HaveOccurred())

		var out []*AmiManifest
		Expect(json.Unmarshal(data, &out)).To(Succeed())
		Expect(out).To(HaveLen(1))
		Expect(out[0].ImageID).To(Equal("ami-abc"))
		Expect(out[0].AccountID).To(Equal("111111111111"))
		Expect(out[0].Region).To(Equal("us-east-1"))
	})
})

var _ = Describe("executeCopies", func() {
	Context("tags-only with no client needed", func() {
		It("succeeds and sets copiedImageID", func() {
			ui := &packersdk.MockUi{}
			srcImage := &types.Image{
				ImageId: aws.String("ami-xyz"),
				Tags:    []types.Tag{},
			}
			c := &copyOperation{
				ctx:             context.Background(),
				client:          nil,
				sourceImage:     srcImage,
				sourceRegion:    "us-east-1",
				sourceImageID:   "ami-xyz",
				ensureAvailable: false,
				tagsOnly:        true,
				tags:            map[string]string{},
				encrypted:       false,
				targetAccountID: "000000000000",
			}
			p := PostProcessor{config: Config{CopyConcurrency: 1}}

			_, errs := p.executeCopies([]*copyOperation{c}, ui)
			Expect(errs.Errors).To(BeEmpty())
			Expect(c.copiedImageID).To(Equal("ami-xyz"))
		})
	})

	Context("cross-region reporting", func() {
		It("reports target region in manifests and log output", func() {
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
			p := PostProcessor{config: Config{CopyConcurrency: 1}}

			manifests, errs := p.executeCopies([]*copyOperation{c}, ui)
			Expect(errs.Errors).To(BeEmpty())
			Expect(manifests).To(HaveLen(1))
			Expect(manifests[0].Region).To(Equal("us-west-1"))

			out := buf.String()
			Expect(out).To(ContainSubstring("[us-east-1:ami-src]"))
			Expect(out).To(ContainSubstring("to us-west-1:000000000000"))
		})
	})

	Context("same-region reporting", func() {
		It("reports the same region in manifests and log output", func() {
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
			Expect(errs.Errors).To(BeEmpty())
			Expect(manifests).To(HaveLen(1))
			Expect(manifests[0].Region).To(Equal("us-east-1"))

			out := buf.String()
			Expect(out).To(ContainSubstring("[us-east-1:ami-src]"))
			Expect(out).To(ContainSubstring("to us-east-1:000000000000"))
		})
	})
})

var _ = Describe("copyOperation", func() {
	Describe("execute", func() {
		It("sets copiedImageID to source image ID when tagsOnly", func() {
			ui := &packersdk.MockUi{}
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

			Expect(c.execute(ui)).To(Succeed())
			Expect(c.copiedImageID).To(Equal("ami-foo"))
		})
	})
})
