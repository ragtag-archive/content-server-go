//go:build nooverride
// +build nooverride

package main

func GetUpstreamOverrides() map[string]OverrideConfig {
	return map[string]OverrideConfig{}
}
