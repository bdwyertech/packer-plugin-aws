package ami_watermark

import (
	"context"
	"fmt"
	"io"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"pgregory.net/rapid"
)

// allowedChars defines the set of characters valid in a watermark name.
const allowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789()[] ./-'@_"

// invalidChars defines characters that are NOT allowed in a watermark name.
const invalidChars = "!#$%^&*{}<>|~`\"+;:,?\\\x00\x01\x02"

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

// drawValidWatermarkName draws a random watermark name with length in [minLen, maxLen]
// using only characters from the allowedChars set.
func drawValidWatermarkName(t *rapid.T, minLen, maxLen int) string {
	n := rapid.IntRange(minLen, maxLen).Draw(t, "length")
	b := make([]byte, n)
	for i := range b {
		b[i] = allowedChars[rapid.IntRange(0, len(allowedChars)-1).Draw(t, "")]
	}
	return string(b)
}

// P1: Watermark name validation completeness — valid names pass, invalid names fail.
var _ = Describe("validateWatermarkName", func() {
	Context("P1: validation completeness", func() {
		It("accepts valid names (3–128 chars from allowed set)", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				name := drawValidWatermarkName(t, 3, 128)
				Expect(validateWatermarkName(name)).To(Succeed())
			})
		})

		It("rejects names that are too short (1–2 chars)", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				name := drawValidWatermarkName(t, 1, 2)
				Expect(validateWatermarkName(name)).To(HaveOccurred())
			})
		})

		It("rejects names that are too long (129+ chars)", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				name := drawValidWatermarkName(t, 129, 228)
				Expect(validateWatermarkName(name)).To(HaveOccurred())
			})
		})

		It("rejects names containing invalid characters", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				n := rapid.IntRange(3, 128).Draw(t, "length")
				b := make([]byte, n)
				for i := range b {
					b[i] = allowedChars[rapid.IntRange(0, len(allowedChars)-1).Draw(t, "")]
				}
				pos := rapid.IntRange(0, n-1).Draw(t, "pos")
				b[pos] = invalidChars[rapid.IntRange(0, len(invalidChars)-1).Draw(t, "")]
				Expect(validateWatermarkName(string(b))).To(HaveOccurred())
			})
		})
	})
})

