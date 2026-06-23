package ami_watermark

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

// Run with: PACKER_ACC=1 go test -count 1 -v ./post-processor/ami-watermark/ -run TestAcc -timeout=120m
func TestAccAMIWatermark_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "ami_watermark_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "aws-ami-watermark",
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

			// Check that the post-processor was invoked or configured
			if matched, _ := regexp.MatchString(`aws-ami-watermark`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected post-processor name 'aws-ami-watermark'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
