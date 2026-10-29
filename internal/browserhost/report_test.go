package browserhost

import "testing"

func TestReadReturnsExplicitEngineBoundaries(t *testing.T) {
	r := Read()
	if len(r.Engines) < 2 {
		t.Fatalf("expected at least webview and CEF entries, got %d", len(r.Engines))
	}
	for _, e := range r.Engines {
		if e.ID == "" || e.Name == "" || e.Kind == "" || e.Detail == "" {
			t.Fatalf("engine lacks explicit identity/boundary: %#v", e)
		}
		if !e.Available && e.Version != "" {
			t.Fatalf("unavailable engine has a version: %#v", e)
		}
	}
}
