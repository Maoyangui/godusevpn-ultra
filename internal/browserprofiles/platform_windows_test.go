//go:build windows

package browserprofiles

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// Pure value checks: none of these tests call a Windows filesystem/token API.
func TestNativeOpenPolicyNeverOverwriteOrShareWriteDelete(t *testing.T) {
	for _, pair := range [][2]bool{{false, false}, {true, false}, {true, true}} {
		p, e := filePolicy(pair[0], pair[1])
		if e != nil {
			t.Fatal(e)
		}
		if p.share != windows.FILE_SHARE_READ || p.attributes&windows.OBJ_DONT_REPARSE == 0 || p.options&windows.FILE_NON_DIRECTORY_FILE == 0 || p.options&windows.FILE_OPEN_REPARSE_POINT == 0 || p.options&windows.FILE_SYNCHRONOUS_IO_NONALERT == 0 {
			t.Fatalf("unsafe open policy: %+v", p)
		}
		if pair[1] && p.disposition != windows.FILE_CREATE || !pair[1] && p.disposition != windows.FILE_OPEN {
			t.Fatal("overwrite/reuse disposition")
		}
		if pair[0] && p.access&windows.DELETE == 0 || !pair[0] && p.access&(windows.FILE_WRITE_DATA|windows.DELETE) != 0 {
			t.Fatal("incorrect access")
		}
	}
	if _, e := filePolicy(false, true); !errors.Is(e, ErrInput) {
		t.Fatal("read-only create accepted")
	}
	for _, e := range []error{windows.ERROR_FILE_NOT_FOUND, windows.STATUS_OBJECT_PATH_NOT_FOUND, windows.STATUS_OBJECT_NAME_NOT_FOUND} {
		if !errors.Is(nativeError(e), os.ErrNotExist) {
			t.Fatal(e)
		}
	}
	if nativeError(windows.STATUS_SHARING_VIOLATION) != ErrBusy || nativeError(windows.STATUS_OBJECT_NAME_COLLISION) != ErrConflict || nativeError(windows.ERROR_ACCESS_DENIED) != ErrStore {
		t.Fatal("raw Windows error escaped")
	}
}

func TestDeleteRefusesFilesNotOpenedByThisProductDirectoryBeforeAnyAPICall(t *testing.T) {
	dir := &nativeDirectory{}
	if err := dir.remove(&memoryFile{}); !errors.Is(err, ErrStore) {
		t.Fatal("foreign implementation accepted")
	}
	// No real os.File is supplied: the parent-identity check must fail first.
	if err := dir.remove(&nativeFile{parent: &nativeDirectory{}}); !errors.Is(err, ErrStore) {
		t.Fatal("another directory's handle accepted")
	}
}
