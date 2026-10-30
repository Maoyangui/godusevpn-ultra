// Package browserprofiles stores non-secret browser drafts. Loading a record
// never authorizes a browser launch, network binding or runtime observation.
package browserprofiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

var (
	ErrInput        = errors.New("browser_profile_input_refused")
	ErrConflict     = errors.New("browser_profile_revision_conflict")
	ErrBusy         = errors.New("browser_profile_store_busy")
	ErrStore        = errors.New("browser_profile_store_untrusted_or_corrupt")
	ErrUnsupported  = errors.New("browser_profile_platform_or_user_unsupported")
	ErrClosed       = errors.New("browser_profile_store_closed")
	ErrNotFound     = errors.New("browser_profile_not_found")
	ErrFull         = errors.New("browser_profile_store_limit")
	idPattern       = regexp.MustCompile(`^p-[a-f0-9]{32}$`)
	revisionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
	chromeVersion   = regexp.MustCompile(`^[1-9][0-9]{0,4}(?:\.[0-9]{1,5}){3}$`)
	firefoxVersion  = regexp.MustCompile(`^[1-9][0-9]{0,2}\.[0-9]{1,3}(?:\.[0-9]{1,3})?(?:esr)?$`)
)

type EngineSelection struct {
	EngineID string `json:"engineId"`
	Version  string `json:"version"`
}

// ProfileMeta and ProfileExtras are the non-launch parts of a browser
// environment. They remain separate from browserhost.ProfileConfig so UI-only
// values cannot silently become runtime capabilities.
type ProfileMeta struct {
	Browser         string   `json:"browser,omitempty"`
	EnvironmentName string   `json:"environmentName,omitempty"`
	Group           string   `json:"group,omitempty"`
	Remark          string   `json:"remark,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}
type ProfileExtras struct {
	Storage     json.RawMessage `json:"storage,omitempty"`
	Tabs        json.RawMessage `json:"tabs,omitempty"`
	Proxy       json.RawMessage `json:"proxy,omitempty"`
	Fingerprint json.RawMessage `json:"fingerprint,omitempty"`
	Advanced    json.RawMessage `json:"advanced,omitempty"`
	Readback    json.RawMessage `json:"readback,omitempty"`
}
type SaveRequest struct {
	SchemaVersion    int             `json:"schemaVersion"`
	ID               string          `json:"id,omitempty"`
	ExpectedRevision string          `json:"expectedRevision,omitempty"`
	Name             string          `json:"name"`
	Meta             ProfileMeta     `json:"meta,omitempty"`
	Extras           ProfileExtras   `json:"extras,omitempty"`
	Config           json.RawMessage `json:"config"`
	Engine           EngineSelection `json:"engine"`
}
type Record struct {
	SchemaVersion            int             `json:"schemaVersion"`
	ID                       string          `json:"id"`
	Name                     string          `json:"name"`
	Meta                     ProfileMeta     `json:"meta,omitempty"`
	Extras                   ProfileExtras   `json:"extras,omitempty"`
	Revision                 string          `json:"revision"`
	Config                   json.RawMessage `json:"config"`
	Engine                   EngineSelection `json:"engine"`
	UpdatedAt                time.Time       `json:"updatedAt"`
	Applied                  bool            `json:"applied"`
	RequiresLaunchValidation bool            `json:"requiresLaunchValidation"`
	RecoveryNeeded           bool            `json:"recoveryNeeded"`
}
type Summary struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Meta           ProfileMeta     `json:"meta,omitempty"`
	Revision       string          `json:"revision"`
	Engine         EngineSelection `json:"engine"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	RecoveryNeeded bool            `json:"recoveryNeeded"`
}
type ListResult struct {
	Records []Summary `json:"records"`
	Applied bool      `json:"applied"`
}
type DeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

func validName(name string) bool {
	if !utf8.ValidString(name) || name == "" || strings.TrimSpace(name) != name || utf8.RuneCountInString(name) > 80 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 {
			return false
		}
	}
	return true
}
func validateEngine(e EngineSelection) bool {
	switch e.EngineID {
	case "webview2":
		return e.Version == ""
	case "chrome-for-testing":
		return chromeVersion.MatchString(e.Version)
	case "firefox":
		return firefoxVersion.MatchString(e.Version)
	}
	return false
}

