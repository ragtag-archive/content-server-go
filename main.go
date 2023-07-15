package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func main() {
	overrides := GetUpstreamOverrides()

	listenAddress := os.Getenv("LISTEN_ADDRESS")
	gd := NewGoogleDrive(GoogleDriveConfig{
		RefreshToken:  os.Getenv("GD_REFRESH_TOKEN"),
		DefaultRootID: os.Getenv("GD_DEFAULT_ROOT_ID"),
		ClientID:      os.Getenv("GD_CLIENT_ID"),
		ClientSecret:  os.Getenv("GD_CLIENT_SECRET"),
	}, false)

	metrics := NewMetrics()

	http.HandleFunc("/_health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		fmt.Fprint(w, "ok")
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		metrics.ServeHTTP(w, r)
	})

	http.HandleFunc("/", metrics.WithMetrics(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)

		u, _ := url.Parse(r.URL.String())

		// Parse the upstream tag and the path
		upstreamTag, path := GetUpstreamTag(u.Path)
		if upstreamTag == "" {
			upstreamTag = fmt.Sprintf("gd:%s", gd.Config.DefaultRootID)
		}

		// Disallow directory listing
		if strings.HasSuffix(path, "/") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		defaultRootId := gd.Config.DefaultRootID
		if strings.HasPrefix(upstreamTag, "gd:") {
			defaultRootId = upstreamTag[3:]
		}

		fileName := path[strings.LastIndex(path, "/")+1:]

		var fileResponse *http.Response
		var err error
		httpClient := &http.Client{}

		// Check first if the file is available from the upstream
		if override, ok := overrides[upstreamTag]; ok {
			upstreamUrl := override.UpstreamUrl + path
			upstreamResponse, err := httpClient.Head(upstreamUrl)
			if err == nil && upstreamResponse.StatusCode == http.StatusOK {
				log.Printf("Using upstream override URL %s\n", upstreamUrl)

				// Check whether to redirect or proxy (Sec-Fetch-Site header)
				if override.MustProxy || r.Header.Get("Sec-Fetch-Mode") == "cors" {
					// Cross-site request, proxy
					u, err := url.Parse(upstreamUrl)
					if err != nil {
						log.Printf("Error parsing upstream URL %s: %s\n", upstreamUrl, err.Error())
						http.Error(w, "Not found", http.StatusNotFound)
						return
					}

					req := &http.Request{
						Method: http.MethodGet,
						URL:    u,
					}

					if r.Header.Get("Range") != "" {
						req.Header = make(http.Header)
						req.Header.Set("Range", r.Header.Get("Range"))
					}

					fileResponse, err = httpClient.Do(req)
					if err != nil {
						log.Printf("Error fetching upstream URL %s: %s\n", upstreamUrl, err.Error())
						http.Error(w, "Not found", http.StatusNotFound)
						return
					}
				} else {
					// Not a cross-site request, redirect
					http.Redirect(w, r, upstreamUrl, http.StatusFound)
					return
				}
			} else {
				log.Printf("Upstream override URL returned %d: %s\n", upstreamResponse.StatusCode, upstreamUrl)
			}
		}

		// Fetch file
		if fileResponse == nil {
			fileResponse, err = gd.DownloadByPath(path, r.Header.Get("Range"), defaultRootId)
			if err != nil || fileResponse == nil {
				http.Error(w, "Not found", http.StatusNotFound)
				return
			}
		}
		defer fileResponse.Body.Close()

		// Modify headers
		fileResponse.Header.Del("X-GUploader-UploadID")
		fileResponse.Header.Del("X-Goog-Hash")
		fileResponse.Header.Del("X-Amz-Request-Id")
		fileResponse.Header.Del("X-Amz-Expiration")
		fileResponse.Header.Del("X-HW")
		fileResponse.Header.Set("Content-Disposition", `inline; filename="`+url.QueryEscape(fileName)+`"`)
		fileResponse.Header.Set("Access-Control-Allow-Origin", "*")
		fileResponse.Header.Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS")
		fileResponse.Header.Set("Access-Control-Allow-Headers", "Range")
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

		// Write status code to client
		w.WriteHeader(fileResponse.StatusCode)

		// Write response to client
		io.Copy(w, fileResponse.Body)
	}))

	log.Println("HTTP webserver running. Access it at", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
