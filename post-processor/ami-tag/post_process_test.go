package ami_tag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ebs "github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeArtifact implements packer.Artifact for PostProcess tests.
type fakeArtifact struct {
	builderId     string
	id            string
	generatedData interface{}
}

func (a *fakeArtifact) BuilderId() string { return a.builderId }
func (a *fakeArtifact) Id() string        { return a.id }
func (a *fakeArtifact) Files() []string   { return nil }
func (a *fakeArtifact) String() string    { return a.id }
func (a *fakeArtifact) State(name string) interface{} {
	if name == "generated_data" {
		return a.generatedData
	}
	return nil
}
func (a *fakeArtifact) Destroy() error { return nil }

// describeErrMock embeds mockDescribeTagsAPI but errors after errAfter calls.
type describeErrMock struct {
	mockDescribeTagsAPI
	calls    int
	errAfter int
	descErr  error
}

func (m *describeErrMock) DescribeTags(
	ctx context.Context, in *ec2.DescribeTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeTagsOutput, error) {
	m.calls++
	if m.calls > m.errAfter {
		return nil, m.descErr
	}
	return m.mockDescribeTagsAPI.DescribeTags(ctx, in, optFns...)
}

// orderMock embeds mockDescribeTagsAPI and records call order.
type orderMock struct {
	mockDescribeTagsAPI
	log []string
}

func (m *orderMock) DescribeTags(
	ctx context.Context, in *ec2.DescribeTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeTagsOutput, error) {
	m.log = append(m.log, "describe")
	return m.mockDescribeTagsAPI.DescribeTags(ctx, in, optFns...)
}

func (m *orderMock) CreateTags(
	ctx context.Context, in *ec2.CreateTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.CreateTagsOutput, error) {
	m.log = append(m.log, "create")
	return m.mockDescribeTagsAPI.CreateTags(ctx, in, optFns...)
}

// retryDescribeMock returns different DescribeTags results on
// successive calls, for testing AMITags retry logic.
type retryDescribeMock struct {
	mockDescribeTagsAPI
	totalCalls int
	callTags   map[int][]ec2types.TagDescription
}

func (m *retryDescribeMock) DescribeTags(
	ctx context.Context, in *ec2.DescribeTagsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeTagsOutput, error) {
	m.totalCalls++
	if tags, ok := m.callTags[m.totalCalls]; ok {
		return &ec2.DescribeTagsOutput{Tags: tags}, nil
	}
	return &ec2.DescribeTagsOutput{}, nil
}