// Reject duplicate/case-alias keys at every depth, not merely unknown struct
// fields. User JSON never supplies a persisted binding or native trust record.
func strictJSON(data []byte, v any) error {
	if len(data) == 0 || len(data) > 64<<10 || !utf8.Valid(data) {
		return ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return ErrInput
		}
		t, e := d.Token()
		if e != nil {
			return ErrInput
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				s, ok := key.(string)
				if e != nil || !ok || seen[strings.ToLower(s)] {
					return ErrInput
				}
				seen[strings.ToLower(s)] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
		} else if delim == '[' {
			for d.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
		} else {
			return ErrInput
		}
		_, e = d.Token()
		return e
	}
	if walk(0) != nil {
		return ErrInput
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInput
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrInput
	}
	return nil
}
func exactKeys(data []byte, allowed ...string) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return ErrInput
	}
	keys := map[string]bool{}
	for _, key := range allowed {
		keys[key] = true
	}
	for key := range object {
		if !keys[key] {
			return ErrInput
		}
	}
	return nil
}
func validateMeta(meta ProfileMeta) error {
	if utf8.RuneCountInString(meta.Browser) > 80 || utf8.RuneCountInString(meta.EnvironmentName) > 100 || utf8.RuneCountInString(meta.Group) > 100 || utf8.RuneCountInString(meta.Remark) > 2000 || len(meta.Tags) > 32 {
		return ErrInput
	}
	if meta.Browser != "" && meta.Browser != "sunbrowser" && meta.Browser != "flowerbrowser" && meta.Browser != "chromium" && meta.Browser != "firefox" && meta.Browser != "webview2" {
		return ErrInput
	}
	for _, value := range []string{meta.Browser, meta.EnvironmentName, meta.Group, meta.Remark} {
		if !utf8.ValidString(value) {
			return ErrInput
		}
		for _, r := range value {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 {
				return ErrInput
			}
		}
	}
	for _, tag := range meta.Tags {
		if !utf8.ValidString(tag) || strings.TrimSpace(tag) != tag || tag == "" || utf8.RuneCountInString(tag) > 64 {
			return ErrInput
		}
		for _, r := range tag {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == 0x2028 || r == 0x2029 {
				return ErrInput
			}
		}
	}
	return nil
}
func validateExtras(extras ProfileExtras) error {
	validateObject := func(value json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
		if len(value) > 24<<10 {
			return nil, ErrInput
		}
		if len(value) == 0 {
			return map[string]json.RawMessage{}, nil
		}
		var object map[string]json.RawMessage
		if strictJSON(value, &object) != nil || object == nil || exactKeys(value, allowed...) != nil {
			return nil, ErrInput
		}
		return object, nil
	}
	stringField := func(object map[string]json.RawMessage, key string, max int) error {
		value, ok := object[key]
		if !ok {
			return nil
		}
		var text string
		if json.Unmarshal(value, &text) != nil || !utf8.ValidString(text) || utf8.RuneCountInString(text) > max {
			return ErrInput
		}
		return nil
	}
	enumField := func(object map[string]json.RawMessage, key string, allowed ...string) error {
		value, ok := object[key]
		if !ok {
			return nil
		}
		var text string
		if json.Unmarshal(value, &text) != nil {
			return ErrInput
		}
		for _, item := range allowed {
			if text == item {
				return nil
			}
		}
		return ErrInput
	}
	boolField := func(object map[string]json.RawMessage, key string) error {
		value, ok := object[key]
		if !ok {
			return nil
		}
		var flag bool
		return func() error {
			if json.Unmarshal(value, &flag) != nil {
				return ErrInput
			}
			return nil
		}()
	}
	integerField := func(object map[string]json.RawMessage, key string, min, max int) error {
		value, ok := object[key]
		if !ok {
			return nil
		}
		var number int
		if json.Unmarshal(value, &number) != nil || number < min || number > max {
			return ErrInput
		}
		return nil
	}
	storage, e := validateObject(extras.Storage, "cookieFormat", "merge", "cookieCount", "cookieSessionOnly")
	if e != nil || enumField(storage, "cookieFormat", "json", "netscape", "name-value") != nil || boolField(storage, "merge") != nil || boolField(storage, "cookieSessionOnly") != nil || integerField(storage, "cookieCount", 0, 10000) != nil {
		return ErrInput
	}
	tabs, e := validateObject(extras.Tabs, "urls", "startupMode")
	if e != nil || enumField(tabs, "startupMode", "restore", "urls", "blank") != nil {
		return ErrInput
	}
	if raw, ok := tabs["urls"]; ok {
		var urls []string
		if json.Unmarshal(raw, &urls) != nil || len(urls) > 20 {
			return ErrInput
		}
		for _, value := range urls {
			parsed, parseErr := url.Parse(value)
			if parseErr != nil || !utf8.ValidString(value) || len(value) > 2048 || strings.ContainsAny(value, "\r\n\t") || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return ErrInput
			}
		}
	}
	proxy, e := validateObject(extras.Proxy, "mode", "type", "provider", "accountRef")
	if e != nil || enumField(proxy, "mode", "no-proxy", "custom", "saved", "dynamic", "integrated") != nil || enumField(proxy, "type", "none", "http", "https", "socks5") != nil || stringField(proxy, "provider", 80) != nil || stringField(proxy, "accountRef", 120) != nil {
		return ErrInput
	}
	fingerprint, e := validateObject(extras.Fingerprint, "webrtc", "timezone", "geo", "language", "uiLanguage", "resolution", "fonts", "noise", "webglMetadata", "webglVendor", "webglRenderer", "webgpu", "seed")
	if e != nil || enumField(fingerprint, "webrtc", "forward", "replace", "real", "disabled", "disable-non-proxied-udp") != nil || enumField(fingerprint, "timezone", "ip", "real", "custom") != nil || enumField(fingerprint, "geo", "ip", "custom", "forbidden", "real") != nil || enumField(fingerprint, "language", "ip", "custom", "real") != nil || enumField(fingerprint, "uiLanguage", "by-language", "real", "custom") != nil || enumField(fingerprint, "resolution", "random", "preset", "custom", "ua") != nil || enumField(fingerprint, "fonts", "default", "custom") != nil || enumField(fingerprint, "webglMetadata", "real", "custom") != nil || enumField(fingerprint, "webgpu", "webgl", "real", "disabled", "runtime") != nil || stringField(fingerprint, "webglVendor", 128) != nil || stringField(fingerprint, "webglRenderer", 256) != nil || stringField(fingerprint, "seed", 128) != nil {
		return ErrInput
	}
	if raw, ok := fingerprint["noise"]; ok {
		noise, e := validateObject(raw, "canvas", "webglImage", "audio", "media", "clientRects", "speechVoices")
		if e != nil || boolField(noise, "canvas") != nil || boolField(noise, "webglImage") != nil || boolField(noise, "audio") != nil || boolField(noise, "media") != nil || boolField(noise, "clientRects") != nil || boolField(noise, "speechVoices") != nil {
			return ErrInput
		}
	}
	advanced, e := validateObject(extras.Advanced, "cpu", "ram", "deviceName", "mac", "dnt", "portScan", "hardwareAcceleration", "tls", "teamApps", "dataSync", "browserSettings", "randomFingerprint", "allowedPorts")
	if e != nil || enumField(advanced, "cpu", "real", "custom") != nil || enumField(advanced, "ram", "real", "custom") != nil || enumField(advanced, "deviceName", "real", "custom") != nil || enumField(advanced, "mac", "real", "custom") != nil || enumField(advanced, "dnt", "default", "on", "off") != nil || enumField(advanced, "portScan", "enabled", "disabled") != nil || enumField(advanced, "hardwareAcceleration", "default", "enabled", "disabled") != nil || enumField(advanced, "tls", "default", "enabled", "disabled") != nil || enumField(advanced, "teamApps", "global", "custom", "disabled") != nil || enumField(advanced, "dataSync", "global", "custom", "disabled") != nil || enumField(advanced, "browserSettings", "global", "custom") != nil || boolField(advanced, "randomFingerprint") != nil {
		return ErrInput
	}
	if raw, ok := advanced["allowedPorts"]; ok {
		var ports []int
		if json.Unmarshal(raw, &ports) != nil || len(ports) > 32 {
			return ErrInput
		}
		for _, port := range ports {
			if port < 1 || port > 65535 {
				return ErrInput
			}
		}
	}
	readback, e := validateObject(extras.Readback, "state", "scope", "runtimeVerified", "warnings", "timestamp")
	if e != nil || enumField(readback, "state", "unknown", "configured", "partial", "verified", "unsupported") != nil || enumField(readback, "scope", "browser-profile", "launch", "runtime") != nil || boolField(readback, "runtimeVerified") != nil || stringField(readback, "scope", 80) != nil || stringField(readback, "timestamp", 80) != nil {
		return ErrInput
	}
	if raw, ok := readback["warnings"]; ok {
		var warnings []string
		if json.Unmarshal(raw, &warnings) != nil || len(warnings) > 32 {
			return ErrInput
		}
		for _, warning := range warnings {
			if !utf8.ValidString(warning) || utf8.RuneCountInString(warning) > 500 {
				return ErrInput
			}
		}
	}
	return nil
}
func decodeSave(data []byte) (SaveRequest, error) {
	var request SaveRequest
	if exactKeys(data, "schemaVersion", "id", "expectedRevision", "name", "meta", "extras", "config", "engine") != nil || strictJSON(data, &request) != nil || (request.SchemaVersion != 1 && request.SchemaVersion != 2) || !validName(request.Name) || !validateEngine(request.Engine) || validateMeta(request.Meta) != nil || validateExtras(request.Extras) != nil {
		return request, ErrInput
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(data, &object)
	_, hasID := object["id"]
	_, hasRevision := object["expectedRevision"]
	if hasID != hasRevision || hasID && (!idPattern.MatchString(request.ID) || !revisionPattern.MatchString(request.ExpectedRevision)) {
		return request, ErrInput
	}
	if exactKeys(object["engine"], "engineId", "version") != nil {
		return request, ErrInput
	}
	var engine map[string]json.RawMessage
	_ = json.Unmarshal(object["engine"], &engine)
	for _, key := range []string{"engineId", "version"} {
		var value any
		if json.Unmarshal(engine[key], &value) != nil {
			return request, ErrInput
		}
		if _, ok := value.(string); !ok {
			return request, ErrInput
		}
	}
	return request, nil
}

func canonicalDraft(data []byte, id string) (json.RawMessage, error) {
	if len(data) == 0 || len(data) > 32<<10 || exactKeys(data, "schemaVersion", "engineId", "profileId", "identity", "environment", "hardware", "region", "viewport", "webrtc", "webgl", "webgpu", "graphics", "permissions", "cookies", "exitCleanup", "randomizationSeed") != nil {
		return nil, ErrInput
	}
	var object map[string]json.RawMessage
	if strictJSON(data, &object) != nil || object == nil {
		return nil, ErrInput
	}
	var identity browserhost.ProfileIdentity
	if strictJSON(object["identity"], &identity) != nil || identity.ClientHints != nil || identity.Mode != "native" && identity.Mode != "generated" && identity.Mode != "manual" {
		return nil, ErrInput
	}
	var identityKeys map[string]json.RawMessage
	_ = json.Unmarshal(object["identity"], &identityKeys)
	for key := range identityKeys {
		if key != "mode" && key != "userAgent" {
			return nil, ErrInput
		}
	}
	object["profileId"], _ = json.Marshal(id)
	// This placeholder exists only during pure schema validation. It is never
	// persisted or returned, and cannot satisfy any live launch binding gate.
	object["binding"] = json.RawMessage(`{"mode":"tunnel","referenceId":"stored-draft","allowDirectFallback":false}`)
	full, _ := json.Marshal(object)
	var parsed browserhost.ProfileConfig
	if json.Unmarshal(full, &parsed) != nil {
		return nil, ErrInput
	}
	runtime := browserhost.RuntimeContext{Engine: browserhost.Engine{ID: parsed.EngineID, Available: true}}
	config, e := browserhost.DecodeProfile(full, runtime)
	if e != nil {
		return nil, ErrInput
	}
	b, e := json.Marshal(config)
	if e != nil {
		return nil, ErrInput
	}
	_ = json.Unmarshal(b, &object)
	delete(object, "binding")
	result, e := json.Marshal(object)
	if e != nil {
		return nil, ErrInput
	}
	return result, nil
}
