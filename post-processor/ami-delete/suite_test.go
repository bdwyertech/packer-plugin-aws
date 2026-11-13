package ami_delete_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAmiDelete(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AmiDelete Suite")
}
