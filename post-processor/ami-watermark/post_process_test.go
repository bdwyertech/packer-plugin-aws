package ami_watermark

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// stubSTS implements stsAPI and always returns the configured error.
type stubSTS struct{ err error }

func (s *stubSTS) GetCallerIdentity(
	_ context.Context,
	_ *sts.GetCallerIdentityInput,
	_ ...func(*sts.Options),
) (*sts.GetCallerIdentityOutput, error) {
	return nil, s.err
}

// stubWatermarkEC2 implements watermarkEC2API as a no-op.
type stubWatermarkEC2 struct{}

func (s *stubWatermarkEC2) AttachImageWatermark(
	_ context.Context,
	_ *ec2.AttachImageWatermarkInput,
	_ ...func(*ec2.Options),
) (*ec2.AttachImageWatermarkOutput, error) {
	return &ec2.AttachImageWatermarkOutput{}, nil
}

// fakeSTS implements stsAPI and returns a valid GetCallerIdentityOutput.
type fakeSTS struct{ arn string }

func (s *fakeSTS) GetCallerIdentity(
	_ context.Context,
	_ *sts.GetCallerIdentityInput,
	_ ...func(*sts.Options),
) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{
		Arn: aws.String(s.arn),
	}, nil
}

// attachCall records one AttachImageWatermark invocation.
type attachCall struct {
	region, imageID, watermarkName string
}

// fakeWatermarkEC2 implements watermarkEC2API, records calls, and returns success.
type fakeWatermarkEC2 struct {
	region string
	calls  []attachCall
}

func (f *fakeWatermarkEC2) AttachImageWatermark(
	_ context.Context,
	input *ec2.AttachImageWatermarkInput,
	_ ...func(*ec2.Options),
) (*ec2.AttachImageWatermarkOutput, error) {
	f.calls = append(f.calls, attachCall{
		region:        f.region,
		imageID:       aws.ToString(input.ImageId),
		watermarkName: aws.ToString(input.WatermarkName),
	})
	key := "123456789012:" + aws.ToString(input.WatermarkName)
	return &ec2.AttachImageWatermarkOutput{WatermarkKey: &key}, nil
}

var _ = Describe("PostProcess", func() {
	Context("nil whoami dereference when STS fails", func() {
		var (
			p        *PostProcessor
			artifact *mockArtifact
			ui       *mockUi
		)

		BeforeEach(func() {
			p = &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"watermark_names":            []string{"test-watermark"},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
				"access_key":                 "AKIAIOSFODNN7EXAMPLE",
				"secret_key":                 "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			})
			Expect(err).NotTo(HaveOccurred())

			p.stsClient = func(_ aws.Config) stsAPI {
				return &stubSTS{err: errors.New("STS auth failed")}
			}
			p.ec2Client = func(_ aws.Config) watermarkEC2API {
				return &stubWatermarkEC2{}
			}

			artifact = &mockArtifact{
				builderId: "mitchellh.amazonebs", // ebs.BuilderId
				id:        "us-east-1:ami-12345",
			}
			ui = &mockUi{}
		})

		It("returns an error instead of panicking", func() {
			_, _, _, err := p.PostProcess(
				context.Background(), ui, artifact,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("GetCallerIdentity"),
				ContainSubstring("STS auth failed"),
			))
		})
	})

	Context("happy path: AttachImageWatermark is called correctly", func() {
		var (
			p        *PostProcessor
			artifact *mockArtifact
			ui       *mockUi
			ec2Fake  *fakeWatermarkEC2
		)

		BeforeEach(func() {
			p = &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"watermark_names":            []string{"test-watermark"},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
				"access_key":                 "AKIAIOSFODNN7EXAMPLE",
				"secret_key":                 "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			})
			Expect(err).NotTo(HaveOccurred())

			p.stsClient = func(_ aws.Config) stsAPI {
				return &fakeSTS{arn: "arn:aws:iam::123456789012:user/test"}
			}
			p.ec2Client = func(cfg aws.Config) watermarkEC2API {
				ec2Fake = &fakeWatermarkEC2{region: cfg.Region}
				return ec2Fake
			}

			artifact = &mockArtifact{
				builderId: "mitchellh.amazonebs", // ebs.BuilderId
				id:        "us-east-1:ami-12345",
			}
			ui = &mockUi{}
		})

		It("returns no error and calls AttachImageWatermark once", func() {
			result, keep, forceOverride, err := p.PostProcess(
				context.Background(), ui, artifact,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(artifact))
			Expect(keep).To(BeFalse())
			Expect(forceOverride).To(BeFalse())

			Expect(ec2Fake).NotTo(BeNil())
			Expect(ec2Fake.calls).To(HaveLen(1))
			Expect(ec2Fake.calls[0].imageID).To(Equal("ami-12345"))
			Expect(ec2Fake.calls[0].region).To(Equal("us-east-1"))
			Expect(ec2Fake.calls[0].watermarkName).To(Equal("test-watermark"))
		})
	})
})
