package locationportalgate

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeEngine struct {
	live, working                              snapshot
	active                                     bool
	calls                                      []string
	reads, safeCalls                           int
	readHook                                   func(int, *fakeEngine) (snapshot, error)
	safeHook                                   func(int) error
	installErr, removeErr, beginErr, commitErr error
	commitDespiteError                         bool
}

func (f *fakeEngine) snapshot() (snapshot, error) {
	f.calls = append(f.calls, "read")
	f.reads++
	if f.readHook != nil {
		return f.readHook(f.reads, f)
	}
	if f.active {
		return f.working, nil
	}
	return f.live, nil
}
func (f *fakeEngine) transaction(operation func() error) error {
	f.calls = append(f.calls, "begin")
	if f.beginErr != nil {
		return f.beginErr
	}
	if f.active {
		return ErrUnknown
	}
	f.active = true
	f.working = f.live
	defer func() { f.active = false }()
	if err := operation(); err != nil {
		f.calls = append(f.calls, "abort")
		return err
	}
	f.calls = append(f.calls, "commit")
	if f.commitErr != nil {
		if f.commitDespiteError {
			f.live = f.working
		}
		f.calls = append(f.calls, "abort")
		return f.commitErr
	}
	f.live = f.working
	return nil
}
func (f *fakeEngine) install() error {
	f.calls = append(f.calls, "install")
	if !f.active {
		return ErrUnknown
	}
	if f.installErr != nil {
		return f.installErr
	}
	f.working = snapshot{true, 4, true}
	return nil
}
func (f *fakeEngine) remove() error {
	f.calls = append(f.calls, "remove")
	if !f.active {
		return ErrUnknown
	}
	if f.removeErr != nil {
		return f.removeErr
	}
	f.working = snapshot{false, 0, true}
	return nil
}
func (f *fakeEngine) serviceSafe() error {
	f.calls = append(f.calls, "service-safe")
	f.safeCalls++
	if f.safeHook != nil {
		return f.safeHook(f.safeCalls)
	}
	return nil
}
func (f *fakeEngine) close() { f.calls = append(f.calls, "close") }
func fakeGate(state snapshot) (*Gate, *fakeEngine) {
	f := &fakeEngine{live: state}
	return &Gate{open: func(context.Context) (engine, error) { return f, nil }}, f
}
func countCall(f *fakeEngine, call string) int {
	n := 0
	for _, item := range f.calls {
		if item == call {
			n++
		}
	}
	return n
}

