package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var passHeaders []string = []string{"Accept", "Range", "Referer", "User-Agent"}

func main() {
	listenAddress := os.Getenv("LISTEN_ADDRESS")
	overridesFile := os.Getenv("OVERRIDES_FILE")
	if overridesFile == "" {
		overridesFile = "overrides.tsv"
	}

	overrides, err := LoadUpstreamOverrides(overridesFile)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Loaded %d upstream overrides from %s\n", len(overrides), overridesFile)

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           NewHandler(overrides, NewMetrics()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Println("HTTP webserver running. Access it at", listenAddress)
	log.Fatal(server.ListenAndServe())
}

// NewUpstreamClient creates the HTTP client used to proxy upstream files. It
// has no overall timeout because responses can be large video files that take
// a long time to stream; requests are cancelled when the client goes away.
func NewUpstreamClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// NewHandler creates the HTTP handler for the content server
func NewHandler(overrides map[string]OverrideConfig, metrics *Metrics) http.Handler {
	httpClient := NewUpstreamClient()
	mux := http.NewServeMux()

	mux.HandleFunc("/_health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		metrics.ServeHTTP(w, r)
	})

	mux.HandleFunc("/", metrics.WithMetrics(func(w http.ResponseWriter, r *http.Request) {
		var tStart, tBeginProxy, tEndProxy time.Time
		tStart = time.Now()
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)

		// CORS preflight
		if r.Method == http.MethodOptions {
			setCorsHeaders(w.Header())
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the upstream tag and the path
		upstreamTag, path := GetUpstreamTag(r.URL.Path)

		// Disallow directory listing
		if strings.HasSuffix(path, "/") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Only serve files from known upstreams
		override, ok := overrides[upstreamTag]
		if !ok {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		escapedPath, err := EscapeFilePath(path)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		upstreamUrl := override.UpstreamUrl + escapedPath
		fileName := path[strings.LastIndex(path, "/")+1:]
		log.Printf("Using upstream override URL %s, proxy: %v\n", upstreamUrl, override.MustProxy)

		if !override.MustProxy {
			// Redirect to the upstream
			setCorsHeaders(w.Header())
			http.Redirect(w, r, upstreamUrl, http.StatusFound)
			return
		}

		// Proxy from the upstream
		u, err := url.Parse(upstreamUrl)
		if err != nil {
			log.Printf("Error parsing upstream URL %s: %s\n", upstreamUrl, err.Error())
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), nil)
		if err != nil {
			log.Printf("Error creating request for upstream URL %s: %s\n", upstreamUrl, err.Error())
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		for _, header := range passHeaders {
			v := r.Header.Get(header)
			if v != "" {
				req.Header.Set(header, v)
			}
		}

		tBeginProxy = time.Now()
		fileResponse, err := httpClient.Do(req)
		if err != nil {
			log.Printf("Error fetching upstream URL %s: %s\n", upstreamUrl, err.Error())
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		defer fileResponse.Body.Close()
		tEndProxy = time.Now()

		// Modify headers
		fileResponse.Header.Del("X-GUploader-UploadID")
		fileResponse.Header.Del("X-Goog-Hash")
		fileResponse.Header.Del("X-Amz-Request-Id")
		fileResponse.Header.Del("X-Amz-Expiration")
		fileResponse.Header.Del("X-HW")
		fileResponse.Header.Set("Content-Disposition", `inline; filename="`+url.QueryEscape(fileName)+`"`)
		setCorsHeaders(fileResponse.Header)
		fileResponse.Header.Set("X-Content-Type-Options", "nosniff")
		if strings.HasSuffix(fileName, ".vtt") {
			fileResponse.Header.Set("Content-Type", "text/vtt")
		}
		if fileResponse.StatusCode >= 200 && fileResponse.StatusCode < 300 {
			fileResponse.Header.Set("Cache-Control", "public, max-age=604800, immutable")
		}

		// Write headers to client
		for k, v := range fileResponse.Header {
			w.Header().Set(k, v[0])
		}

		// Write performance metrics to client
		dTotal := time.Since(tStart)
		dProxy := tEndProxy.Sub(tBeginProxy)
		w.Header().Set("X-Content-Server-Go-Total", fmt.Sprintf("%d", dTotal.Milliseconds()))
		w.Header().Set("X-Content-Server-Go-Proxy", fmt.Sprintf("%d", dProxy.Milliseconds()))

		// Write status code to client
		w.WriteHeader(fileResponse.StatusCode)

		// Write response to client
		io.Copy(w, fileResponse.Body)
	}))

	return mux
}

func setCorsHeaders(h http.Header) {
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Range")
}
