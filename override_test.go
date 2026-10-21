package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUpstreamTag(t *testing.T) {
	assert := assert.New(t)

	tag, path := GetUpstreamTag("/gd:123/abc/def")
	assert.Equal("gd:123", tag)
	assert.Equal("/abc/def", path)

	tag, path = GetUpstreamTag("/abc/def")
	assert.Equal("", tag)
	assert.Equal("/abc/def", path)
}

func TestEscapeFilePath(t *testing.T) {
	assert := assert.New(t)

	for in, out := range map[string]string{
		"/abc/abc.webp":      "/abc/abc.webp",
		"/a-b_c/a.ja.vtt":    "/a-b_c/a.ja.vtt",
		"/a/b c.mp4":         "/a/b%20c.mp4",
		"/a/b?c.mp4":         "/a/b%3Fc.mp4",
		"/a/b#c.mp4":         "/a/b%23c.mp4",
		"/archive-202311/x/": "",
	} {
		escaped, err := EscapeFilePath(in)
		if out == "" {
			assert.Error(err, in)
		} else {
			assert.NoError(err, in)
			assert.Equal(out, escaped, in)
		}
	}

	for _, in := range []string{"", "abc", "/", "/a//b", "/a/../b", "/a/./b", "/..", "/a/\r\nb", "/a\\b"} {
		_, err := EscapeFilePath(in)
		assert.Error(err, in)
	}
}

func TestParseUpstreamOverrides(t *testing.T) {
	assert := assert.New(t)

	overrides := ParseUpstreamOverrides("UPSTREAM TAG\tUPSTREAM URL\tMUST PROXY\n" +
		"gd:abc\thttps://cdn.example.com/archive\tfalse\n" +
		"s3:def\thttps://s3.example.com/\ttrue\n" +
		"\n# comment\n")

	assert.Len(overrides, 2)
	assert.Equal(OverrideConfig{UpstreamUrl: "https://cdn.example.com/archive", MustProxy: false}, overrides["gd:abc"])
	assert.Equal(OverrideConfig{UpstreamUrl: "https://s3.example.com", MustProxy: true}, overrides["s3:def"])
}

func TestHandler(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Path", r.URL.EscapedPath())
		w.Header().Set("X-Upstream-Range", r.Header.Get("Range"))
		w.Header().Set("X-Amz-Request-Id", "secret")
		w.Write([]byte("hello"))
	}))
	defer upstream.Close()

	handler := NewHandler(map[string]OverrideConfig{
		"gd:redirect": {UpstreamUrl: "https://cdn.example.com/archive", MustProxy: false},
		"s3:proxy":    {UpstreamUrl: upstream.URL + "/bucket", MustProxy: true},
	}, NewMetrics())

	do := func(method, target string, headers map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	t.Run("redirect", func(t *testing.T) {
		rec := do("GET", "/gd:redirect/abc/abc.webp", nil)
		assert.Equal(t, http.StatusFound, rec.Code)
		assert.Equal(t, "https://cdn.example.com/archive/abc/abc.webp", rec.Header().Get("Location"))
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("proxy", func(t *testing.T) {
		rec := do("GET", "/s3:proxy/abc/abc.webm", map[string]string{"Range": "bytes=0-0"})
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "hello", rec.Body.String())
		assert.Equal(t, "/bucket/abc/abc.webm", rec.Header().Get("X-Upstream-Path"))
		assert.Equal(t, "bytes=0-0", rec.Header().Get("X-Upstream-Range"))
		assert.Equal(t, "", rec.Header().Get("X-Amz-Request-Id"))
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, `inline; filename="abc.webm"`, rec.Header().Get("Content-Disposition"))
	})

	t.Run("preflight", func(t *testing.T) {
		rec := do("OPTIONS", "/s3:proxy/abc/abc.webm", nil)
		assert.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "Range", rec.Header().Get("Access-Control-Allow-Headers"))
	})

	t.Run("unknown upstream", func(t *testing.T) {
		for _, target := range []string{
			"/abc/abc.webp",
			"/gd:root/secret.txt",
			"/gd:x'%20in%20parents%20or%20'x/a",
			"/UC2L0MeiLsfJf4rH_xTVteDw/profile.jpg",
		} {
			assert.Equal(t, http.StatusNotFound, do("GET", target, nil).Code, target)
		}
	})

	t.Run("path traversal", func(t *testing.T) {
		for _, target := range []string{
			"/gd:redirect/..%2f..%2fx",
			"/gd:redirect/abc%2f..%2f..%2fx",
			"/s3:proxy/a/%2e%2e/b",
		} {
			assert.Equal(t, http.StatusNotFound, do("GET", target, nil).Code, target)
		}
	})

	t.Run("directory listing", func(t *testing.T) {
		assert.Equal(t, http.StatusUnauthorized, do("GET", "/gd:redirect/abc/", nil).Code)
	})

	t.Run("no pprof", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, do("GET", "/debug/pprof/cmdline", nil).Code)
	})

	t.Run("method", func(t *testing.T) {
		assert.Equal(t, http.StatusMethodNotAllowed, do("POST", "/gd:redirect/abc/abc.webp", nil).Code)
	})
}
