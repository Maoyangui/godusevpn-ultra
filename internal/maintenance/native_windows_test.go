package maintenance

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestGateDescriptorRejectsForeignOwnershipAndExcessAccess(t *testing.T) {
	for _, tc := range []struct {
		s  string
		ok bool
	}{{gateSDDL, true}, {"O:BUG:BUD:P(A;;GA;;;SY)(A;;GA;;;BA)", false}, {"O:BAG:BAD:P(A;;GA;;;BA)(A;;GA;;;IU)", false}, {"O:BAG:BAD:P(A;;GA;;;BA)(A;;0x00120001;;;WD)", false}} {
		sd, err := windows.SecurityDescriptorFromString(tc.s)
		if err != nil {
			t.Fatal(err)
		}
		if (checkDescriptor(sd) == nil) != tc.ok {
			t.Fatal("wrong descriptor decision")
		}
	}
}
