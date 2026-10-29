//go:build windows

package privacydata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestReportDestinationStartsWithPrivateProtectedACL(t *testing.T) {
	p := filepath.Join(t.TempDir(), "synthetic-report.json")
	if err := WriteNewLocal(p, []byte(`{"fixture":true}`)); err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(p, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal("cannot inspect synthetic report ACL")
	}
	sddl := sd.String()
	if !strings.Contains(sddl, "D:P") || strings.Contains(sddl, ";;;BU)") || strings.Contains(sddl, ";;;WD)") || strings.Contains(sddl, ";;;AU)") {
		t.Fatal("report has inherited or public access")
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != `{"fixture":true}` {
		t.Fatal("report content mismatch")
	}
}
