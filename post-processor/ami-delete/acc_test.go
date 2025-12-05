package ami_delete

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

// Run with: PACKER_ACC=1 go test -count 1 -v ./post-processor/ami-delete/acc_test.go -timeout=120m
func TestAccAMIDelete_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "ami_delete_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "aws-ami-delete",
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			// We expect this to fail because there's no artifact from the null builder,
			// but we want to verify the plugin configuration is valid and loads correctly.

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

			// Check that the post-processor was invoked or configured
			// The null builder won't produce an artifact ID, so we expect an error
			// but the post-processor should still be configured
			if matched, _ := regexp.MatchString(`aws-ami-delete`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected post-processor name 'aws-ami-delete'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}