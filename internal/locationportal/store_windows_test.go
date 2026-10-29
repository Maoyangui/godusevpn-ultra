//go:build windows

package locationportal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every test in this file uses synthetic data only: never call nativeBackend
// methods, NewCallback, openNativeStore, or a Windows setting/ACL API.
const fakeSID = "S-1-5-21-1001"

func TestOrdinaryIdentityRejectsElevatedServiceAndImpersonation(t *testing.T) {
	for _, integrity := range []string{"S-1-16-8192", "S-1-16-8448"} {
		if !ordinaryIdentity(fakeSID, integrity, false, false) {
			t.Fatal(integrity)
		}
	}
	for _, test := range []struct {
		sid, integrity          string
		elevated, impersonating bool
	}{
		{"", "S-1-16-8192", false, false}, {"S-1-5-18", "S-1-16-8192", false, false}, {"S-1-5-19", "S-1-16-8192", false, false}, {"S-1-5-20", "S-1-16-8192", false, false},
		{fakeSID, "S-1-16-4096", false, false}, {fakeSID, "S-1-16-12288", false, false}, {fakeSID, "S-1-16-8192", true, false}, {fakeSID, "S-1-16-8192", false, true},
	} {
		if ordinaryIdentity(test.sid, test.integrity, test.elevated, test.impersonating) {
			t.Fatal("unsupported execution identity accepted", test)
		}
	}
}
func TestPrivateACLRequiresOnlyUserSystemAdministrators(t *testing.T) {
	for _, directory := range []bool{false, true} {
		flags := uint8(0)
		if directory {
			flags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
		}
		entries := []privateACE{{fakeSID, flags, windows.ACCESS_ALLOWED_ACE_TYPE, portalFileAllAccess}, {"S-1-5-18", flags, windows.ACCESS_ALLOWED_ACE_TYPE, portalFileAllAccess}, {"S-1-5-32-544", flags, windows.ACCESS_ALLOWED_ACE_TYPE, portalFileAllAccess}}
		if !portalPrivateACL(fakeSID, fakeSID, true, directory, entries) {
			t.Fatal("exact private ACL refused")
		}
		for _, kind := range []string{"foreign-owner", "unprotected", "missing", "duplicate", "foreign-ace", "inherited", "deny", "partial-rights"} {
			t.Run(kind+map[bool]string{true: "-directory", false: "-file"}[directory], func(t *testing.T) {
				copy := append([]privateACE(nil), entries...)
				owner, protected := fakeSID, true
				switch kind {
				case "foreign-owner":
					owner = "S-1-1-0"
				case "unprotected":
					protected = false
				case "missing":
					copy = copy[:2]
				case "duplicate":
					copy[0] = copy[1]
				case "foreign-ace":
					copy[0].sid = "S-1-1-0"
				case "inherited":
					copy[0].flags |= windows.INHERITED_ACE
				case "deny":
					copy[0].kind = windows.ACCESS_DENIED_ACE_TYPE
				case "partial-rights":
					copy[0].mask = windows.FILE_GENERIC_READ
				}
				if portalPrivateACL(owner, fakeSID, protected, directory, copy) {
					t.Fatal("unsafe ACL accepted")
				}
			})
		}
	}
}
func TestRelativeOpenPoliciesDoNotFollowOrShareDelete(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, write := range []bool{false, true} {
			p := portalPolicy(directory, write, windows.FILE_CREATE)
			if p.share&windows.FILE_SHARE_DELETE != 0 || p.attributes&windows.OBJ_DONT_REPARSE == 0 || p.options&windows.FILE_OPEN_REPARSE_POINT == 0 || p.options&windows.FILE_SYNCHRONOUS_IO_NONALERT == 0 {
				t.Fatal(p)
			}
			if directory && (p.share&windows.FILE_SHARE_WRITE == 0 || p.options&windows.FILE_DIRECTORY_FILE == 0) {
				t.Fatal("relative atomic rename directory blocked", p)
			}
			if !directory && p.share&windows.FILE_SHARE_WRITE != 0 {
				t.Fatal("journal write sharing", p)
			}
			if !directory && write && (p.access&windows.DELETE == 0 || p.options&windows.FILE_WRITE_THROUGH == 0) {
				t.Fatal("unheld replacement file", p)
			}
		}
	}
	p := portalLockPolicy(windows.FILE_OPEN_IF)
	if p.access&windows.DELETE != 0 || p.share != windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE || p.options&windows.FILE_WRITE_THROUGH != 0 {
		t.Fatal("permanent lock incompatible with other app instance", p)
	}
	for _, name := range []string{"", ".", "..", "../portal", "a/b", "a\\b", "a:b", "a\x00b", "a\n", "a.", "a "} {
		if f, err := portalOpen(nil, name, fakeSID, false, false, windows.FILE_OPEN); err == nil || f != nil {
			t.Fatal("invalid open reached native call")
		}
	}
}
func TestRelativeRenameBufferHasFixedLeafAndCorrectABI(t *testing.T) {
	for _, replace := range []bool{false, true} {
		data, err := portalRenameBuffer(0x1234, replace)
		if err != nil {
			t.Fatal(err)
		}
		var header portalRename
		offset := int(unsafe.Offsetof(header.Name))
		if unsafe.Sizeof(uintptr(0)) == 8 && offset != 20 {
			t.Fatal("unexpected x64 FILE_RENAME_INFO ABI", offset)
		}
		value := (*portalRename)(unsafe.Pointer(&data[0]))
		expectedFlags := uint32(0)
		if replace {
			expectedFlags = 1
		}
		if value.Root != 0x1234 || value.Flags != expectedFlags || int(value.Length) != 2*len(journalName) || len(data) != offset+2*(len(journalName)+1) {
			t.Fatal(value, len(data), offset)
		}
		units := make([]uint16, (len(data)-offset)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(data[offset+i*2:])
		}
		if windows.UTF16ToString(units) != journalName || units[len(units)-1] != 0 {
			t.Fatal("non-fixed rename target", units)
		}
	}
	for _, handle := range []windows.Handle{0, windows.InvalidHandle} {
		if _, err := portalRenameBuffer(handle, false); err == nil {
			t.Fatal("invalid root handle accepted")
		}
	}
}

