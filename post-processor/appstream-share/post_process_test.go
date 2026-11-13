package appstream

import (
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appstream"
	"github.com/aws/aws-sdk-go-v2/service/appstream/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// updatePermCall records one UpdateImagePermissions invocation.
type updatePermCall struct {
	imageName string
	accountID string
}

// fakeAppStream implements appstreamAPI for testing.
type fakeAppStream struct {
	describeResult *appstream.DescribeImagesOutput
	describeErr    error
	updateCalls    []updatePermCall
}

func (f *fakeAppStream) CopyImage(
	_ context.Context, _ *appstream.CopyImageInput, _ ...func(*appstream.Options),
) (*appstream.CopyImageOutput, error) {
	return &appstream.CopyImageOutput{}, nil
}

func (f *fakeAppStream) DescribeImages(
	_ context.Context, _ *appstream.DescribeImagesInput, _ ...func(*appstream.Options),
) (*appstream.DescribeImagesOutput, error) {
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	return f.describeResult, nil
}

func (f *fakeAppStream) UpdateImagePermissions(
	_ context.Context, params *appstream.UpdateImagePermissionsInput, _ ...func(*appstream.Options),
) (*appstream.UpdateImagePermissionsOutput, error) {
	f.updateCalls = append(f.updateCalls, updatePermCall{
		imageName: aws.ToString(params.Name),
		accountID: aws.ToString(params.SharedAccountId),
	})
	return &appstream.UpdateImagePermissionsOutput{}, nil
}

// mockArtifact implements packer.Artifact for testing.
type mockArtifact struct {
	builderId string
	id        string
}

func (a *mockArtifact) BuilderId() string             { return a.builderId }
func (a *mockArtifact) Id() string                    { return a.id }
func (a *mockArtifact) Files() []string               { return nil }
func (a *mockArtifact) String() string                { return a.id }
func (a *mockArtifact) State(name string) interface{} { return nil }
func (a *mockArtifact) Destroy() error                { return nil }

// mockUi implements packer.Ui for testing.
type mockUi struct{}

func (u *mockUi) Ask(string) (string, error)          { return "", nil }
func (u *mockUi) Askf(string, ...any) (string, error) { return "", nil }
func (u *mockUi) Say(string)                          {}
func (u *mockUi) Sayf(string, ...any)                 {}
func (u *mockUi) Message(string)                      {}
func (u *mockUi) Error(string)                        {}
func (u *mockUi) Errorf(string, ...any)               {}
func (u *mockUi) Machine(string, ...string)           {}
func (u *mockUi) TrackProgress(_ string, _, _ int64, stream io.ReadCloser) io.ReadCloser {
	return stream
}

var _ = Describe("PostProcess", func() {
	Context("sharing with configured accounts", func() {
		var (
			p    *PostProcessor
			fake *fakeAppStream
			ui   *mockUi
			art  *mockArtifact
		)

		BeforeEach(func() {
			p = &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"image_name":                 "test-image",
				"account_ids":                []string{"111111111111", "222222222222"},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
				"access_key":                 "AKIAIOSFODNN7EXAMPLE",
				"secret_key":                 "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			})
			Expect(err).NotTo(HaveOccurred())

			fake = &fakeAppStream{
				describeResult: &appstream.DescribeImagesOutput{
					Images: []types.Image{{
						Name:  aws.String("test-image"),
						State: types.ImageStateAvailable,
					}},
				},
			}
			p.appstreamClient = func(_ aws.Config) appstreamAPI {
				return fake
			}

			art = &mockArtifact{builderId: "appstream", id: "test-image"}
			ui = &mockUi{}
		})

		It("returns no error and calls UpdateImagePermissions once per account", func() {
			result, keep, forceOverride, err := p.PostProcess(
				context.Background(), ui, art,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(art))
			Expect(keep).To(BeTrue())
			Expect(forceOverride).To(BeFalse())

			Expect(fake.updateCalls).To(HaveLen(2))
			Expect(fake.updateCalls[0].imageName).To(Equal("test-image"))
			Expect(fake.updateCalls[0].accountID).To(Equal("111111111111"))
			Expect(fake.updateCalls[1].imageName).To(Equal("test-image"))
			Expect(fake.updateCalls[1].accountID).To(Equal("222222222222"))
		})
	})
})

// errFakeDescribe is a sentinel error for testing %w wrapping.
var errFakeDescribe = errors.New("fake describe error")

var _ = Describe("PostProcessor", func() {
	Context("appstreamClient hook", func() {
		It("is nil by default (production path)", func() {
			p := &PostProcessor{}
			Expect(p.appstreamClient).To(BeNil())
		})

		It("overrides client creation when set", func() {
			fake := &fakeAppStream{
				describeResult: &appstream.DescribeImagesOutput{
					Images: []types.Image{{
						Name:  aws.String("hook-image"),
						State: types.ImageStateAvailable,
					}},
				},
			}
			var hookCalled bool
			p := &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"image_name":                 "hook-image",
				"account_ids":                []string{"111111111111"},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
				"access_key":                 "AKIAIOSFODNN7EXAMPLE",
				"secret_key":                 "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			})
			Expect(err).NotTo(HaveOccurred())

			p.appstreamClient = func(_ aws.Config) appstreamAPI {
				hookCalled = true
				return fake
			}

			_, _, _, err = p.PostProcess(
				context.Background(), &mockUi{},
				&mockArtifact{builderId: "appstream", id: "hook-image"},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(hookCalled).To(BeTrue(),
				"appstreamClient hook should be invoked during PostProcess")
		})
	})

	Context("error wrapping with %w", func() {
		It("wraps DescribeImages errors matchable via errors.Is", func() {
			p := &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"image_name":                 "err-image",
				"account_ids":                []string{"111111111111"},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
				"access_key":                 "AKIAIOSFODNN7EXAMPLE",
				"secret_key":                 "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			})
			Expect(err).NotTo(HaveOccurred())

			p.appstreamClient = func(_ aws.Config) appstreamAPI {
				return &fakeAppStream{describeErr: errFakeDescribe}
			}

			_, _, _, err = p.PostProcess(
				context.Background(), &mockUi{},
				&mockArtifact{builderId: "appstream", id: "err-image"},
			)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, errFakeDescribe)).To(BeTrue(),
				"returned error should wrap the cause via %%w, matchable with errors.Is")
		})
	})
})
