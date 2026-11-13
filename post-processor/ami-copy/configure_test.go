package ami_copy_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ami_copy "github.com/bdwyertech/packer-plugin-aws/post-processor/ami-copy"
)

var _ = Describe("Configure", func() {
	Context("missing AccessConfig.Prepare", func() {
		It("returns an error for an invalid AWS access config", func() {
			p := new(ami_copy.PostProcessor)

			// Set access_key without secret_key.
			// AccessConfig.Prepare rejects this combo,
			// but Configure never calls Prepare today.
			err := p.Configure(map[string]interface{}{
				"ami_users":  []string{"123456789012"},
				"access_key": "test",
			})

			Expect(err).To(HaveOccurred(),
				"Configure should surface AccessConfig.Prepare errors")
		})
	})
})