// P2: Configure rejects empty, >5, and invalid watermark_names.
var _ = Describe("Configure", func() {
	Context("P2: watermark_names validation", func() {
		It("rejects empty watermark_names", func() {
			p := &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"watermark_names":            []string{},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
			})
			Expect(err).To(HaveOccurred())
		})

		It("rejects more than five names", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				count := rapid.IntRange(6, 10).Draw(t, "count")
				names := make([]string, count)
				for i := range names {
					names[i] = drawValidWatermarkName(t, 3, 128)
				}
				p := &PostProcessor{}
				err := p.Configure(map[string]interface{}{
					"watermark_names":            names,
					"skip_credential_validation": true,
					"region":                     "us-east-1",
				})
				Expect(err).To(HaveOccurred())
			})
		})

		It("rejects invalid names", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				idx := rapid.IntRange(0, len(invalidChars)-1).Draw(t, "charIdx")
				name := string(invalidChars[idx]) + "abc"
				p := &PostProcessor{}
				err := p.Configure(map[string]interface{}{
					"watermark_names":            []string{name},
					"skip_credential_validation": true,
					"region":                     "us-east-1",
				})
				Expect(err).To(HaveOccurred())
			})
		})

		It("accepts valid config with 1–5 names", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				count := rapid.IntRange(1, 5).Draw(t, "count")
				names := make([]string, count)
				for i := range names {
					names[i] = drawValidWatermarkName(t, 3, 128)
				}
				p := &PostProcessor{}
				err := p.Configure(map[string]interface{}{
					"watermark_names":            names,
					"skip_credential_validation": true,
					"region":                     "us-east-1",
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

var _ = Describe("PostProcess", func() {
	// P3: Artifact pass-through invariant — PostProcess always returns the original artifact.
	Context("P3: artifact pass-through on builder rejection", func() {
		It("always returns the original artifact with keep=true", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				builders := []string{"packer.null", "packer.docker", "packer.virtualbox", "packer.qemu"}
				idx := rapid.IntRange(0, len(builders)-1).Draw(t, "builderIdx")
				amiNum := rapid.Int64Range(0, 999999999999).Draw(t, "amiNum")

				artifact := &mockArtifact{
					builderId: builders[idx],
					id:        fmt.Sprintf("us-east-1:ami-%012d", amiNum),
				}

				p := &PostProcessor{}
				_ = p.Configure(map[string]interface{}{
					"watermark_names":            []string{"test-watermark"},
					"skip_credential_validation": true,
					"region":                     "us-east-1",
				})

				result, keep, _, _ := p.PostProcess(context.Background(), &mockUi{}, artifact)
				Expect(result).To(Equal(artifact))
				Expect(keep).To(BeTrue())
			})
		})
	})

	// P4: Builder compatibility gate rejects unsupported builder IDs.
	Context("P4: builder compatibility gate", func() {
		It("rejects unsupported builders with 'unexpected artifact type' error", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				unsupported := []string{
					"packer.null", "packer.docker", "packer.virtualbox",
					"packer.qemu", "packer.vmware", "some.random.builder",
				}
				idx := rapid.IntRange(0, len(unsupported)-1).Draw(t, "builderIdx")

				artifact := &mockArtifact{
					builderId: unsupported[idx],
					id:        "us-east-1:ami-12345678",
				}

				p := &PostProcessor{}
				_ = p.Configure(map[string]interface{}{
					"watermark_names":            []string{"test-watermark"},
					"skip_credential_validation": true,
					"region":                     "us-east-1",
				})

				_, _, _, err := p.PostProcess(context.Background(), &mockUi{}, artifact)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unexpected artifact type"))
			})
		})
	})

	// keepInputArtifact on success — when ami-watermark succeeds,
	// keepInputArtifact must be false so the chain does not accumulate input artifacts.
	Context("keepInputArtifact on success", func() {
		It("returns keepInputArtifact=false", func() {
			artifact := &mockArtifact{
				builderId: "mitchellh.amazonebs",
				id:        "",
			}

			p := &PostProcessor{}
			err := p.Configure(map[string]interface{}{
				"watermark_names":            []string{"test-watermark"},
				"skip_credential_validation": true,
				"region":                     "us-east-1",
				"access_key":                 "AKIAIOSFODNN7EXAMPLE",
				"secret_key":                 "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			})
			Expect(err).NotTo(HaveOccurred())

			_, keepInputArtifact, _, err := p.PostProcess(
				context.Background(), &mockUi{}, artifact,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(keepInputArtifact).To(BeFalse(),
				"causes input artifact duplication across post-processor chain")
		})
	})

	// P5: Fail-fast on API error — PostProcess returns keep=true, forceOverride=false on error.
	Context("P5: fail-fast on config error", func() {
		It("preserves artifact with keep=true, forceOverride=false on error", func() {
			rapid.Check(GinkgoT(), func(t *rapid.T) {
				supportedBuilders := []string{
					"mitchellh.amazonebs",
					"mitchellh.amazon.ebssurrogate",
					"mitchellh.amazon.ebsvolume",
					"mitchellh.amazon.chroot",
					"mitchellh.amazon.instance",
				}
				idx := rapid.IntRange(0, len(supportedBuilders)-1).Draw(t, "builderIdx")

				artifact := &mockArtifact{
					builderId: supportedBuilders[idx],
					id:        "us-east-1:ami-12345678",
				}

				p := &PostProcessor{}
				_ = p.Configure(map[string]interface{}{
					"watermark_names":            []string{"test-watermark"},
					"skip_credential_validation": true,
					"region":                     "us-east-1",
				})

				_, keep, forceOverride, err := p.PostProcess(
					context.Background(), &mockUi{}, artifact,
				)
				if err != nil {
					Expect(keep).To(BeTrue())
					Expect(forceOverride).To(BeFalse())
				}
			})
		})
	})
})

var _ = Describe("helpers.ParseArtifactID", func() {
	It("returns ErrEmptyArtifactID for empty input", func() {
		amis, err := helpers.ParseArtifactID("")
		Expect(err).To(MatchError(helpers.ErrEmptyArtifactID))
		Expect(amis).To(BeEmpty())
	})
})
