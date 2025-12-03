package image

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

//go:embed test-fixtures/basic.pkr.hcl
var testBasicHCL2 string

// Run with: PACKER_ACC=1 go test -count 1 -v ./datasource/appstream-image/acc_test.go -timeout=120m
func TestAccImageDatasource_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "image_builder_datasource_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "aws-appstream-image", // This is technically the plugin name, but for acc tests it might not matter as much if we are just running packer
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := io.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			// Check for the error message
			if matched, _ := regexp.MatchString(`Builder IP: .*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Builder IP: <ip_address>'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/non-extant.pkr.hcl
var testNonExtantHCL2 string

func TestAccImageDatasource_NonExtant(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "image_datasource_non_extant_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testNonExtantHCL2,
		Type:     "aws-appstream-image", // This is technically the plugin name, but for acc tests it might not matter as much if we are just running packer
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			// We expect this to fail because the builder doesn't exist.
			// But we want to see the error message from the datasource.

			logs, err := os.Open(logfile)
			if err != nil {
				return fmt.Errorf("Unable find %s", logfile)
			}
			defer logs.Close()

			logsBytes, err := io.ReadAll(logs)
			if err != nil {
				return fmt.Errorf("Unable to read %s", logfile)
			}
			logsString := string(logsBytes)

			// Check for the error message
			if matched, _ := regexp.MatchString(`ResourceNotFoundException: The image builder test-builder-does-not-exist for account .* does not exist.`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'image builder ... does not exist'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
