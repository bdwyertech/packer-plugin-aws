package helpers

import "github.com/aws/aws-sdk-go-v2/aws"

// RegionalConfig returns a copy of base with Region overridden.
// base is not mutated.
func RegionalConfig(base *aws.Config, region string) aws.Config {
	cfg := base.Copy()
	cfg.Region = region
	return cfg
}
