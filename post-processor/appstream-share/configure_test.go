package appstream_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appstream "github.com/bdwyertech/packer-plugin-aws/post-processor/appstream-share"
)

var _ = Describe("Configure", func() {
	Context("Decode-before-Prepare ordering", func() {
		// Configure calls AccessConfig.Prepare *before*
		// checking the Decode error.  When Decode fails (e.g. a
		// malformed template that cannot be interpolated), Prepare
		// still runs on the partially-decoded config.  After the
		// fix, the Decode error should be returned immediately
		// without ever calling Prepare.
		It("returns the Decode error, not a Prepare error", func() {
			p := new(appstream.PostProcessor)

			// An unclosed Go template action forces config.Decode
			// to fail during interpolation (Interpolate: true,
			// Exclude list is empty).  With WeaklyTypedInput the
			// mapstructure step succeeds, but the render step
			// returns a template-parse error.
			err := p.Configure(map[string]interface{}{
				"image_name": "{{ .Broken",
			})

			Expect(err).To(HaveOccurred(),
				"Configure must return an error when Decode fails")

			// The error must originate from the Decode/interpolation
			// step — not from AccessConfig.Prepare or from field
			// validation (e.g. "account_ids is required").
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("unclosed action"),
				ContainSubstring("unexpected"),
				ContainSubstring("template"),
			), "expected a Decode/interpolation error, got: %s", err)
		})
	})
})

var _ = Describe("BuilderId", func() {
	It("equals \"appstream-share\"", func() {
		Expect(appstream.BuilderId).To(Equal("appstream-share"))
	})
})
