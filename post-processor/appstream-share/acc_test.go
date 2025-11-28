package appstream

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

// Run with: PACKER_ACC=1 go test -count 1 -v ./post-processor/appstream-share/acc_test.go -timeout=120m
func TestAccAppStreamShare_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "appstream_share_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "appstream-share",
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			// We expect this to fail because the image doesn't exist, but we want to verify the plugin ran.
			// So we don't check for exit code 0 necessarily, or we expect a specific error.
			// However, the acctest might fail the test if exit code is not 0.
			// Let's assume we want to see the "Waiting for image" log.

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

			// Check for the start message
			if matched, _ := regexp.MatchString(`Sharing AppStream image...`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Sharing AppStream image...'")
			}

			// Check for the waiting message
			if matched, _ := regexp.MatchString(`Waiting for image test-image-does-not-exist to be available`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Waiting for image...'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
