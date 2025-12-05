package main

import (
	"fmt"
	"os"

	builder "github.com/bdwyertech/packer-plugin-aws/builder/appstream"
	ds_image "github.com/bdwyertech/packer-plugin-aws/datasource/appstream-image"
	ds_image_builder "github.com/bdwyertech/packer-plugin-aws/datasource/appstream-image-builder"
	ds_security_group "github.com/bdwyertech/packer-plugin-aws/datasource/security-group"
	ds_subnet "github.com/bdwyertech/packer-plugin-aws/datasource/subnet"
	ami_delete "github.com/bdwyertech/packer-plugin-aws/post-processor/ami-delete"
	pp_appstream_share "github.com/bdwyertech/packer-plugin-aws/post-processor/appstream-share"
	"github.com/bdwyertech/packer-plugin-aws/version"
	"github.com/hashicorp/packer-plugin-sdk/plugin"
)

func main() {
	pps := plugin.NewSet()
	pps.RegisterDatasource("appstream-image", new(ds_image.Datasource))
	pps.RegisterDatasource("appstream-image-builder", new(ds_image_builder.Datasource))
	pps.RegisterDatasource("security-group", new(ds_security_group.Datasource))
	pps.RegisterDatasource("subnet", new(ds_subnet.Datasource))
	pps.RegisterBuilder("appstream-image-builder", new(builder.Builder))
	pps.RegisterPostProcessor("appstream-share", new(pp_appstream_share.PostProcessor))
	pps.RegisterPostProcessor("ami-delete", new(ami_delete.PostProcessor))
	pps.SetVersion(version.PluginVersion)
	if err := pps.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
