//go:build windows

package browserengine

import (
	"os"
	"path/filepath"
	"testing"
)

// Only a new test-owned TEMP directory is touched. This exercises actual OS
// byte-range exclusion, not any installed cache, browser, network or service.
func TestStoreFileLockTEMPIndependentHandles(t *testing.T) {
	directory := t.TempDir()
	first, err := lockStoreFile(directory, storeLockName, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockStoreFile(directory, storeLockName, true); err != ErrBusy {
		t.Fatal("exclusive conflict", err)
	}
	if _, err := lockStoreFile(directory, storeLockName, false); err != ErrBusy {
		t.Fatal("shared conflict", err)
	}
	first()
	first()
	shared1, err := lockStoreFile(directory, storeLockName, false)
	if err != nil {
		t.Fatal(err)
	}
	shared2, err := lockStoreFile(directory, storeLockName, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockStoreFile(directory, storeLockName, true); err != ErrBusy {
		t.Fatal(err)
	}
	shared1()
	if _, err := lockStoreFile(directory, storeLockName, true); err != ErrBusy {
		t.Fatal(err)
	}
	shared2()
	last, err := lockStoreFile(directory, storeLockName, true)
	if err != nil {
		t.Fatal(err)
	}
	last()
	if info, err := os.Stat(filepath.Join(directory, storeLockName)); err != nil || info.Size() != 0 {
		t.Fatal(info, err)
	}
}
func TestStoreFileLockTEMPRefusesUnexpectedLeaf(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, storeLockName), []byte("unexpected"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := lockStoreFile(directory, storeLockName, true); err != ErrStore {
		t.Fatal(err)
	}
	if _, err := lockStoreFile(directory, "../outside", true); err != ErrStore {
		t.Fatal(err)
	}
	if _, err := lockStoreFile(directory, "leaf:stream", true); err != ErrStore {
		t.Fatal(err)
	}
}
