package main

import (
	"errors"
	"net/url"
	"strings"
)

type OverrideConfig struct {
	UpstreamUrl string
	MustProxy   bool
}

var ErrInvalidPath = errors.New("invalid path")

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

// EscapeFilePath validates a decoded file path and returns it with every
// segment escaped, so that it can be safely appended to an upstream URL.
// Empty, "." and ".." segments and control characters are rejected so that a
// request cannot escape the upstream prefix or inject a query or fragment.
func EscapeFilePath(path string) (string, error) {
	if !strings.HasPrefix(path, "/") {
		return "", ErrInvalidPath
	}

	segments := strings.Split(path[1:], "/")
	for i, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", ErrInvalidPath
		}
		for _, c := range segment {
			if c < 0x20 || c == 0x7f || c == '\\' {
				return "", ErrInvalidPath
			}
		}
		segments[i] = url.PathEscape(segment)
	}

	return "/" + strings.Join(segments, "/"), nil
}