var _ = Describe("PostProcess", func() {
	var (
		p    *PostProcessor
		mock *mockDescribeTagsAPI
		ui   *packer.MockUi
	)

	newArtifact := func(data interface{}) *fakeArtifact {
		return &fakeArtifact{
			builderId:     ebs.BuilderId,
			id:            "us-east-1:ami-abc123",
			generatedData: data,
		}
	}

	configure := func(tags map[string]interface{}) {
		p = &PostProcessor{
			newClient: func(_ aws.Config, _ string) tagAPI {
				return mock
			},
			retryDelay: func(_ uint) time.Duration { return 0 },
		}
		cfg := baseConfigMap(tags)
		cfg["skip_credential_validation"] = true
		ExpectWithOffset(1, p.Configure(cfg)).To(Succeed())
	}

	BeforeEach(func() {
		mock = &mockDescribeTagsAPI{
			pages: []ec2.DescribeTagsOutput{{}},
		}
		ui = &packer.MockUi{}
	})

	It("both map shapes give identical CreateTags input", func() {
		configure(map[string]interface{}{"Name": "{{ .SourceAMIName }}"})
		shapes := []interface{}{
			map[string]interface{}{"SourceAMIName": "base-ami"},
			map[interface{}]interface{}{"SourceAMIName": "base-ami"},
		}
		for _, data := range shapes {
			mock.createInputs = nil
			mock.pageIndex = 0
			_, _, _, err := p.PostProcess(
				context.Background(), ui, newArtifact(data))
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.createInputs).To(HaveLen(1))
			Expect(aws.ToString(
				mock.createInputs[0].Tags[0].Value)).To(Equal("base-ami"))
		}
	})

	It("nil data with template ref returns error and zero CreateTags", func() {
		configure(map[string]interface{}{"T": "{{ .SourceAMI }}"})
		_, _, _, err := p.PostProcess(
			context.Background(), ui, newArtifact(nil))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("SourceAMI"))
		Expect(mock.createInputs).To(BeEmpty())
	})

	It("over-length rendered value returns error and zero CreateTags", func() {
		configure(map[string]interface{}{"Name": "{{ .SourceAMIName }}"})
		long := strings.Repeat("x", 257)
		art := newArtifact(map[string]interface{}{"SourceAMIName": long})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("Name"))
		Expect(mock.createInputs).To(BeEmpty())
	})

	It("non-string values render via fmt.Sprint", func() {
		configure(map[string]interface{}{"Num": "{{ .NumericValue }}"})
		art := newArtifact(map[string]interface{}{"NumericValue": 42})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mock.createInputs[0].Tags[0].Value)).To(Equal("42"))
	})

	It("arbitrary generated key renders", func() {
		configure(map[string]interface{}{"C": "{{ .CustomBuilder_Thing }}"})
		art := newArtifact(
			map[string]interface{}{"CustomBuilder_Thing": "hello"})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mock.createInputs[0].Tags[0].Value)).To(Equal("hello"))
	})

	It("missing AMITags key that never appears fails with zero CreateTags", func() {
		configure(map[string]interface{}{
			"CopiedTag": "{{ .AMITags.NeverExists }}",
		})
		art := newArtifact(map[string]interface{}{})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("NeverExists"))
		Expect(mock.createInputs).To(BeEmpty())
	})

	It("DescribeTags error on second AMI makes zero CreateTags", func() {
		dm := &describeErrMock{
			mockDescribeTagsAPI: mockDescribeTagsAPI{
				pages: []ec2.DescribeTagsOutput{{}},
			},
			errAfter: 1,
			descErr:  fmt.Errorf("simulated describe error"),
		}
		p = &PostProcessor{
			newClient:  func(_ aws.Config, _ string) tagAPI { return dm },
			retryDelay: func(_ uint) time.Duration { return 0 },
		}
		cfg := baseConfigMap(map[string]interface{}{"Env": "prod"})
		cfg["skip_credential_validation"] = true
		Expect(p.Configure(cfg)).To(Succeed())
		art := &fakeArtifact{
			builderId:     ebs.BuilderId,
			id:            "us-east-1:ami-aaa,eu-west-1:ami-bbb",
			generatedData: map[string]interface{}{},
		}
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).To(HaveOccurred())
		Expect(dm.createInputs).To(BeEmpty())
	})

	It("all DescribeTags before first CreateTags with two AMIs", func() {
		om := &orderMock{
			mockDescribeTagsAPI: mockDescribeTagsAPI{
				pages: []ec2.DescribeTagsOutput{{}},
			},
		}
		p = &PostProcessor{
			newClient:  func(_ aws.Config, _ string) tagAPI { return om },
			retryDelay: func(_ uint) time.Duration { return 0 },
		}
		cfg := baseConfigMap(map[string]interface{}{"Env": "prod"})
		cfg["skip_credential_validation"] = true
		Expect(p.Configure(cfg)).To(Succeed())
		art := &fakeArtifact{
			builderId:     ebs.BuilderId,
			id:            "us-east-1:ami-aaa,eu-west-1:ami-bbb",
			generatedData: map[string]interface{}{},
		}
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		sawCreate := false
		for _, e := range om.log {
			if e == "create" {
				sawCreate = true
			}
			if e == "describe" {
				Expect(sawCreate).To(BeFalse(),
					"describe after create: %s",
					strings.Join(om.log, ","))
			}
		}
		Expect(om.log).To(ContainElement("create"))
	})

	It("over-length on any AMI prevents all tagging", func() {
		configure(map[string]interface{}{
			"Name": "{{ .SourceAMIName }}",
		})
		long := strings.Repeat("x", 257)
		art := &fakeArtifact{
			builderId:     ebs.BuilderId,
			id:            "us-east-1:ami-aaa,eu-west-1:ami-bbb",
			generatedData: map[string]interface{}{"SourceAMIName": long},
		}
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).To(HaveOccurred())
		Expect(mock.createInputs).To(BeEmpty())
	})

	It("rendered tags per AMI use that AMI's existing tags", func() {
		mocks := map[string]*mockDescribeTagsAPI{
			"us-east-1": {pages: []ec2.DescribeTagsOutput{{
				Tags: []ec2types.TagDescription{
					{Key: aws.String("Test"), Value: aws.String("val-aaa")},
				},
			}}},
			"eu-west-1": {pages: []ec2.DescribeTagsOutput{{
				Tags: []ec2types.TagDescription{
					{Key: aws.String("Test"), Value: aws.String("val-bbb")},
				},
			}}},
		}
		p = &PostProcessor{
			newClient: func(_ aws.Config, region string) tagAPI {
				return mocks[region]
			},
			retryDelay: func(_ uint) time.Duration { return 0 },
		}
		cfg := baseConfigMap(map[string]interface{}{
			"CopiedTag": "{{ .AMITags.Test }}",
		})
		cfg["skip_credential_validation"] = true
		Expect(p.Configure(cfg)).To(Succeed())
		art := &fakeArtifact{
			builderId:     ebs.BuilderId,
			id:            "us-east-1:ami-aaa,eu-west-1:ami-bbb",
			generatedData: map[string]interface{}{},
		}
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mocks["us-east-1"].createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mocks["us-east-1"].createInputs[0].Tags[0].Value,
		)).To(Equal("val-aaa"))
		Expect(mocks["eu-west-1"].createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mocks["eu-west-1"].createInputs[0].Tags[0].Value,
		)).To(Equal("val-bbb"))
	})

	It("missing AMITags key that appears on re-read succeeds", func() {
		rm := &retryDescribeMock{
			callTags: map[int][]ec2types.TagDescription{
				// Call 1 (initial read): empty tags
				// Call 2 (first retry read): tag appears
				2: {{
					Key:   aws.String("Test"),
					Value: aws.String("found-it"),
				}},
			},
		}
		p = &PostProcessor{
			newClient:  func(_ aws.Config, _ string) tagAPI { return rm },
			retryDelay: func(_ uint) time.Duration { return 0 },
		}
		cfg := baseConfigMap(map[string]interface{}{
			"CopiedTag": "{{ .AMITags.Test }}",
		})
		cfg["skip_credential_validation"] = true
		Expect(p.Configure(cfg)).To(Succeed())
		art := newArtifact(map[string]interface{}{})
		_, _, _, err := p.PostProcess(
			context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(rm.createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			rm.createInputs[0].Tags[0].Value,
		)).To(Equal("found-it"))
	})
})

