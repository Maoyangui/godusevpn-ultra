// Package browserprofiles stores non-secret browser drafts. Loading a record
// never authorizes a browser launch, network binding or runtime observation.
package browserprofiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
type SaveRequest struct {
	SchemaVersion    int             `json:"schemaVersion"`
	ID               string          `json:"id,omitempty"`
	ExpectedRevision string          `json:"expectedRevision,omitempty"`
	Name             string          `json:"name"`
	Config           json.RawMessage `json:"config"`
	Engine           EngineSelection `json:"engine"`
}
type Record struct {
	SchemaVersion            int             `json:"schemaVersion"`
	ID                       string          `json:"id"`
	Name                     string          `json:"name"`
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
func decodeSave(data []byte) (SaveRequest, error) {
	var request SaveRequest
	if exactKeys(data, "schemaVersion", "id", "expectedRevision", "name", "config", "engine") != nil || strictJSON(data, &request) != nil || request.SchemaVersion != 1 || !validName(request.Name) || !validateEngine(request.Engine) {
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
