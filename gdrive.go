package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// AccessTokenExpiration is the duration until the access token expires
	AccessTokenExpiration = 3500 * time.Second

	// GoogleOauthTokenURL is the URL for Google's OAuth token
	GoogleOauthTokenURL = "https://www.googleapis.com/oauth2/v4/token"

	// GoogleDriveAPIURL is the base URL for Google Drive's v3 API
	GoogleDriveAPIURL = "https://www.googleapis.com/drive/v3/"
)

// GoogleDriveConfig represents the configuration for a GoogleDrive client
type GoogleDriveConfig struct {
	RefreshToken  string
	DefaultRootID string
	ClientID      string
	ClientSecret  string
}

// CacheItem represents an item in the cache
type CacheItem struct {
	Expires time.Time
	Data    interface{}
}

// GoogleDrive represents a client for Google Drive
type GoogleDrive struct {
	Config      GoogleDriveConfig
	AccessToken string
	Expires     time.Time
	SkipCache   bool

	cache     map[string]CacheItem
	cacheLock sync.RWMutex
	client    *http.Client
}

// NewGoogleDrive creates a new GoogleDrive client
func NewGoogleDrive(config GoogleDriveConfig, skipCache bool) *GoogleDrive {
	return &GoogleDrive{
		Config:      config,
		AccessToken: "",
		Expires:     time.Time{},
		SkipCache:   skipCache,
		cache:       map[string]CacheItem{},
		cacheLock:   sync.RWMutex{},
		client:      &http.Client{},
	}
}

func (gd *GoogleDrive) cacheGet(key string) (interface{}, bool) {
	gd.cacheLock.RLock()
	defer gd.cacheLock.RUnlock()

	val, found := gd.cache[key]
	if !found {
		return "", false
	}

	if time.Now().After(val.Expires) {
		delete(gd.cache, key)
		return "", false
	}

	return val.Data, true
}

func (gd *GoogleDrive) cacheSet(key string, val interface{}, ttl time.Duration) {
	gd.cacheLock.Lock()
	defer gd.cacheLock.Unlock()

	gd.cache[key] = CacheItem{
		Expires: time.Now().Add(ttl),
		Data:    val,
	}
}

func (gd *GoogleDrive) initialize() error {
	// Check if the token has expired
	if time.Now().Before(gd.Expires) {
		return nil
	}

	// Check from cache
	if tokenData, found := gd.cacheGet("gd:access-token"); found {
		tokenCache := tokenData.(map[string]string)
		accessToken := tokenCache["accessToken"]
		expires, _ := strconv.ParseInt(tokenCache["expires"], 10, 64)

		if time.Now().Before(time.Unix(expires, 0)) && accessToken != "" {
			// Cached token is still valid, use it
			gd.AccessToken = accessToken
			gd.Expires = time.Unix(expires, 0)
			return nil
		}
	}

	// Construct the request body
	data := url.Values{}
	data.Set("client_id", gd.Config.ClientID)
	data.Set("client_secret", gd.Config.ClientSecret)
	data.Set("refresh_token", gd.Config.RefreshToken)
	data.Set("grant_type", "refresh_token")

	// Make the request
	log.Println("Refreshing Google Drive access token")
	resp, err := http.Post(GoogleOauthTokenURL, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Decode the response
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// Access tokens normally expire after 3600 seconds
	gd.Expires = time.Now().Add(AccessTokenExpiration)
	gd.AccessToken = result["access_token"].(string)

	// Cache it
	gd.cacheSet("gd:access-token", map[string]string{
		"accessToken": gd.AccessToken,
		"expires":     strconv.FormatInt(gd.Expires.Unix(), 10),
	}, AccessTokenExpiration)

	return nil
}

func (gd *GoogleDrive) query(path string, query map[string]string, headers map[string]string) (*http.Response, error) {
	err := gd.initialize()
	if err != nil {
		return nil, err
	}

	qs := url.Values{}
	for k, v := range query {
		qs.Add(k, v)
	}

	req, err := http.NewRequest("GET", GoogleDriveAPIURL+path+"?"+qs.Encode(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+gd.AccessToken)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return gd.client.Do(req)
}

func (gd *GoogleDrive) getIdFromPath(path string, root string, skipCache bool) (string, error) {
	parts := []string{}
	for _, part := range strings.Split(path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}

	pathClean := strings.Join(parts, "/")
	cacheKey := "gd:path-id:" + root + ":" + pathClean

	if cachedId, found := gd.cacheGet(cacheKey); found && !skipCache {
		return cachedId.(string), nil
	}

	id := root
	for _, part := range parts {
		name := strings.Replace(part, "'", "\\'", -1)

		queryResponse, err := gd.query("files", map[string]string{
			"q":                         fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", id, name),
			"fields":                    "files(id)",
			"supportsAllDrives":         "true",
			"includeItemsFromAllDrives": "true",
		}, map[string]string{})
		if err != nil {
			return "", fmt.Errorf("failed to get file id from path: %s", err.Error())
		}
		defer queryResponse.Body.Close()

		var queryResponseData map[string]interface{}
		if err = json.NewDecoder(queryResponse.Body).Decode(&queryResponseData); err != nil {
			return "", fmt.Errorf("failed to decode file id: %s", err.Error())
		}

		if len(queryResponseData["files"].([]interface{})) == 0 {
			id = ""
			break
		} else {
			id = queryResponseData["files"].([]interface{})[0].(map[string]interface{})["id"].(string)
		}
	}

	if id != "" {
		gd.cacheSet(cacheKey, id, 86400*time.Second)
	}

	return id, nil
}

func (gd *GoogleDrive) download(id string, rangeParam string) (*http.Response, error) {
	return gd.query("files/"+id, map[string]string{
		"alt":                       "media",
		"supportsAllDrives":         "true",
		"includeItemsFromAllDrives": "true",
	}, map[string]string{
		"Range": rangeParam,
	})
}

func (gd *GoogleDrive) DownloadByPath(path string, rangeParam string, root string) (*http.Response, error) {
	id, err := gd.getIdFromPath(path, root, gd.SkipCache)
	if err != nil {
		return nil, err
	}

	return gd.download(id, rangeParam)
}