func TestEnsureCommitsOnlyAfterExactReadbackAndFreshSnapshot(t *testing.T) {
	g, f := fakeGate(snapshot{false, 0, true})
	r, err := g.Ensure(context.Background())
	if err != nil || r.Status != "filters_verified" || !r.Verified || !r.LoopbackGateVerified || !r.Installed || r.InstalledFilters != 4 || r.ExpectedFilters != 4 || r.HTTPPort != 58080 || r.HTTPSPort != 58443 {
		t.Fatal(r, err)
	}
	if !reflect.DeepEqual(f.calls, []string{"begin", "read", "install", "read", "commit", "read", "close"}) {
		t.Fatal(f.calls)
	}
	f.calls = nil
	if r, err = g.Ensure(context.Background()); err != nil || !r.Verified || countCall(f, "install") != 0 {
		t.Fatal(r, err, f.calls)
	}
}
func TestAbsenceIsExplicitlyDifferentFromUnknownOrReady(t *testing.T) {
	for _, state := range []snapshot{{false, 0, true}, {true, 0, true}} {
		g, _ := fakeGate(state)
		r, err := g.Inspect(context.Background())
		if err != nil || r.Status != "absent_verified" || !r.Verified || r.Installed || r.LoopbackGateVerified {
			t.Fatal(r, err)
		}
	}
	for _, state := range []snapshot{{false, 0, false}, {true, 3, true}, {false, 4, true}, {true, 5, true}, {true, 4, false}} {
		g, _ := fakeGate(state)
		r, err := g.Inspect(context.Background())
		if err == nil || r.Status != "unknown" || r.Verified || r.LoopbackGateVerified {
			t.Fatal(r, err)
		}
	}
}
func TestPartialForeignAndUnknownNamespacesAreNeverRepairedOrRemoved(t *testing.T) {
	for _, operation := range []string{"ensure", "remove"} {
		for _, state := range []snapshot{{true, 1, true}, {true, 3, true}, {true, 4, false}, {false, 4, true}} {
			g, f := fakeGate(state)
			var err error
			if operation == "ensure" {
				_, err = g.Ensure(context.Background())
			} else {
				_, err = g.Remove(context.Background())
			}
			if err == nil || countCall(f, "install") != 0 || countCall(f, "remove") != 0 || f.live != state {
				t.Fatal(operation, state, err, f.calls)
			}
		}
	}
}
func TestEnsureFailureOrCancellationNeverClaimsReadyOrRemovesProtection(t *testing.T) {
	for _, kind := range []string{"begin", "install", "in-transaction-readback", "commit", "commit-uncertain", "final-readback", "cancel-after-install"} {
		t.Run(kind, func(t *testing.T) {
			g, f := fakeGate(snapshot{false, 0, true})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "begin":
				f.beginErr = ErrUnknown
			case "install":
				f.installErr = ErrUnknown
			case "commit", "commit-uncertain":
				f.commitErr = ErrUnknown
				f.commitDespiteError = kind == "commit-uncertain"
			}
			f.readHook = func(n int, f *fakeEngine) (snapshot, error) {
				if kind == "in-transaction-readback" && n == 2 || kind == "final-readback" && n == 3 {
					return snapshot{}, ErrUnknown
				}
				if kind == "cancel-after-install" && n == 2 {
					cancel()
				}
				if f.active {
					return f.working, nil
				}
				return f.live, nil
			}
			r, err := g.Ensure(ctx)
			if err == nil || r.Verified || r.LoopbackGateVerified || countCall(f, "remove") != 0 {
				t.Fatal(kind, r, err, f.calls)
			}
			if (kind == "commit-uncertain" || kind == "final-readback") && !f.live.complete() {
				t.Fatal("lost possible protection", f.live)
			}
		})
	}
}
func TestRemoveRequiresFreshDisabledStoppedProofBeforeAndAfterMutation(t *testing.T) {
	g, f := fakeGate(snapshot{true, 4, true})
	r, err := g.Remove(context.Background())
	if err != nil || r.Status != "absent_verified" || r.Installed || r.LoopbackGateVerified || !r.Verified || f.live.base || f.live.count != 0 {
		t.Fatal(r, err)
	}
	want := []string{"begin", "service-safe", "read", "remove", "read", "service-safe", "commit", "service-safe", "read", "close"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatal(f.calls)
	}
	f.calls = nil
	if r, err = g.Remove(context.Background()); err != nil || r.Status != "absent_verified" || countCall(f, "remove") != 0 {
		t.Fatal(r, err)
	}
}
func TestRemoveRefusesActiveUnknownOrLateChangedService(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			g, f := fakeGate(snapshot{true, 4, true})
			f.safeHook = func(n int) error {
				if n >= failAt {
					return ErrServiceActive
				}
				return nil
			}
			r, err := g.Remove(context.Background())
			if !errors.Is(err, ErrServiceActive) || r.Verified || r.Status != "unknown" || !f.live.complete() {
				t.Fatal(failAt, r, err, f.live, f.calls)
			}
			if failAt == 1 && countCall(f, "remove") != 0 {
				t.Fatal("removed before proof", f.calls)
			}
			if failAt == 3 && countCall(f, "install") != 1 {
				t.Fatal("late service change not reprotected", f.calls)
			}
		})
	}
}
func TestUncertainRemoveRetainsOrReinstallsOnlyOriginallyVerifiedProtection(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		g, f := fakeGate(snapshot{true, 4, true})
		f.commitErr = ErrUnknown
		f.commitDespiteError = uncertain
		r, err := g.Remove(context.Background())
		if err == nil || r.Verified || !f.live.complete() {
			t.Fatal(r, err, f.calls)
		}
	}
	g, f := fakeGate(snapshot{false, 0, true})
	f.safeHook = func(int) error { return ErrServiceActive }
	if _, err := g.Remove(context.Background()); err == nil || countCall(f, "install") != 0 {
		t.Fatal("took over unowned running portal", err, f.calls)
	}
}
func TestNilCancelledOrUnavailableGateNeverOpensBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	g := &Gate{open: func(context.Context) (engine, error) { calls++; return nil, ErrUnknown }}
	for _, candidate := range []*Gate{nil, {}, g} {
		for _, operation := range []func(*Gate, context.Context) (GateReport, error){(*Gate).Ensure, (*Gate).Inspect, (*Gate).Remove} {
			r, err := operation(candidate, ctx)
			if err == nil || r.Verified || r.Status != "unknown" {
				t.Fatal(r, err)
			}
		}
	}
	if calls != 0 {
		t.Fatal("cancelled request opened native engine")
	}
	g.open = func(context.Context) (engine, error) { return nil, ErrUnavailable }
	r, err := g.Ensure(context.Background())
	if !errors.Is(err, ErrUnavailable) || r.Status == "absent_verified" {
		t.Fatal(r, err)
	}
}
