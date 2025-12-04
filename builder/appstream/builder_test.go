package appstream

import (
	"testing"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestBuilder_Prepare(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]any
		wantErr   bool
		wantWarns bool
	}{
		{
			name: "minimal valid config",
			config: map[string]any{
				"name":              "test-builder",
				"source_image_name": "test-image",
				"instance_type":     "stream.standard.small",
				"communicator":      "winrm",
				"winrm_username":    "Administrator",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Builder{}
			_, warns, err := b.Prepare(tt.config)

			hasErr := err != nil
			if multiErr, ok := err.(*packersdk.MultiError); ok {
				hasErr = len(multiErr.Errors) > 0
			}

			if hasErr != tt.wantErr {
				t.Errorf("Prepare() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantWarns && len(warns) == 0 {
				t.Errorf("Prepare() expected warnings but got none")
			}
		})
	}
}
