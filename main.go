package main

import (
	"fmt"
	"os"

	"github.com/bdwyer/packer-plugin-aws-appstream/post-processor/appstream-share"
	"github.com/bdwyer/packer-plugin-aws-appstream/version"
	"github.com/hashicorp/packer-plugin-sdk/plugin"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterPostProcessor(plugin.DEFAULT_NAME, new(appstream.PostProcessor))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
