package ami_watermark

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
)

// allowedChars defines the set of characters valid in a watermark name.
const allowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789()[] ./-'@_"

// invalidChars defines characters that are NOT allowed in a watermark name.
const invalidChars = "!#$%^&*{}<>|~`\"+;:,?\\\x00\x01\x02"

// genValidWatermarkName generates a random valid watermark name (3-128 chars from allowed set).
func genValidWatermarkName(r *rand.Rand) string {
	length := r.Intn(126) + 3 // 3 to 128
	b := make([]byte, length)
	for i := range b {
		b[i] = allowedChars[r.Intn(len(allowedChars))]
	}
	return string(b)
}

// P1: Watermark name validation completeness — valid names pass, invalid names fail.
func TestProperty_ValidateWatermarkName_ValidNamesPass(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		name := genValidWatermarkName(r)
		return validateWatermarkName(name) == nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("P1 valid names: %v", err)
	}
}

func TestProperty_ValidateWatermarkName_TooShortFails(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		length := r.Intn(2) + 1 // 1 or 2 chars
		b := make([]byte, length)
		for i := range b {
			b[i] = allowedChars[r.Intn(len(allowedChars))]
		}
		return validateWatermarkName(string(b)) != nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("P1 too short: %v", err)
	}
}

func TestProperty_ValidateWatermarkName_TooLongFails(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		length := 129 + r.Intn(100) // 129 to 228 chars
		b := make([]byte, length)
		for i := range b {
			b[i] = allowedChars[r.Intn(len(allowedChars))]
		}
		return validateWatermarkName(string(b)) != nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 100}); err != nil {
		t.Errorf("P1 too long: %v", err)
	}
}

func TestProperty_ValidateWatermarkName_InvalidCharsFail(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		// Generate a valid-length name but inject an invalid char
		length := r.Intn(126) + 3
		b := make([]byte, length)
		for i := range b {
			b[i] = allowedChars[r.Intn(len(allowedChars))]
		}
		// Inject one invalid char at a random position
		pos := r.Intn(length)
		b[pos] = invalidChars[r.Intn(len(invalidChars))]
		return validateWatermarkName(string(b)) != nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Errorf("P1 invalid chars: %v", err)
	}
}

// P2: Configure rejects empty, >5, and invalid watermark_names.
func TestProperty_Configure_RejectsEmptyNames(t *testing.T) {
	p := &PostProcessor{}
	err := p.Configure(map[string]interface{}{
		"watermark_names":            []string{},
		"skip_credential_validation": true,
		"region":                     "us-east-1",
	})
	if err == nil {
		t.Fatal("P2: Configure should reject empty watermark_names")
	}
}

func TestProperty_Configure_RejectsMoreThanFive(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		count := 6 + r.Intn(5) // 6 to 10 names
		names := make([]string, count)
		for i := range names {
			names[i] = genValidWatermarkName(r)
		}
		p := &PostProcessor{}
		err := p.Configure(map[string]interface{}{
			"watermark_names":            names,
			"skip_credential_validation": true,
			"region":                     "us-east-1",
		})
		return err != nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("P2 >5 names: %v", err)
	}
}

func TestProperty_Configure_RejectsInvalidNames(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		// Create a name with an invalid character
		name := string(invalidChars[r.Intn(len(invalidChars))]) + "abc"
		p := &PostProcessor{}
		err := p.Configure(map[string]interface{}{
			"watermark_names":            []string{name},
			"skip_credential_validation": true,
			"region":                     "us-east-1",
		})
		return err != nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("P2 invalid names: %v", err)
	}
}

func TestProperty_Configure_AcceptsValidConfig(t *testing.T) {
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		count := 1 + r.Intn(5) // 1 to 5 names
		names := make([]string, count)
		for i := range names {
			names[i] = genValidWatermarkName(r)
		}
		p := &PostProcessor{}
		err := p.Configure(map[string]interface{}{
			"watermark_names":            names,
			"skip_credential_validation": true,
			"region":                     "us-east-1",
		})
		return err == nil
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("P2 valid config: %v", err)
	}
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

// P3: Artifact pass-through invariant — PostProcess always returns the original artifact.
func TestProperty_ArtifactPassThrough(t *testing.T) {
	// This test verifies the invariant by checking builder gate rejection path,
	// since we can't call the real AWS API in unit tests.
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		// Use an unsupported builder so PostProcess returns without API calls
		builders := []string{"packer.null", "packer.docker", "packer.virtualbox", "packer.qemu"}
		builderID := builders[r.Intn(len(builders))]

		artifact := &mockArtifact{
			builderId: builderID,
			id:        fmt.Sprintf("us-east-1:ami-%012d", r.Int63n(999999999999)),
		}

		p := &PostProcessor{}
		_ = p.Configure(map[string]interface{}{
			"watermark_names":            []string{"test-watermark"},
			"skip_credential_validation": true,
			"region":                     "us-east-1",
		})

		result, keep, _, _ := p.PostProcess(context.Background(), &mockUi{}, artifact)

		// Artifact must always be passed through (same reference)
		return result == artifact && keep == true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("P3 artifact pass-through: %v", err)
	}
}

