package maintenance

import "testing"

func TestLeaseRejectsOtherThreadAndClosedReuse(t *testing.T) {
	thread := uint32(1)
	closed := 0
	l := newLease(1, func() uint32 { return thread }, func() error { closed++; return nil })
	if l.Check() != nil {
		t.Fatal("owner rejected")
	}
	thread = 2
	if l.Check() == nil || l.Close() == nil || closed != 0 {
		t.Fatal("borrower released owner's lease")
	}
	thread = 1
	if l.Close() != nil || l.Close() != nil || closed != 1 || l.Check() == nil {
		t.Fatal("close/reuse invariant")
	}
}
