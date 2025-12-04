package subnet

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

// Run with: PACKER_ACC=1 go test -count 1 -v ./datasource/subnet/acc_test.go -timeout=120m
func TestAccSubnetDatasource_Basic(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "subnet_datasource_basic_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testBasicHCL2,
		Type:     "aws-subnet",
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
			if matched, _ := regexp.MatchString(`Subnet ID: subnet-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Subnet ID: subnet-...'")
			}

			if matched, _ := regexp.MatchString(`Subnet CIDR: \d+\.\d+\.\d+\.\d+/\d+`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output with CIDR block")
			}

			if matched, _ := regexp.MatchString(`Subnet AZ: [a-z]+-[a-z]+-\d+[a-z]`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output with availability zone")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

//go:embed test-fixtures/filters.pkr.hcl
var testFiltersHCL2 string

func TestAccSubnetDatasource_Filters(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name: "subnet_datasource_filters_test",
		Setup: func() error {
			return nil
		},
		Teardown: func() error {
			return nil
		},
		Template: testFiltersHCL2,
		Type:     "aws-subnet",
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

			// Check for the filtered subnet output
			if matched, _ := regexp.MatchString(`Filtered Subnet ID: subnet-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Filtered Subnet ID: subnet-...'")
			}

			// Check for the default subnet output
			if matched, _ := regexp.MatchString(`Default Subnet ID: subnet-.*`, logsString); !matched {
				t.Fatalf("logs doesn't contain expected output 'Default Subnet ID: subnet-...'")
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}
