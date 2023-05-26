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
	listenAddress := os.Getenv("LISTEN_ADDRESS")
	gd := NewGoogleDrive(GoogleDriveConfig{
		RefreshToken:  os.Getenv("GD_REFRESH_TOKEN"),
		DefaultRootID: os.Getenv("GD_DEFAULT_ROOT_ID"),
		ClientID:      os.Getenv("GD_CLIENT_ID"),
		ClientSecret:  os.Getenv("GD_CLIENT_SECRET"),
	}, false)

	http.HandleFunc("/_health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		fmt.Fprint(w, "ok")
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)

		u, _ := url.Parse(r.URL.String())
		path := u.Path

		defaultRootId := gd.Config.DefaultRootID
		if strings.HasPrefix(path, "/gd:") {
			parts := strings.Split(path, "/")
			defaultRootId = parts[1][3:]
			path = "/" + strings.Join(parts[2:], "/")
		}

		fileName := path[strings.LastIndex(path, "/")+1:]

		if strings.HasSuffix(path, "/") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Fetch file
		fileResponse, err := gd.DownloadByPath(path, r.Header.Get("Range"), defaultRootId)
		if err != nil || fileResponse == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
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
	})

	log.Println("HTTP webserver running. Access it at", listenAddress)
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
