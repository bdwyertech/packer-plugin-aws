package main

import (
	"fmt"
	"os"

	builder "github.com/bdwyer/packer-plugin-aws/builder/appstream"
	datasource "github.com/bdwyer/packer-plugin-aws/datasource/appstream-image-builder"
	postprocessor "github.com/bdwyer/packer-plugin-aws/post-processor/appstream-share"
	"github.com/bdwyer/packer-plugin-aws/version"
	"github.com/hashicorp/packer-plugin-sdk/plugin"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterDatasource("appstream-image-builder", new(datasource.Datasource))
	pps.RegisterBuilder("appstream-image-builder", new(builder.Builder))
	pps.RegisterPostProcessor("appstream-share", new(postprocessor.PostProcessor))
	pps.SetVersion(version.PluginVersion)
	err := pps.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
