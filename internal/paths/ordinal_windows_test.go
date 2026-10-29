package paths

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Pure ordinal comparison: no file, account, ACL or service is accessed.
func ordinalEqualForTest(t *testing.T, a, b string) bool {
	t.Helper()
	ap, err := windows.UTF16PtrFromString(a)
	if err != nil {
		t.Fatal(err)
	}
	bp, err := windows.UTF16PtrFromString(b)
	if err != nil {
		t.Fatal(err)
	}
	ret, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("CompareStringOrdinal").Call(uintptr(unsafe.Pointer(ap)), ^uintptr(0), uintptr(unsafe.Pointer(bp)), ^uintptr(0), 1)
	if ret == 0 {
		t.Fatal(err)
	}
	return ret == 2 // CSTR_EQUAL
}

func TestWindowsOrdinalCandidatesRemainDistinct(t *testing.T) {
	for _, tc := range []struct{ alias, fixed string }{{"settıngs.json", "settings.json"}, {"ſtate.json", "state.json"}, {"profıles", "profiles"}} {
		if ordinalEqualForTest(t, tc.alias, tc.fixed) {
			t.Fatalf("Windows ordinal behavior changed for %q; recheck filesystem behavior separately", tc.alias)
		}
	}
}
