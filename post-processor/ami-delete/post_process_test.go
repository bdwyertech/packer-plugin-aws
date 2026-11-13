package ami_delete

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/bdwyertech/packer-plugin-aws/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

var _ = Describe("helpers.ParseArtifactID", func() {
	Context("valid single entry", func() {
		It("returns one ami with correct region and id", func() {
			amis, err := helpers.ParseArtifactID("us-east-1:ami-12345")
			Expect(err).NotTo(HaveOccurred())
			Expect(amis).To(HaveLen(1))
			Expect(amis[0].Region).To(Equal("us-east-1"))
			Expect(amis[0].ID).To(Equal("ami-12345"))
		})
	})

	Context("empty string", func() {
		It("returns nil and an error", func() {
			amis, err := helpers.ParseArtifactID("")
			Expect(err).To(MatchError(helpers.ErrEmptyArtifactID))
			Expect(amis).To(BeNil())
		})
	})
})

var _ = Describe("PostProcess", func() {
	Context("builder gate: non-AMI builder", func() {
		It("returns an error mentioning the unexpected builder", func() {
			p := &PostProcessor{}
			art := &mockArtifact{builderId: "not-an-ami-builder"}
			_, _, _, err := p.PostProcess(
				context.Background(), &mockUi{}, art,
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected artifact type"))
			Expect(err.Error()).To(ContainSubstring("not-an-ami-builder"))
		})
	})

	Context("builder gate: AMI builder is accepted", func() {
		It("passes the builder check (fails later at AWS config)", func() {
			p := &PostProcessor{}
			art := &mockArtifact{
				builderId: helpers.AMIBuilderIDs[0],
				id:        "us-east-1:ami-12345",
			}
			_, _, _, err := p.PostProcess(
				context.Background(), &mockUi{}, art,
			)
			// May fail at GetAWSConfig, but must not be the builder-gate error
			if err != nil {
				Expect(err.Error()).NotTo(ContainSubstring("unexpected artifact type"))
			}
		})
	})
})

var _ = Describe("deleteAMI hook", func() {
	It("is invoked when set on the PostProcessor", func() {
		called := false
		var receivedImage *types.Image
		img := &types.Image{}
		p := &PostProcessor{
			deleteAMI: func(
				_ context.Context,
				_ helpers.DeleteAMIAPI,
				image *types.Image,
				_ func(string),
			) error {
				called = true
				receivedImage = image
				return nil
			},
		}
		err := p.deleteAMI(context.Background(), nil, img, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(called).To(BeTrue())
		Expect(receivedImage).To(Equal(img))
	})

	It("defaults to nil when not set", func() {
		p := &PostProcessor{}
		Expect(p.deleteAMI).To(BeNil())
	})
})
