//go:build !nooverride
// +build !nooverride

package main

import (
	_ "embed"
	"strconv"
	"strings"
)

//go:embed overrides.tsv
var overridesTsv string

func GetUpstreamOverrides() map[string]OverrideConfig {
	overrideConfig := make(map[string]OverrideConfig)
	overrideStrings := strings.Split(strings.TrimSpace(overridesTsv), "\n")

	for _, overrideString := range overrideStrings {
		parts := strings.Split(overrideString, "\t")
		if len(parts) != 3 {
			continue
		}

		mustProxy, err := strconv.ParseBool(parts[2])
		if err != nil {
			continue
		}

		overrideConfig[parts[0]] = OverrideConfig{
			UpstreamUrl: parts[1],
			MustProxy:   mustProxy,
		}
	}

	return overrideConfig
}
