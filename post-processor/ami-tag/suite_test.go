package ami_tag_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAmiTag(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AmiTag Suite")
}
