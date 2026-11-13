package helpers

import (
	"slices"

	"github.com/hashicorp/packer-plugin-amazon/builder/chroot"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebs"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebssurrogate"
	"github.com/hashicorp/packer-plugin-amazon/builder/ebsvolume"
	"github.com/hashicorp/packer-plugin-amazon/builder/instance"
)

// AMIBuilderIDs lists the Amazon builders whose artifact ID is "region:ami-id,...".
// Do not mutate this slice.
var AMIBuilderIDs = []string{
	ebs.BuilderId,
	ebssurrogate.BuilderId,
	ebsvolume.BuilderId,
	chroot.BuilderId,
	instance.BuilderId,
}

// IsAMIBuilder reports whether id is in AMIBuilderIDs.
func IsAMIBuilder(id string) bool {
	return slices.Contains(AMIBuilderIDs, id)
}
