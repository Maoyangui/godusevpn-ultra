package browserprofiles

import (
	"encoding/json"
	"strings"
)

// LaunchRequest is the complete v2 browser launch envelope. The envelope is
// deliberately separate from browserhost.ProfileConfig: metadata and extras
// must be validated before a host is allowed to interpret the runtime config.
type LaunchRequest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Config        json.RawMessage `json:"config"`
	Meta          ProfileMeta     `json:"meta"`
	Extras        ProfileExtras   `json:"extras"`
	Engine        EngineSelection `json:"engine"`
}

// CapabilityReceipt is returned for every syntactically valid v2 envelope.
// Accepted=false is explicit: a caller must not treat an unsupported extra as
// a best-effort setting or continue to launch a partially applied profile.
type CapabilityReceipt struct {
	SchemaVersion int      `json:"schemaVersion"`
	Accepted      bool     `json:"accepted"`
	Unsupported   []string `json:"unsupported,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

func rawObjectHasKeys(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && len(object) != 0
}

// ParseLaunchRequest validates the v2 boundary without touching a browser,
// profile store, filesystem, process, proxy, or network. It rejects unknown
// top-level/config fields and returns a receipt for valid-but-unimplemented
// extras, so those fields cannot be silently discarded.
func ParseLaunchRequest(data []byte) (LaunchRequest, CapabilityReceipt, error) {
	var request LaunchRequest
	receipt := CapabilityReceipt{SchemaVersion: 2}
	if exactKeys(data, "schemaVersion", "config", "meta", "extras", "engine") != nil || strictJSON(data, &request) != nil || request.SchemaVersion != 2 {
		return request, receipt, ErrInput
	}
	if len(request.Config) == 0 || len(request.Config) > 32<<10 || exactKeys(request.Config,
		"schemaVersion", "engineId", "profileId", "identity", "environment", "hardware", "region", "viewport", "webrtc", "webgl", "webgpu", "graphics", "permissions", "cookies", "exitCleanup", "randomizationSeed") != nil {
		return request, receipt, ErrInput
	}
	if !validateEngine(request.Engine) || validateMeta(request.Meta) != nil || validateExtras(request.Extras) != nil {
		return request, receipt, ErrInput
	}
	var configObject map[string]json.RawMessage
	if strictJSON(request.Config, &configObject) != nil || configObject == nil {
		return request, receipt, ErrInput
	}
	var configSchema int
	if json.Unmarshal(configObject["schemaVersion"], &configSchema) != nil || configSchema != 1 {
		return request, receipt, ErrInput
	}
	var configEngine string
	if json.Unmarshal(configObject["engineId"], &configEngine) != nil || configEngine == "" {
		return request, receipt, ErrInput
	}
	canonical := func(id string) string {
		if id == "webview" {
			return "webview2"
		}
		return id
	}
	if canonical(configEngine) != canonical(request.Engine.EngineID) {
		return request, receipt, ErrInput
	}
	// A caller cannot provide a binding, service PID, relay credentials,
	// executable path, or arbitrary arguments in the v2 config. The trusted
	// Windows controller supplies its own binding after the envelope passes.
	unsupported := make([]string, 0, 8)
	for path, raw := range map[string]json.RawMessage{
		"extras.storage":     request.Extras.Storage,
		"extras.tabs":        request.Extras.Tabs,
		"extras.proxy":       request.Extras.Proxy,
		"extras.fingerprint": request.Extras.Fingerprint,
		"extras.advanced":    request.Extras.Advanced,
		"extras.readback":    request.Extras.Readback,
	} {
		if rawObjectHasKeys(raw) {
			unsupported = append(unsupported, path)
		}
	}
	// Keep deterministic receipts for UI and audit logs.
	for i := 0; i < len(unsupported); i++ {
		for j := i + 1; j < len(unsupported); j++ {
			if strings.Compare(unsupported[j], unsupported[i]) < 0 {
				unsupported[i], unsupported[j] = unsupported[j], unsupported[i]
			}
		}
	}
	receipt.Unsupported = unsupported
	receipt.Accepted = len(unsupported) == 0
	if len(request.Meta.Browser)+len(request.Meta.EnvironmentName)+len(request.Meta.Group)+len(request.Meta.Remark)+len(request.Meta.Tags) > 0 {
		receipt.Warnings = []string{"meta is accepted as profile metadata; it does not change browser runtime behavior"}
	}
	return request, receipt, nil
}
