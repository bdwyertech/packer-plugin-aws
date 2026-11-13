package appstream_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAppStreamShare(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AppStreamShare Suite")
}
