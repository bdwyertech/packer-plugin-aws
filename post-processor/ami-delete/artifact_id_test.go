package ami_delete

import (
	"github.com/bdwyertech/packer-plugin-aws/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("helpers.ParseArtifactID", func() {
	Context("malformed input without colon", func() {
		It("returns an error instead of panicking", func() {
			var amis []helpers.AMIRef
			var err error
			Expect(func() {
				amis, err = helpers.ParseArtifactID("no-colon-here")
			}).NotTo(Panic())
			Expect(err).To(MatchError(helpers.ErrMalformedArtifact))
			Expect(amis).To(BeNil())
		})
	})
})
