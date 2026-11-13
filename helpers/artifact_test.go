package helpers_test

import (
	"github.com/bdwyertech/packer-plugin-aws/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseArtifactID", func() {
	DescribeTable("valid inputs",
		func(input string, expected []helpers.AMIRef) {
			refs, err := helpers.ParseArtifactID(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(refs).To(Equal(expected))
		},
		Entry("single region:ami",
			"us-east-1:ami-12345",
			[]helpers.AMIRef{{Region: "us-east-1", ID: "ami-12345"}},
		),
		Entry("multiple entries",
			"us-east-1:ami-aaa,eu-west-1:ami-bbb",
			[]helpers.AMIRef{
				{Region: "us-east-1", ID: "ami-aaa"},
				{Region: "eu-west-1", ID: "ami-bbb"},
			},
		),
		Entry("preserves order",
			"c:3,a:1,b:2",
			[]helpers.AMIRef{
				{Region: "c", ID: "3"},
				{Region: "a", ID: "1"},
				{Region: "b", ID: "2"},
			},
		),
		Entry("first colon split only",
			"region:ami:extra",
			[]helpers.AMIRef{{Region: "region", ID: "ami:extra"}},
		),
	)

	Context("error cases", func() {
		It("returns ErrEmptyArtifactID for empty string", func() {
			_, err := helpers.ParseArtifactID("")
			Expect(err).To(MatchError(helpers.ErrEmptyArtifactID))
		})

		It("returns ErrMalformedArtifact for missing colon", func() {
			_, err := helpers.ParseArtifactID("no-colon-here")
			Expect(err).To(MatchError(ContainSubstring("malformed")))
		})

		It("returns ErrMalformedArtifact for empty region", func() {
			_, err := helpers.ParseArtifactID(":ami-12345")
			Expect(err).To(MatchError(ContainSubstring("malformed")))
		})

		It("returns ErrMalformedArtifact for empty ID", func() {
			_, err := helpers.ParseArtifactID("us-east-1:")
			Expect(err).To(MatchError(ContainSubstring("malformed")))
		})
	})

	Context("output invariants", func() {
		It("has non-empty Region and ID for all refs", func() {
			refs, err := helpers.ParseArtifactID("a:1,b:2")
			Expect(err).NotTo(HaveOccurred())
			for _, r := range refs {
				Expect(r.Region).NotTo(BeEmpty())
				Expect(r.ID).NotTo(BeEmpty())
			}
		})
	})
})
