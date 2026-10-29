package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNativeStoreDirectoryPinDeniesDeleteAndAllowsChildPersistence(t *testing.T) {
	dir := t.TempDir()
	n, err := (nativeStore{}).open(dir)
	if err != nil {
		t.Fatal("cannot open synthetic directory")
	}
	defer n.close()
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h, e := windows.CreateFile(p, windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0); e == nil {
		windows.CloseHandle(h)
		t.Fatal("directory lease did not exclude delete access")
	} else if !errors.Is(e, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("test did not establish the intended sharing constraint")
	}
	file := filepath.Join(dir, "fixture-state.tmp")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal("pin blocks legitimate child writes")
	}
	if err := os.Rename(file, filepath.Join(dir, "fixture-state.json")); err != nil {
		t.Fatal("pin blocks legitimate child replacement")
	}
}
