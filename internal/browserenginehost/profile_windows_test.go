//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestRemoveSessionTreeOnlyRemovesGeneratedSession(t *testing.T) {
	base := t.TempDir()
	instances := filepath.Join(base, "BrowserInstances")
	if err := os.Mkdir(instances, 0700); err != nil {
		t.Fatal(err)
	}
	sessionID := strings.Repeat("a", 64)
	session := filepath.Join(instances, sessionID)
	if err := os.MkdirAll(filepath.Join(session, "profile", "Default"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(session, "profile", "Default", "Preferences"), []byte("private session data"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(instances, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeSessionTree(session); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(session); !os.IsNotExist(err) {
		t.Fatalf("session remains or stat failed: %v", err)
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "keep" {
		t.Fatalf("sibling was modified: %v %q", err, data)
	}
}

func TestRemoveSessionTreeRejectsNonGeneratedPath(t *testing.T) {
	if err := removeSessionTree(filepath.Join(t.TempDir(), "BrowserInstances", "not-a-session")); err == nil {
		t.Fatal("non-generated session path was accepted")
	}
}

func TestPrivateProfileDescriptorRequiresExactOwnerAndInheritableACL(t *testing.T) {
	const sid = "S-1-5-21-11-22-33-1001"
	const expected = "O:" + sid + "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + sid + ")"
	want, err := windows.SecurityDescriptorFromString(expected)
	if err != nil || !sameProfileDescriptor(want, want) {
		t.Fatal("private descriptor rejected")
	}
	for _, raw := range []string{
		"O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + sid + ")",
		"O:" + sid + "D:(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + sid + ")",
		"O:" + sid + "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + sid + ")",
		"O:" + sid + "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;WD)",
	} {
		got, err := windows.SecurityDescriptorFromString(raw)
		if err != nil {
			t.Fatal(err)
		}
		if sameProfileDescriptor(got, want) {
			t.Fatal("unsafe private profile descriptor accepted")
		}
	}
}
func TestNativeDirectoryObjectLayoutsAre64Bit(t *testing.T) {
	// Synthetic ABI checks only: never calls NtCreateFile or creates a path.
	if unsafe.Sizeof(uintptr(0)) != 8 || unsafe.Sizeof(unicodeString{}) != 16 || unsafe.Offsetof(unicodeString{}.buffer) != 8 || unsafe.Sizeof(objectAttributes{}) != 48 || unsafe.Offsetof(objectAttributes{}.root) != 8 || unsafe.Offsetof(objectAttributes{}.name) != 16 || unsafe.Offsetof(objectAttributes{}.sd) != 32 || unsafe.Sizeof(ioStatus{}) != 16 {
		t.Fatal("NT directory layout mismatch")
	}
}
