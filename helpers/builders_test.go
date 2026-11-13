package helpers_test

import (
	"github.com/bdwyertech/packer-plugin-aws/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("IsAMIBuilder", func() {
	DescribeTable("returns true for known AMI builders",
		func(builderID string) {
			Expect(helpers.IsAMIBuilder(builderID)).To(BeTrue())
		},
		Entry("ebs", "mitchellh.amazonebs"),
		Entry("ebssurrogate", "mitchellh.amazon.ebssurrogate"),
		Entry("ebsvolume", "mitchellh.amazon.ebsvolume"),
		Entry("chroot", "mitchellh.amazon.chroot"),
		Entry("instance", "mitchellh.amazon.instance"),
	)

	DescribeTable("returns false for non-AMI builders",
		func(builderID string) {
			Expect(helpers.IsAMIBuilder(builderID)).To(BeFalse())
		},
		Entry("empty string", ""),
		Entry("unknown builder", "some.other.builder"),
		Entry("partial match", "mitchellh.amazon"),
	)

	It("AMIBuilderIDs contains exactly 5 entries", func() {
		Expect(helpers.AMIBuilderIDs).To(HaveLen(5))
	})
})
