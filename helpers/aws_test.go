package helpers_test

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/bdwyertech/packer-plugin-aws/helpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RegionalConfig", func() {
	It("returned config has overridden region", func() {
		base := aws.Config{Region: "us-east-1"}
		result := helpers.RegionalConfig(&base, "eu-west-1")
		Expect(result.Region).To(Equal("eu-west-1"))
	})

	It("base config is not mutated", func() {
		base := aws.Config{Region: "us-east-1"}
		_ = helpers.RegionalConfig(&base, "eu-west-1")
		Expect(base.Region).To(Equal("us-east-1"))
	})

	It("preserves other fields", func() {
		base := aws.Config{
			Region: "us-east-1",
			AppID:  "test-app",
		}
		result := helpers.RegionalConfig(&base, "ap-southeast-1")
		Expect(result.AppID).To(Equal("test-app"))
		Expect(result.Region).To(Equal("ap-southeast-1"))
	})
})
