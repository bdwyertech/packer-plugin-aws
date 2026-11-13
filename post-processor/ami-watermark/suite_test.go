package ami_watermark_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAmiWatermark(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AmiWatermark Suite")
}
