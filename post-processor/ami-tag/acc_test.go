package ami_tag

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/hashicorp/packer-plugin-sdk/acctest"
)

//go:embed test-fixtures/basic.pkr.hcl
var testBasicHCL2 string

//go:embed test-fixtures/template-interpolation.pkr.hcl
var testTemplateInterpolationHCL2 string

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

// readLogs opens and reads the logfile, returning its contents.
func readLogs(logfile string) (string, error) {
	f, err := os.Open(logfile)
	if err != nil {
		return "", fmt.Errorf("unable to find %s", logfile)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("unable to read %s", logfile)
	}
	return string(b), nil
}

// parseAMIIDFromLogs extracts the first AMI ID from
// a "Tagged AMI ami-..." line in the Packer log output.
func parseAMIIDFromLogs(logs string) (string, error) {
	re := regexp.MustCompile(`Tagged AMI (ami-[a-z0-9]+)`)
	m := re.FindStringSubmatch(logs)
	if m == nil {
		return "", fmt.Errorf(
			"no 'Tagged AMI ami-...' found in logs")
	}
	return m[1], nil
}

// Run with: PACKER_ACC=1 go test -count 1 -v ./post-processor/ami-tag/ -run TestAccAMITag_TemplateInterpolation -timeout=120m
func TestAccAMITag_TemplateInterpolation(t *testing.T) {
	testCase := &acctest.PluginTestCase{
		Name:     "ami_tag_template_interpolation_test",
		Setup:    func() error { return nil },
		Teardown: func() error { return nil },
		Template: testTemplateInterpolationHCL2,
		Type:     "aws-ami-tag",
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			return checkTemplateInterpolation(t, logfile)
		},
	}
	acctest.TestPlugin(t, testCase)
}

func checkTemplateInterpolation(
	t *testing.T, logfile string,
) error {
	t.Helper()
	logs, err := readLogs(logfile)
	if err != nil {
		return err
	}

	// Post-processor must have been invoked.
	if ok, _ := regexp.MatchString(
		`aws-ami-tag`, logs); !ok {
		t.Fatalf("logs missing post-processor name")
	}

	// CopiedName key must appear in the tag key log.
	if ok, _ := regexp.MatchString(
		`CopiedName`, logs); !ok {
		t.Fatalf("logs missing 'CopiedName' tag key")
	}

	// Parse the AMI ID so we can query its tags.
	amiID, err := parseAMIIDFromLogs(logs)
	if err != nil {
		return err
	}

	// Build an EC2 client using default credentials.
	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}
	client := ec2.NewFromConfig(cfg)

	// DescribeImages to retrieve the AMI's tags.
	resp, err := client.DescribeImages(ctx,
		&ec2.DescribeImagesInput{
			ImageIds: []string{amiID},
		})
	if err != nil {
		return fmt.Errorf(
			"DescribeImages for %s: %w", amiID, err)
	}
	if len(resp.Images) == 0 {
		return fmt.Errorf("AMI %s not found", amiID)
	}

	// Collect tag values by key.
	tags := make(map[string]string)
	for _, tag := range resp.Images[0].Tags {
		if tag.Key != nil && tag.Value != nil {
			tags[*tag.Key] = *tag.Value
		}
	}

	nameVal, ok := tags["Name"]
	if !ok {
		return fmt.Errorf(
			"AMI %s has no 'Name' tag", amiID)
	}
	copiedVal, ok := tags["CopiedName"]
	if !ok {
		return fmt.Errorf(
			"AMI %s has no 'CopiedName' tag", amiID)
	}

	// The fixture sets Name = "ami-tag-interpolation-test"
	// and CopiedName = "{{ .AMITags.Name }}".
	// After rendering, CopiedName must equal Name.
	if copiedVal != nameVal {
		return fmt.Errorf(
			"CopiedName %q != Name %q: "+
				".AMITags interpolation mismatch",
			copiedVal, nameVal)
	}
	return nil
}
