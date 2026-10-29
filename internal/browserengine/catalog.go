// Package browserengine downloads and inventories official browser distributions.
// It never executes a browser, installer, archive utility, or system configuration.
package browserengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	Chrome          = "chrome-for-testing"
	Firefox         = "firefox"
	Platform        = "win64"
	chromeCatalog   = "https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json"
	mozillaVersions = "https://product-details.mozilla.org/1.0/firefox_versions.json"
	mozillaArchive  = "https://archive.mozilla.org/pub/firefox/releases/"
	maxMetadata     = 64 << 20
)

var (
	ErrInput       = errors.New("browser_engine_input_refused")
	ErrTransport   = errors.New("browser_engine_explicit_transport_required")
	ErrSource      = errors.New("browser_engine_official_source_failed")
	ErrArchive     = errors.New("browser_engine_archive_refused")
	ErrStore       = errors.New("browser_engine_store_refused")
	ErrExists      = errors.New("browser_engine_version_already_exists")
	ErrIntegrity   = errors.New("browser_engine_integrity_failed")
	ErrUnsupported = errors.New("browser_engine_platform_unsupported")
	chromeVersion  = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$`)
	foxVersion     = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(?:\.[0-9]{1,3})?(?:esr)?$`)
)

// Release is public display data, never an authorization to download its URL.
// DownloadVersion resolves the selected ID/version again from official metadata.
type Release struct {
	EngineID string `json:"engineId"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Channel  string `json:"channel"`
	Source   string `json:"source"`
	Format   string `json:"format"`
	License  string `json:"license"`
}
type Catalog struct {
	Releases  []Release `json:"releases"`
	FetchedAt time.Time `json:"fetchedAt"`
	Sources   []string  `json:"sources"`
}
type resolved struct {
	Release
	url, checksum, checksumSource, prefix, executable, metadataSHA256 string
}
type chromeData struct {
	Version   string `json:"version"`
	Downloads struct {
		Chrome []struct {
			Platform string `json:"platform"`
			URL      string `json:"url"`
		} `json:"chrome"`
	} `json:"downloads"`
}

// HTTP clients must be explicitly supplied with a transport. No implicit
// http.DefaultTransport, environment proxy, or direct fallback is constructed.
// The caller owns route authorization (for example its verified tunnel proxy).
func cloneClient(client *http.Client) (*http.Client, error) {
	if client == nil || client.Transport == nil {
		return nil, ErrTransport
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	copy.Timeout = 10 * time.Minute
	copy.Jar = nil
	return &copy, nil
}
func allowedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Port() != "" {
		return false
	}
	if u.RawPath != "" && u.EscapedPath() != u.RawPath {
		return false
	}
	switch u.Host {
	case "googlechromelabs.github.io":
		return strings.HasPrefix(u.Path, "/chrome-for-testing/")
	case "storage.googleapis.com":
		return strings.HasPrefix(u.Path, "/chrome-for-testing-public/")
	case "product-details.mozilla.org":
		return raw == mozillaVersions
	case "archive.mozilla.org":
		return strings.HasPrefix(u.Path, "/pub/firefox/releases/")
	}
	return false
}
func (m *Manager) fetch(ctx context.Context, raw string, limit int64) ([]byte, error) {
	var buffer bytes.Buffer
	if _, err := m.transfer(ctx, raw, &buffer, limit); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func (m *Manager) transfer(ctx context.Context, raw string, output io.Writer, limit int64) (int64, error) {
	if m.client == nil || !allowedURL(raw) {
		return 0, ErrSource
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, ErrSource
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "GodUseVPN-Ultra-EngineCatalog/1")
	response, err := m.client.Do(req)
	if err != nil {
		return 0, ErrSource
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > limit || response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return 0, ErrSource
	}
	n, err := io.Copy(output, io.LimitReader(response.Body, limit+1))
	if err != nil || n > limit || response.ContentLength >= 0 && n != response.ContentLength || ctx.Err() != nil {
		return n, ErrSource
	}
	return n, nil
}
func metadataDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func strictJSON(data []byte, result any) error {
	// Official feeds may add fields; duplicate keys are nevertheless rejected.
	decoder := json.NewDecoder(bytes.NewReader(data))
	var object any
	if err := uniqueJSON(decoder, &object); err != nil {
		return ErrSource
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrSource
	}
	if json.Unmarshal(data, result) != nil {
		return ErrSource
	}
	return nil
}
func uniqueJSON(d *json.Decoder, result *any) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return ErrSource
				}
				seen[name] = true
				var v any
				if err := uniqueJSON(d, &v); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				var v any
				if err := uniqueJSON(d, &v); err != nil {
					return err
				}
			}
		default:
			return ErrSource
		}
		end, err := d.Token()
		if err != nil || delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
			return ErrSource
		}
	}
	return nil
}
func chromeRelease(data chromeData) (resolved, error) {
	if !chromeVersion.MatchString(data.Version) {
		return resolved{}, ErrSource
	}
	expected := "https://storage.googleapis.com/chrome-for-testing-public/" + data.Version + "/win64/chrome-win64.zip"
	count := 0
	for _, download := range data.Downloads.Chrome {
		if download.Platform == Platform {
			if download.URL != expected {
				return resolved{}, ErrSource
			}
			count++
		}
	}
	if count != 1 {
		return resolved{}, ErrSource
	}
	return resolved{Release: Release{Chrome, data.Version, Platform, "known-good", chromeCatalog, "zip", "Google Chrome executable terms; bundled component notices retained"}, url: expected, prefix: "chrome-win64/", executable: "chrome.exe"}, nil
}

// RefreshCatalog is an explicit network operation, never called by a constructor.
// Chrome versions come from CfT; Firefox release/ESR versions come from the
// official archive directory and current-channel JSON, with installers read as
// 7z archives later. Old catalog entries are not recommendations to run them.
func (m *Manager) RefreshCatalog(ctx context.Context) (Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	chromeBytes, err := m.fetch(ctx, chromeCatalog, maxMetadata)
	if err != nil {
		return Catalog{}, err
	}
	var chrome struct {
		Versions []chromeData `json:"versions"`
	}
	if strictJSON(chromeBytes, &chrome) != nil || len(chrome.Versions) == 0 || len(chrome.Versions) > 50000 {
		return Catalog{}, ErrSource
	}
	result := Catalog{FetchedAt: time.Now().UTC(), Sources: []string{chromeCatalog, mozillaVersions, mozillaArchive}}
	seen := map[string]bool{}
	for _, version := range chrome.Versions {
		release, err := chromeRelease(version)
		if err != nil {
			for _, download := range version.Downloads.Chrome {
				if download.Platform == Platform {
					return Catalog{}, ErrSource
				}
			}
			continue
		}
		key := Chrome + "/" + release.Version
		if seen[key] {
			return Catalog{}, ErrSource
		}
		seen[key] = true
		result.Releases = append(result.Releases, release.Release)
	}
	versionsBytes, err := m.fetch(ctx, mozillaVersions, 128<<10)
	if err != nil {
		return Catalog{}, err
	}
	var channels map[string]string
	if strictJSON(versionsBytes, &channels) != nil {
		return Catalog{}, ErrSource
	}
	current := map[string]string{}
	for _, field := range []string{"LATEST_FIREFOX_VERSION", "FIREFOX_ESR", "FIREFOX_ESR115"} {
		if version := channels[field]; version != "" {
			if !foxVersion.MatchString(version) {
				return Catalog{}, ErrSource
			}
			current[version] = field
		}
	}
	if len(current) == 0 {
		return Catalog{}, ErrSource
	}
	archive, err := m.fetch(ctx, mozillaArchive, 4<<20)
	if err != nil {
		return Catalog{}, err
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(archive))
	foxSeen := map[string]bool{}
	for {
		typ := tokenizer.Next()
		if typ == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return Catalog{}, ErrSource
			}
			break
		}
		if typ != html.StartTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data != "a" {
			continue
		}
		for _, attr := range token.Attr {
			if attr.Key != "href" {
				continue
			}
			value := strings.TrimSuffix(attr.Val, "/")
			value = strings.TrimPrefix(value, "/pub/firefox/releases/")
			if !foxVersion.MatchString(value) || foxSeen[value] {
				continue
			}
			foxSeen[value] = true
			channel := "archive"
			if label := current[value]; label != "" {
				channel = label
			}
			result.Releases = append(result.Releases, firefoxRelease(value, channel).Release)
		}
	}
	for version := range current {
		if !foxSeen[version] {
			return Catalog{}, ErrSource
		}
	}
	if len(foxSeen) == 0 || len(result.Releases) > 60000 {
		return Catalog{}, ErrSource
	}
	sort.Slice(result.Releases, func(i, j int) bool {
		a, b := result.Releases[i], result.Releases[j]
		if a.EngineID != b.EngineID {
			return a.EngineID < b.EngineID
		}
		return versionLess(b.Version, a.Version)
	})
	return result, nil
}
func versionLess(a, b string) bool {
	var av, bv [4]int
	for i, s := range strings.Split(strings.TrimSuffix(a, "esr"), ".") {
		if i < 4 {
			fmt.Sscan(s, &av[i])
		}
	}
	for i, s := range strings.Split(strings.TrimSuffix(b, "esr"), ".") {
		if i < 4 {
			fmt.Sscan(s, &bv[i])
		}
	}
	for i := range av {
		if av[i] != bv[i] {
			return av[i] < bv[i]
		}
	}
	return a < b
}
func firefoxRelease(version, channel string) resolved {
	return resolved{Release: Release{Firefox, version, Platform, channel, mozillaArchive + version + "/", "7z-sfx-core", "Mozilla executable distribution terms and MPL/component notices retained"}, url: mozillaArchive + version + "/win64/en-US/Firefox%20Setup%20" + version + ".exe", prefix: "core/", executable: "firefox.exe", checksumSource: mozillaArchive + version + "/SHA512SUMS"}
}
func (m *Manager) resolve(ctx context.Context, id, version string) (resolved, error) {
	switch id {
	case Chrome:
		if !chromeVersion.MatchString(version) {
			return resolved{}, ErrInput
		}
		raw, err := m.fetch(ctx, "https://googlechromelabs.github.io/chrome-for-testing/"+version+".json", 2<<20)
		if err != nil {
			return resolved{}, err
		}
		var data chromeData
		if strictJSON(raw, &data) != nil || data.Version != version {
			return resolved{}, ErrSource
		}
		release, err := chromeRelease(data)
		release.metadataSHA256 = metadataDigest(raw)
		return release, err
	case Firefox:
		if !foxVersion.MatchString(version) {
			return resolved{}, ErrInput
		}
		release := firefoxRelease(version, "archive")
		sums, err := m.fetch(ctx, release.checksumSource, 8<<20)
		if err != nil {
			return resolved{}, err
		}
		target := "win64/en-US/Firefox Setup " + version + ".exe"
		count := 0
		for _, line := range strings.Split(string(sums), "\n") {
			parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "  ", 2)
			if len(parts) == 2 && parts[1] == target {
				hash, err := hex.DecodeString(parts[0])
				if err != nil || len(hash) != 64 {
					return resolved{}, ErrSource
				}
				release.checksum = strings.ToLower(parts[0])
				count++
			}
		}
		if count != 1 {
			return resolved{}, ErrSource
		}
		release.metadataSHA256 = metadataDigest(sums)
		return release, nil
	}
	return resolved{}, ErrInput
}
