//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"golang.org/x/sys/windows"
	"testing"
	"unsafe"
)

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
