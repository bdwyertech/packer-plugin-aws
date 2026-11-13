package helpers

import (
	"errors"
	"fmt"
	"strings"
)

// AMIRef identifies one AMI in a Packer artifact ID.
type AMIRef struct{ Region, ID string }

var (
	// ErrEmptyArtifactID is returned when ParseArtifactID receives an empty string.
	ErrEmptyArtifactID = errors.New("artifact ID is empty")
	// ErrMalformedArtifact is returned when an entry lacks a colon
	// or has an empty region or ID.
	ErrMalformedArtifact = errors.New("malformed artifact entry")
)

// ParseArtifactID splits "region:ami-id[,region:ami-id...]" into AMIRefs.
// It never panics. Empty string returns ErrEmptyArtifactID. An entry without a
// colon, or with an empty region or ID, returns an error wrapping ErrMalformedArtifact.
func ParseArtifactID(id string) ([]AMIRef, error) {
	if id == "" {
		return nil, ErrEmptyArtifactID
	}
	entries := strings.Split(id, ",")
	refs := make([]AMIRef, 0, len(entries))
	for _, entry := range entries {
		region, amiID, ok := strings.Cut(entry, ":")
		if !ok || region == "" || amiID == "" {
			return nil, fmt.Errorf("%w: %q", ErrMalformedArtifact, entry)
		}
		refs = append(refs, AMIRef{Region: region, ID: amiID})
	}
	return refs, nil
}
