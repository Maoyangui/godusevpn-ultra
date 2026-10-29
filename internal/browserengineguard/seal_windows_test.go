//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"golang.org/x/sys/windows"
	"testing"
)

// These descriptors are in-memory fixtures. No file ACL/owner is changed and
// no filesystem, Job, WFP or browser operation is invoked.
func TestCodeSealRequiresAdminOwnerAndExactProtectedReadOnlyACL(t *testing.T) {
	const sid = "S-1-5-21-11-22-33-1001"
	sd, err := codeDescriptor(sid)
	if err != nil || verifyCodeDescriptor(sd, sid) != nil {
		t.Fatal("generated sealed descriptor rejected")
	}
	for _, raw := range []string{
		"O:" + sid + "D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;" + sid + ")",
		"O:BAD:(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;" + sid + ")",
		"O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + sid + ")",
		"O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;WD)",
		"O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;OICI;0x1200a9;;;" + sid + ")",
		"O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;0x1200a9;;;" + sid + ")(A;;FA;;;WD)",
		"O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(D;;0x1200a9;;;" + sid + ")",
	} {
		descriptor, err := windows.SecurityDescriptorFromString(raw)
		if err != nil {
			t.Fatal("invalid test fixture", err)
		}
		if verifyCodeDescriptor(descriptor, sid) == nil {
			t.Fatal("unsafe seal accepted", raw)
		}
	}
	for _, invalid := range []string{"", sid + ")(A;;FA;;;WD)", "S-1-5-18", "S-1-5-32-544"} {
		if _, err := codeDescriptor(invalid); err == nil {
			t.Fatal("unsafe originating SID accepted")
		}
	}
}