type fakeJournalWriter struct {
	data              bytes.Buffer
	short             bool
	writeErr, syncErr error
	syncCalls         int
}

func (w *fakeJournalWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	if w.short {
		return len(data) - 1, nil
	}
	return w.data.Write(data)
}
func (w *fakeJournalWriter) Sync() error { w.syncCalls++; return w.syncErr }
func TestDurableWriteRejectsShortWriteAndFlushFailure(t *testing.T) {
	for _, kind := range []string{"ok", "short", "write-error", "sync-error"} {
		t.Run(kind, func(t *testing.T) {
			w := &fakeJournalWriter{}
			switch kind {
			case "short":
				w.short = true
			case "write-error":
				w.writeErr = errors.New("fake")
			case "sync-error":
				w.syncErr = errors.New("fake")
			}
			err := writeJournalContent(w, []byte("known pending"))
			if (err == nil) != (kind == "ok") {
				t.Fatal(err)
			}
			if kind == "short" || kind == "write-error" {
				if w.syncCalls != 0 {
					t.Fatal("flushed incomplete content")
				}
			} else if w.syncCalls != 1 {
				t.Fatal("durability not attempted")
			}
		})
	}
	if !errors.Is(portalFileError(windows.STATUS_OBJECT_NAME_NOT_FOUND), os.ErrNotExist) || !errors.Is(portalFileError(windows.STATUS_SHARING_VIOLATION), ErrBusy) || !errors.Is(portalFileError(nil), ErrStore) {
		t.Fatal("native errors misclassified")
	}
}