var _ = Describe("backward compatibility", func() {
	var (
		p    *PostProcessor
		mock *mockDescribeTagsAPI
		ui   *packer.MockUi
	)

	newArtifact := func(data interface{}) *fakeArtifact {
		return &fakeArtifact{
			builderId:     ebs.BuilderId,
			id:            "us-east-1:ami-abc123",
			generatedData: data,
		}
	}

	configure := func(tags map[string]interface{}) {
		p = &PostProcessor{
			newClient: func(_ aws.Config, _ string) tagAPI {
				return mock
			},
			retryDelay: func(_ uint) time.Duration { return 0 },
		}
		cfg := baseConfigMap(tags)
		cfg["skip_credential_validation"] = true
		ExpectWithOffset(1, p.Configure(cfg)).To(Succeed())
	}

	BeforeEach(func() {
		mock = &mockDescribeTagsAPI{
			pages: []ec2.DescribeTagsOutput{{}},
		}
		ui = &packer.MockUi{}
	})

	It("literal-only tags make same DescribeTags and CreateTags calls as before", func() {
		configure(map[string]interface{}{
			"Env":  "prod",
			"Team": "platform",
		})
		art := newArtifact(map[string]interface{}{})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.pageIndex).To(Equal(1))
		Expect(mock.createInputs).To(HaveLen(1))
		tagMap := make(map[string]string)
		for _, t := range mock.createInputs[0].Tags {
			tagMap[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		Expect(tagMap).To(HaveKeyWithValue("Env", "prod"))
		Expect(tagMap).To(HaveKeyWithValue("Team", "platform"))
	})

	It("user function resolves via saved context", func() {
		configure(map[string]interface{}{
			"U": `{{ user "myvar" }}`,
		})
		p.config.ctx.UserVariables = map[string]string{
			"myvar": "user-value",
		}
		art := newArtifact(map[string]interface{}{})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mock.createInputs[0].Tags[0].Value)).To(Equal("user-value"))
	})

	It("build_name resolves via saved context", func() {
		configure(map[string]interface{}{
			"B": "{{ build_name }}",
		})
		p.config.ctx.BuildName = "my-build"
		art := newArtifact(map[string]interface{}{})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mock.createInputs[0].Tags[0].Value)).To(Equal("my-build"))
	})

	It("clean_resource_name resolves via awscommon.TemplateFuncs", func() {
		configure(map[string]interface{}{
			"C": `{{ clean_resource_name "my#ami#name" }}`,
		})
		art := newArtifact(map[string]interface{}{})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		Expect(mock.createInputs).To(HaveLen(1))
		Expect(aws.ToString(
			mock.createInputs[0].Tags[0].Value)).To(Equal("my-ami-name"))
	})

	It("logs contain no rendered values", func() {
		configure(map[string]interface{}{
			"Secret": "{{ .SourceAMIName }}",
		})
		art := newArtifact(map[string]interface{}{
			"SourceAMIName": "sensitive-value",
		})
		_, _, _, err := p.PostProcess(context.Background(), ui, art)
		Expect(err).NotTo(HaveOccurred())
		for _, msg := range ui.SayMessages {
			Expect(msg.Message).NotTo(
				ContainSubstring("sensitive-value"),
				"rendered value leaked in log: %s", msg.Message)
		}
		found := false
		for _, msg := range ui.SayMessages {
			if strings.Contains(msg.Message, "Secret") {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(),
			"tag key 'Secret' should appear in logs")
	})

	It("no new config fields or schema changes", func() {
		p = &PostProcessor{}
		cfg := baseConfigMap(map[string]interface{}{"Env": "prod"})
		cfg["skip_credential_validation"] = true
		Expect(p.Configure(cfg)).To(Succeed())
	})
})
