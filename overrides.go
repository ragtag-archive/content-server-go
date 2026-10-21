package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// LoadUpstreamOverrides reads the upstream overrides from a TSV file with the
// columns: upstream tag, upstream URL, must proxy. Lines that do not match
// this format (headers, comments, blank lines) are ignored.
func LoadUpstreamOverrides(path string) (map[string]OverrideConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read overrides file: %w", err)
	}
	return ParseUpstreamOverrides(string(data)), nil
}

// ParseUpstreamOverrides parses the contents of an overrides TSV file
func ParseUpstreamOverrides(tsv string) map[string]OverrideConfig {
	overrideConfig := make(map[string]OverrideConfig)

	for _, overrideString := range strings.Split(tsv, "\n") {
		parts := strings.Split(strings.TrimSpace(overrideString), "\t")
		if len(parts) != 3 {
			continue
		}

		mustProxy, err := strconv.ParseBool(strings.TrimSpace(parts[2]))
		if err != nil {
			continue
		}

		overrideConfig[strings.TrimSpace(parts[0])] = OverrideConfig{
			UpstreamUrl: strings.TrimRight(strings.TrimSpace(parts[1]), "/"),
			MustProxy:   mustProxy,
		}
	}

	return overrideConfig
}
