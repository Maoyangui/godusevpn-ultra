package browserprofiles

import (
	"encoding/json"
	"testing"
)

func validLaunchEnvelope(t *testing.T) []byte {
	t.Helper()
	config := map[string]any{
		"schemaVersion": 1, "engineId": "webview2", "profileId": "private",
		"identity": map[string]any{"mode": "generated"},
		"region":   map[string]any{"presetId": "en-us-pacific", "language": "en-US", "locale": "en-US", "timezone": "America/Los_Angeles", "acceptLanguages": []string{"en-US", "en"}},
		"viewport": map[string]any{"width": 1280, "height": 720}, "webrtc": "disable-non-proxied-udp", "webgl": "runtime-default", "webgpu": "runtime-default",
		"permissions": map[string]any{"camera": "block", "microphone": "block", "geolocation": "block", "notifications": "block", "clipboardRead": "block"},
		"cookies":     "runtime-default", "exitCleanup": "retain-profile",
	}
	request := map[string]any{
		"schemaVersion": 2, "config": config, "meta": map[string]any{"browser": "webview2", "environmentName": "QA"},
		"extras": map[string]any{}, "engine": map[string]any{"engineId": "webview2", "version": ""},
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseLaunchRequestAcceptsV2EnvelopeAndMetadata(t *testing.T) {
	request, receipt, err := ParseLaunchRequest(validLaunchEnvelope(t))
	if err != nil {
		t.Fatal(err)
	}
	if request.SchemaVersion != 2 || !receipt.Accepted || len(receipt.Unsupported) != 0 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	if len(receipt.Warnings) != 1 {
		t.Fatalf("expected metadata warning, got %#v", receipt.Warnings)
	}
}

func TestParseLaunchRequestRejectsUnknownAndMismatchedFields(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"unknown top-level": func(v map[string]any) { v["danger"] = true },
		"unknown config":    func(v map[string]any) { v["config"].(map[string]any)["binding"] = map[string]any{"mode": "tunnel"} },
		"engine mismatch":   func(v map[string]any) { v["engine"].(map[string]any)["engineId"] = "firefox" },
		"schema mismatch":   func(v map[string]any) { v["schemaVersion"] = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(validLaunchEnvelope(t), &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			data, _ := json.Marshal(value)
			if _, _, err := ParseLaunchRequest(data); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestParseLaunchRequestReturnsExplicitUnsupportedExtras(t *testing.T) {
	var value map[string]any
	if err := json.Unmarshal(validLaunchEnvelope(t), &value); err != nil {
		t.Fatal(err)
	}
	value["extras"] = map[string]any{
		"tabs":        map[string]any{"urls": []string{"https://example.com"}, "startupMode": "urls"},
		"advanced":    map[string]any{"mac": "custom"},
		"fingerprint": map[string]any{"noise": map[string]any{"canvas": true}},
	}
	data, _ := json.Marshal(value)
	_, receipt, err := ParseLaunchRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Accepted || len(receipt.Unsupported) != 3 {
		t.Fatalf("expected explicit unsupported receipt, got %#v", receipt)
	}
	if receipt.Unsupported[0] != "extras.advanced" || receipt.Unsupported[1] != "extras.fingerprint" || receipt.Unsupported[2] != "extras.tabs" {
		t.Fatalf("receipt order is unstable: %#v", receipt.Unsupported)
	}
}
