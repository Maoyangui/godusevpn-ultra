package trial

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDeviceIDPersistsAndDoesNotUseMachineIdentity(t *testing.T) {
	dir := t.TempDir()
	c := New("https://example.com/trial", dir)
	a, err := c.DeviceID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.DeviceID()
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a != b {
		t.Fatalf("device id not persistent: %q %q", a, b)
	}
	if platformDeviceID() == "" {
		st, err := os.Stat(filepath.Join(dir, "trial-device.json"))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
			t.Fatalf("device file too open: %o", st.Mode().Perm())
		}
	}
}

func TestStatusAndClaim(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/trial/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"available","enabled":true,"plan":"free","purchaseUrl":"https://buy.example/"}`))
	})
	mux.HandleFunc("/trial/claim", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-GodUseVPN-Device") == "" {
			t.Error("missing device header")
		}
		_, _ = w.Write([]byte(`{"ok":true,"status":"claimed","subscriptionUrl":"https://sub.example/x","purchaseUrl":"https://buy.example/","expiresAt":1735689600}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New(srv.URL+"/trial", t.TempDir())
	s, err := c.Status(context.Background())
	if err != nil || !s.Enabled || s.Status != "available" || s.Plan != "free" {
		t.Fatalf("status: %#v %v", s, err)
	}
	got, err := c.Claim(context.Background())
	if err != nil || !got.OK || got.SubscriptionURL == "" || got.ExpiresAt != 1735689600 {
		t.Fatalf("claim: %#v %v", got, err)
	}
}
