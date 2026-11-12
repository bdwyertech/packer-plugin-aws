package ami_tag

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// mockDescribeTagsAPI is a mock implementation of tagAPI for
// testing existingTags. Pages are served sequentially; set
// NextToken on non-final pages so the paginator continues.
type mockDescribeTagsAPI struct {
	pages     []ec2.DescribeTagsOutput
	err       error
	pageIndex int
	// createTags fields for later reuse by task 7/8 tests.
	createInputs []*ec2.CreateTagsInput
	createErr    error
}

func (m *mockDescribeTagsAPI) DescribeTags(
	ctx context.Context, in *ec2.DescribeTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeTagsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.pageIndex >= len(m.pages) {
		return &ec2.DescribeTagsOutput{}, nil
	}
	page := m.pages[m.pageIndex]
	m.pageIndex++
	return &page, nil
}

func (m *mockDescribeTagsAPI) CreateTags(
	ctx context.Context, in *ec2.CreateTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.CreateTagsOutput, error) {
	m.createInputs = append(m.createInputs, in)
	return &ec2.CreateTagsOutput{}, m.createErr
}

var _ = Describe("existingTags", func() {
	It("returns keys and values", func() {
		mock := &mockDescribeTagsAPI{
			pages: []ec2.DescribeTagsOutput{{
				Tags: []ec2types.TagDescription{
					{Key: aws.String("Name"), Value: aws.String("my-ami")},
					{Key: aws.String("Env"), Value: aws.String("prod")},
				},
			}},
		}
		got, err := existingTags(context.Background(), mock, "ami-abc")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveLen(2))
		Expect(got).To(HaveKeyWithValue("Name", "my-ami"))
		Expect(got).To(HaveKeyWithValue("Env", "prod"))
	})

	It("collects across paginated results", func() {
		mock := &mockDescribeTagsAPI{
			pages: []ec2.DescribeTagsOutput{
				{
					Tags: []ec2types.TagDescription{
						{Key: aws.String("K1"), Value: aws.String("V1")},
					},
					NextToken: aws.String("page2"),
				},
				{
					Tags: []ec2types.TagDescription{
						{Key: aws.String("K2"), Value: aws.String("V2")},
					},
				},
			},
		}
		got, err := existingTags(context.Background(), mock, "ami-xyz")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveLen(2))
		Expect(got).To(HaveKeyWithValue("K1", "V1"))
		Expect(got).To(HaveKeyWithValue("K2", "V2"))
	})

	It("returns empty non-nil map for AMI with no tags", func() {
		mock := &mockDescribeTagsAPI{
			pages: []ec2.DescribeTagsOutput{{}},
		}
		got, err := existingTags(context.Background(), mock, "ami-empty")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).NotTo(BeNil())
		Expect(got).To(BeEmpty())
	})

	It("returns error wrapping the AMI ID", func() {
		mock := &mockDescribeTagsAPI{
			err: fmt.Errorf("simulated API error"),
		}
		_, err := existingTags(context.Background(), mock, "ami-err")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("ami-err"))
	})
})

var _ = Describe("AMITags rendering", func() {
	var base interpolate.Context

	BeforeEach(func() {
		base = interpolate.Context{}
	})

	It("renders per-AMI tags with .AMITags.Test and index for non-identifier keys", func() {
		tags := map[string]string{
			"Copied":  "{{ .AMITags.Test }}",
			"Indexed": `{{ index .AMITags "a/b c" }}`,
		}
		for _, n := range []string{"ami1", "ami2"} {
			data := map[string]interface{}{
				"AMITags": map[string]string{
					"Test":  "value-from-" + n,
					"a/b c": "special-key-" + n,
				},
			}
			got, errs := interpolateTags(tags, base, data)
			Expect(errs).To(BeEmpty())
			Expect(got).To(HaveKeyWithValue("Copied", "value-from-"+n))
			Expect(got).To(HaveKeyWithValue("Indexed", "special-key-"+n))
		}
	})

	It("uses reserved AMITags over generated-data collision", func() {
		tags := map[string]string{"Tag": "{{ .AMITags.Name }}"}
		data := map[string]interface{}{
			"AMITags":   map[string]string{"Name": "stale-generated"},
			"SourceAMI": "ami-aaaa",
		}
		// Override with real AMI tags (as PostProcess will do).
		data["AMITags"] = map[string]string{"Name": "real-ami-name"}

		got, errs := interpolateTags(tags, base, data)
		Expect(errs).To(BeEmpty())
		Expect(got).To(HaveKeyWithValue("Tag", "real-ami-name"))
	})

	It("errors on template reference with nil generated data", func() {
		data := normalizeData(nil)
		Expect(data).NotTo(BeNil())
		Expect(data).To(BeEmpty())

		tags := map[string]string{"T": "{{ .SourceAMI }}"}
		_, errs := interpolateTags(tags, base, data)
		Expect(errs).To(HaveLen(1))
		Expect(strings.Contains(errs[0].Error(), "SourceAMI")).To(BeTrue())
	})
})
