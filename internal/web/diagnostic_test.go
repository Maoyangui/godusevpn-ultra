package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/settings"
	"github.com/Maoyangui/godusevpn/internal/uiapi"
)

type diagnosticBackend struct{ sensitiveCalls int }

func (*diagnosticBackend) Logf(string, ...any) {}
func (b *diagnosticBackend) Dispatch(method string, _ json.RawMessage) (any, error) {
	if method == ipc.MGetSettings {
		return settings.Default(), nil
	}
	b.sensitiveCalls++
	return nil, nil
}

func TestLegacyDiagnosticDownloadIsGoneWithoutDispatchOrPathRead(t *testing.T) {
	b := &diagnosticBackend{}
	s := New(uiapi.New(b, uiapi.Options{}), nil)
	for _, path := range []string{"/api/diag/godusevpn-diag-legacy.zip", "/api/diag/unknown.zip"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "Windows Ultra") || w.Header().Get("Content-Disposition") != "" {
			t.Fatal("legacy unreviewed download was not explicitly removed", w.Code)
		}
	}
	if b.sensitiveCalls != 0 {
		t.Fatal("legacy download collected diagnostics")
	}
}
