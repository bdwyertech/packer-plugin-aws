package ami_tag

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

// Run with: PACKER_ACC=1 go test -count 1 -v ./post-processor/ami-tag/ -run TestAcc -timeout=120m
func TestAccAMITag_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "ami_tag_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "aws-ami-tag",
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

			// Check that the post-processor was invoked
			if matched, _ := regexp.MatchString(`aws-ami-tag`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected post-processor name 'aws-ami-tag'")
			}

			// Check that tagging was reported
			if matched, _ := regexp.MatchString(`Tagged AMI ami-`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected 'Tagged AMI ami-' success message")
			}

			// Check that ec2:SharedTag keys appeared in logs
			if matched, _ := regexp.MatchString(`ec2:SharedTag/team`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected 'ec2:SharedTag/team' key")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
