package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/paths"
)

func TestStoreTrustFailureStopsBeforeAllInitialization(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uncreated-store")
	t.Setenv(paths.DataEnvironment, root)
	t.Setenv("GODUSEVPN_CONF", root)
	old := prepareServiceStore
	t.Cleanup(func() { prepareServiceStore = old })
	prepareServiceStore = func() (func(), error) { return nil, errors.New("fake untrusted owner") }
	// NoListen must not bypass the check. Returning at this first dependency
	// also precedes the only startup UnprotectChecked call (see NewWithOptions).
	d, err := NewWithOptions(Options{NoListen: true})
	if d != nil || err == nil || !strings.Contains(err.Error(), "现有网络保护保持不变") {
		t.Fatalf("unexpected result: %v %v", d, err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed trust check wrote a store")
	}
	CaptureCrashes(false)
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed trust check opened crash log")
	}
}