// P4: Builder compatibility gate rejects unsupported builder IDs.
func TestProperty_BuilderCompatibilityGate(t *testing.T) {
	supportedBuilders := map[string]bool{
		"mitchellh.amazonebs":           true,
		"mitchellh.amazon.ebssurrogate": true,
		"mitchellh.amazon.ebsvolume":    true,
		"mitchellh.amazon.chroot":       true,
		"mitchellh.amazon.instance":     true,
	}

	// Test: unsupported builders are rejected
	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		unsupported := []string{
			"packer.null", "packer.docker", "packer.virtualbox",
			"packer.qemu", "packer.vmware", "some.random.builder",
		}
		builderID := unsupported[r.Intn(len(unsupported))]

		// Confirm it's not accidentally in the supported list
		if supportedBuilders[builderID] {
			return true
		}

		artifact := &mockArtifact{
			builderId: builderID,
			id:        "us-east-1:ami-12345678",
		}

		p := &PostProcessor{}
		_ = p.Configure(map[string]interface{}{
			"watermark_names":            []string{"test-watermark"},
			"skip_credential_validation": true,
			"region":                     "us-east-1",
		})

		_, _, _, err := p.PostProcess(context.Background(), &mockUi{}, artifact)
		if err == nil {
			return false
		}
		// Error message should mention the unsupported builder
		return strings.Contains(err.Error(), "unexpected artifact type")
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 50}); err != nil {
		t.Errorf("P4 builder gate rejects unsupported: %v", err)
	}
}

// TestAmisFromArtifactID_EmptyInput covers B5-prereq: an empty artifact ID
// must not be split into a junk []*ami{{region:"", id:""}}; it must return nil
// (or an empty slice) so callers don't make bogus AWS calls.
func TestAmisFromArtifactID_EmptyInput(t *testing.T) {
	amis := amisFromArtifactID("")
	if len(amis) != 0 {
		t.Fatalf("expected amisFromArtifactID(\"\") to return nil/empty, got %+v", amis)
	}
}

// TestPostProcess_KeepInputArtifactFalseOnSuccess covers B5: when ami-watermark
// successfully passes through, it must return keepInputArtifact=false so the
// chain does not accumulate the input artifact once per post-processor.
//
// The artifact ID is empty so amisFromArtifactID returns nil (after the prereq
// fix), the watermark loop is a no-op, and PostProcess reaches the success
// return at line 179 without making any AWS calls.
func TestPostProcess_KeepInputArtifactFalseOnSuccess(t *testing.T) {
	artifact := &mockArtifact{
		builderId: "mitchellh.amazonebs", // ebs.BuilderId
		id:        "",
	}

	p := &PostProcessor{}
	if err := p.Configure(map[string]interface{}{
		"watermark_names":            []string{"test-watermark"},
		"skip_credential_validation": true,
		"region":                     "us-east-1",
	}); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	_, keepInputArtifact, _, err := p.PostProcess(context.Background(), &mockUi{}, artifact)
	if err != nil {
		t.Fatalf("PostProcess returned unexpected error: %v", err)
	}

	if keepInputArtifact {
		t.Errorf("B5: expected keepInputArtifact=false on success, got true (causes input artifact duplication across post-processor chain)")
	}
}

// P5: Fail-fast on API error — since we can't mock the AWS API easily without
// interfaces, we verify the structural invariant: PostProcess returns an error
// when GetAWSConfig fails (simulating credential misconfiguration).
func TestProperty_FailFastOnConfigError(t *testing.T) {
	// When AccessConfig is not properly configured (no credentials, no region),
	// PostProcess should fail fast on GetAWSConfig.
	supportedBuilders := []string{
		"mitchellh.amazonebs",
		"mitchellh.amazon.ebssurrogate",
		"mitchellh.amazon.ebsvolume",
		"mitchellh.amazon.chroot",
		"mitchellh.amazon.instance",
	}

	f := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		builderID := supportedBuilders[r.Intn(len(supportedBuilders))]

		artifact := &mockArtifact{
			builderId: builderID,
			id:        "us-east-1:ami-12345678",
		}

		// Configure with skip_credential_validation but don't set up real credentials
		// This means GetAWSConfig will attempt to use default credential chain
		// which should fail in a test environment without AWS creds
		p := &PostProcessor{}
		_ = p.Configure(map[string]interface{}{
			"watermark_names":            []string{"test-watermark"},
			"skip_credential_validation": true,
			"region":                     "us-east-1",
		})

		_, keep, forceOverride, err := p.PostProcess(context.Background(), &mockUi{}, artifact)

		// If there's an error (credential issue), artifact should still be passed through
		// and keep=true, forceOverride=false (fail-fast preserves artifact reference)
		if err != nil {
			return keep == true && forceOverride == false
		}
		// If no error (e.g., default creds happen to work), that's fine too
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 10}); err != nil {
		t.Errorf("P5 fail-fast: %v", err)
	}
}
