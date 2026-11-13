package ami_copy

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Artifact", func() {
	Describe("Id", func() {
		// Artifact.Id() must include per-manifest Region in each
		// region:account:image tuple.
		It("includes each manifest's region:account:image tuple", func() {
			a := &Artifact{
				CopiedAmis: []*AmiManifest{
					{Region: "us-east-1", AccountID: "111111111111", ImageID: "ami-east"},
					{Region: "us-west-1", AccountID: "222222222222", ImageID: "ami-west"},
				},
			}

			id := a.Id()

			Expect(id).To(ContainSubstring("us-east-1:111111111111:ami-east"))
			Expect(id).To(ContainSubstring("us-west-1:222222222222:ami-west"))
		})
	})

	Describe("String", func() {
		// Artifact.String() must group AMIs under per-manifest
		// Region headers in the final summary.
		It("groups AMIs by region header", func() {
			a := &Artifact{
				CopiedAmis: []*AmiManifest{
					{Region: "us-east-1", AccountID: "111111111111", ImageID: "ami-east-a"},
					{Region: "us-east-1", AccountID: "111111111111", ImageID: "ami-east-b"},
					{Region: "us-west-1", AccountID: "222222222222", ImageID: "ami-west"},
				},
			}

			out := a.String()

			Expect(out).To(ContainSubstring("us-east-1:"))
			Expect(out).To(ContainSubstring("us-west-1:"))

			westIdx := strings.Index(out, "us-west-1:")
			westAmiIdx := strings.Index(out, "ami-west")
			Expect(westIdx).To(BeNumerically(">=", 0))
			Expect(westAmiIdx).To(BeNumerically(">", westIdx))

			eastIdx := strings.Index(out, "us-east-1:")
			eastAmiAIdx := strings.Index(out, "ami-east-a")
			eastAmiBIdx := strings.Index(out, "ami-east-b")
			Expect(eastAmiAIdx).To(BeNumerically(">", eastIdx))
			Expect(eastAmiBIdx).To(BeNumerically(">", eastIdx))
		})

		It("returns empty-manifest sentinel when there are no copied AMIs", func() {
			a := &Artifact{}
			Expect(a.String()).To(Equal("No AMIs were copied"))
		})
	})
})
