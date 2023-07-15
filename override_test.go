package main_test

import (
	"testing"

	csg "github.com/ragtag-archive/content-server-go"
	"github.com/stretchr/testify/assert"
)

func TestGetUpstreamTag(t *testing.T) {
	assert := assert.New(t)

	tag, path := csg.GetUpstreamTag("/gd:123/abc/def")
	assert.Equal("gd:123", tag)
	assert.Equal("/abc/def", path)

	tag, path = csg.GetUpstreamTag("/abc/def")
	assert.Equal("", tag)
	assert.Equal("/abc/def", path)
}
