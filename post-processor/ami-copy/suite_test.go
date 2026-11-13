package ami_copy_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAmiCopy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AmiCopy Suite")
}
