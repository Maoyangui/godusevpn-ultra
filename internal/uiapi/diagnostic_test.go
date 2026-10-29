package uiapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/ipc"
)

type diagnosticBackend struct{ calls []string }

func (b *diagnosticBackend) Logf(string, ...any) {}
func (b *diagnosticBackend) Dispatch(method string, params json.RawMessage) (any, error) {
	b.calls = append(b.calls, method)
	if method == ipc.MPreviewDiag {
		var selected ipc.DiagnosticOptions
		if json.Unmarshal(params, &selected) != nil || selected != (ipc.DiagnosticOptions{}) {
			panic("web bridge forwarded a sensitive collection option")
		}
		return ipc.DiagnosticPreview{ID: "synthetic-preview", Files: []ipc.DiagnosticFile{{Name: "summary.json", Content: `{}`, Bytes: 2}}}, nil
	}
	return map[string]any{"scope": "minimal_allowlist"}, nil
}

func TestWebAndroidDiagnosticCompatibilityIsExplicitAndNeverCollectsDetails(t *testing.T) {
	b := &diagnosticBackend{}
	s := New(b, Options{Platform: "synthetic-web"})
	for _, name := range []string{"ExportDiag", "ExportDiagPreview"} {
		if _, err := s.Call(name, nil); err == nil || !strings.Contains(err.Error(), "Windows Ultra") {
			t.Fatal("removed ZIP path contract failed silently", name, err)
		}
	}
	for _, field := range []string{"logs", "config", "crashes", "network"} {
		raw := json.RawMessage(`{"` + field + `":true,"acknowledgeSensitive":true}`)
		if _, err := s.Call("PreviewDiag", []json.RawMessage{raw}); err == nil {
			t.Fatal("non-desktop bridge accepted detailed collection")
		}
	}
	if len(b.calls) != 0 {
		t.Fatal("unsupported action reached daemon")
	}
	if _, err := s.Call("PreviewDiag", nil); err != nil {
		t.Fatal("minimal preview unavailable", err)
	}
	if out, err := s.Call("Diagnose", nil); err != nil || !strings.Contains(out.(string), "minimal_allowlist") {
		t.Fatal("minimal troubleshooting summary lost", err)
	}
}
