package main

import "strings"

type OverrideConfig struct {
	UpstreamUrl string
	MustProxy   bool
}

// GetUpstreamTag returns the upstream tag and the path to the file
// Example: /gd:123/abc -> gd:123, /abc
// Example: /abc/def -> "", /abc/def
func GetUpstreamTag(path string) (string, string) {
	upstreamTag := strings.TrimPrefix(path, "/")
	filePath := "/"
	nextSlash := strings.Index(upstreamTag, "/")
	if nextSlash != -1 {
		upstreamTag = upstreamTag[:nextSlash]
		filePath = path[nextSlash+1:]
	}
	if !strings.Contains(upstreamTag, ":") {
		return "", path
	}
	return upstreamTag, filePath
}
