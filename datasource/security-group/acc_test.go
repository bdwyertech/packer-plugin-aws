package securitygroup

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

// Run with: PACKER_ACC=1 go test -count 1 -v ./datasource/security-group/acc_test.go -timeout=120m
func TestAccSecurityGroupDatasource_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "security_group_datasource_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "aws-security-group",
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

			// Check for the expected output
			if matched, _ := regexp.MatchString(`Security Group ID: sg-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Security Group ID: sg-...'")
			}

			if matched, _ := regexp.MatchString(`Security Group Name: .*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Security Group Name: ...'")
			}

			if matched, _ := regexp.MatchString(`Security Group ARN: arn:aws:ec2:.*:.*:security-group/sg-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output with ARN")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/filters.pkr.hcl
var testFiltersHCL2 string

func TestAccSecurityGroupDatasource_Filters(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "security_group_datasource_filters_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testFiltersHCL2,
		Type:     "aws-security-group",
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

			// Check for the filtered security group output
			if matched, _ := regexp.MatchString(`Filtered Security Group ID: sg-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Filtered Security Group ID: sg-...'")
			}

			// Check for the default security group output
			if matched, _ := regexp.MatchString(`Default Security Group ID: sg-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Default Security Group ID: sg-...'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/tags.pkr.hcl
var testTagsHCL2 string

func TestAccSecurityGroupDatasource_Tags(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "security_group_datasource_tags_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testTagsHCL2,
		Type:     "aws-security-group",
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

			// Check for the expected output
			if matched, _ := regexp.MatchString(`Security Group ID: sg-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Security Group ID: sg-...'")
			}

			if matched, _ := regexp.MatchString(`Security Group Name: .*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Security Group Name: ...'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
