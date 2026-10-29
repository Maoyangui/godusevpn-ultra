package locationportal

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	data   []byte
	saves  int
	fail   bool
	events *[]string
	onSave func()
}

func (s *memoryStore) read() ([]byte, error) {
	if s.data == nil {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), s.data...), nil
}
func (s *memoryStore) save(b []byte) error {
	if s.fail {
		return ErrStore
	}
	s.saves++
	s.data = append([]byte(nil), b...)
	if s.onSave != nil {
		s.onSave()
	}
	if s.events != nil {
		*s.events = append(*s.events, "backup")
	}
	return nil
}
func (*memoryStore) close() {}

type fakePortal struct {
	on                                    bool
	position                              Position
	failPosition, failEnable, failDisable bool
	calls                                 int
	events                                *[]string
	injectedErr                           error
}

func (p *fakePortal) Check(context.Context) (bool, error)        { return p.on, nil }
func (p *fakePortal) Override(context.Context) (bool, error)     { return p.on, nil }
func (p *fakePortal) Injected(context.Context) (Position, error) { return p.position, p.injectedErr }
func (p *fakePortal) setPosition(_ context.Context, v Position) error {
	p.calls++
	if p.events != nil {
		*p.events = append(*p.events, "set-position")
	}
	if p.failPosition {
		return ErrReadback
	}
	if !p.on {
		return ErrReadback
	} // official WDP precondition, independent of manager ordering
	p.position = v
	return nil
}
func (p *fakePortal) setOverride(_ context.Context, on bool) error {
	p.calls++
	if p.events != nil {
		*p.events = append(*p.events, "set-override")
	}
	if on && p.failEnable || !on && p.failDisable {
		return ErrReadback
	}
	p.on = on
	return nil
}
func (*fakePortal) Close() error { return nil }
func fixtureManager() (*Manager, *fakePortal, *memoryStore) {
	p := &fakePortal{}
	s := &memoryStore{}
	m := New(nil)
	m.client = p
	m.config = fixtureConfig()
	m.identity = func() (userIdentity, error) { return userIdentity{"S-1-5-21-1001", "fake"}, nil }
	m.open = func(userIdentity, storeMode) (store, error) { return s, nil }
	m.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	return m, p, s
}
func TestOriginalCacheUnknownOrChangedNeverTreatedAsEmpty(t *testing.T) {
	m, p, s := fixtureManager()
	p.injectedErr = ErrReadback
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}}); e == nil || p.calls != 0 || s.saves != 0 {
		t.Fatal("unknown original cache written")
	}
	m, p, s = fixtureManager()
	s.onSave = func() {
		if s.saves == 1 {
			p.position = Position{8, 9}
		}
	}
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}}); e != ErrConflict || p.calls != 0 {
		t.Fatal("post-backup external change overwritten")
	}
}
func TestPartialRestoreOfOriginalCacheRemainsRecoverable(t *testing.T) {
	m, p, _ := fixtureManager()
	before := Position{3, 4}
	p.position = before
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}}); e != nil {
		t.Fatal(e)
	}
	p.failDisable = true
	if _, e := m.Restore(context.Background()); e == nil || p.position != before || !p.on {
		t.Fatal("restore failure fabricated success")
	}
	if r, e := m.Read(context.Background()); e != nil || !r.CanRestore || r.ConfigurationVerified {
		t.Fatal("partial restoration became unreachable", e)
	}
	p.failDisable = false
	if _, e := m.Restore(context.Background()); e != nil || p.on || p.position != before {
		t.Fatal(e)
	}
}
func TestTrajectoryStopLeavesOnlyFirstConfirmedPoint(t *testing.T) {
	m, p, _ := fixtureManager()
	r, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}, {3, 4}}, IntervalSeconds: 2})
	if e != nil || !r.RunningTrajectory || r.CompletedPoints != 1 {
		t.Fatal(e)
	}
	done := m.done
	m.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled synthetic trajectory did not finish")
	}
	if p.position != (Position{1, 2}) || m.completed != 1 {
		t.Fatal("stopped trajectory advanced")
	}
	if r, e = m.Read(context.Background()); e != nil || r.RunningTrajectory || !r.CanRestore {
		t.Fatal("last point lost recovery")
	}
}
func TestTransactionsBackupFirstAndIndependentConsumer(t *testing.T) {
	m, p, s := fixtureManager()
	before := Position{-1, -2}
	p.position = before
	var events []string
	s.events = &events
	p.events = &events
	target := Position{12, 23}
	r, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{target}})
	if e != nil || !r.ConfigurationVerified || r.ConsumerVerified || !p.on {
		t.Fatal(r, e)
	}
	if strings.Join(events, ",") != "backup,set-override,set-position,backup" {
		t.Fatal(events)
	}
	m.consume = func(_ context.Context, request ObserveRequest) (Observation, error) {
		if request.RequestedAfter != m.now().Format(time.RFC3339Nano) {
			t.Fatal("stale consumer request")
		}
		return Observation{target, m.now().Format(time.RFC3339Nano), 10, "default"}, nil
	}
	r, e = m.Observe(context.Background())
	if e != nil || !r.ConsumerVerified {
		t.Fatal(r, e)
	}
	r, e = m.Restore(context.Background())
	if e != nil || r.Status != "restored" || p.on || p.position != before {
		t.Fatal(r, e)
	}
}
func TestOldTrajectoryCompletionCannotCancelReplacement(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			m, p, _ := fixtureManager()
			tick := make(chan struct{})
			paused := make(chan struct{})
			release := make(chan struct{})
			m.waitPoint = func(ctx context.Context, _ time.Duration) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-tick:
					return nil
				}
			}
			m.beforeTrajectoryFinish = func(owner uint64) {
				if owner == 1 {
					close(paused)
					<-release
				}
			}
			if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}, {3, 4}}, IntervalSeconds: 2}); e != nil {
				t.Fatal(e)
			}
			oldDone := m.done
			if failed {
				p.failPosition = true
			}
			close(tick)
			select {
			case <-paused:
			case <-time.After(time.Second):
				t.Fatal("controlled old completion not reached")
			}
			m.Stop()
			p.failPosition = false
			// A failed old point leaves durable recovery intent. Restore that intent
			// before starting a replacement; success can apply a fresh point directly.
			if failed {
				if _, e := m.Restore(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			replacementTick := make(chan struct{})
			m.waitPoint = func(ctx context.Context, _ time.Duration) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-replacementTick:
					return nil
				}
			}
			if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{5, 6}, {7, 8}}, IntervalSeconds: 2}); e != nil {
				t.Fatal(e)
			}
			newDone := m.done
			newID := m.trajectoryID
			close(release)
			select {
			case <-oldDone:
			case <-time.After(time.Second):
				t.Fatal("old worker did not finish")
			}
			m.mu.Lock()
			running := m.cancel != nil && m.done == newDone && m.trajectoryID == newID && m.trajectoryError == nil
			m.mu.Unlock()
			if !running {
				t.Fatal("old completion modified the replacement")
			}
			m.Stop()
			select {
			case <-newDone:
			case <-time.After(time.Second):
				t.Fatal("replacement no longer cancellable")
			}
		})
	}
}
func TestConsumerCanBeCancelledWithoutWaitingForTransactionLock(t *testing.T) {
	m, _, _ := fixtureManager()
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}}); e != nil {
		t.Fatal(e)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	m.consume = func(ctx context.Context, _ ObserveRequest) (Observation, error) {
		close(started)
		<-ctx.Done()
		return Observation{}, ctx.Err()
	}
	go func() { _, e := m.Observe(context.Background()); done <- e }()
	<-started
	m.CancelObservation()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("cancelled consumer fabricated result")
		}
	case <-time.After(time.Second):
		t.Fatal("consumer cancellation waited on transaction lock")
	}
	if r, e := m.Read(context.Background()); e != nil || !r.ConfigurationVerified || !r.CanRestore {
		t.Fatal("cancelling read changed applied configuration")
	}
}
func TestForeignOverrideAndExternalChangeNeverOverwritten(t *testing.T) {
	m, p, _ := fixtureManager()
	p.on = true
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}}); e != ErrConflict || p.calls != 0 {
		t.Fatal("foreign override written", e)
	}
	m, p, _ = fixtureManager()
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}}); e != nil {
		t.Fatal(e)
	}
	p.position = Position{3, 4}
	before := p.calls
	if _, e := m.Restore(context.Background()); e != ErrConflict || p.calls != before {
		t.Fatal("external position overwritten", e)
	}
}
func TestFailurePreservesRecoverableIntent(t *testing.T) {
	for _, phase := range []string{"backup", "position", "enable", "disable"} {
		t.Run(phase, func(t *testing.T) {
			m, p, s := fixtureManager()
			if phase == "backup" {
				s.fail = true
			}
			if phase == "position" {
				p.failPosition = true
			}
			if phase == "enable" {
				p.failEnable = true
			}
			r, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{1, 2}}})
			if phase == "disable" {
				if e != nil {
					t.Fatal(e)
				}
				p.failDisable = true
				_, e = m.Restore(context.Background())
			}
			if e == nil {
				t.Fatal("false success", r)
			}
			if phase == "backup" {
				if p.calls != 0 {
					t.Fatal("write before durable intent")
				}
				return
			}
			j, e := load(s, "S-1-5-21-1001")
			if e != nil || j == nil || j.Stage == "restored" {
				t.Fatal("lost intent")
			}
			p.failPosition = false
			p.failEnable = false
			p.failDisable = false
			if _, e = m.Restore(context.Background()); e != nil || p.on {
				t.Fatal("failed recovery", e)
			}
		})
	}
}
func TestIndependentConsumerRejectsCachedWrongOrChangedEvidence(t *testing.T) {
	for _, kind := range []string{"stale", "future", "wrong-source", "wrong-coordinate", "bad-accuracy", "config-changed", "consumer-error"} {
		t.Run(kind, func(t *testing.T) {
			m, p, _ := fixtureManager()
			target := Position{1, 2}
			if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{target}}); e != nil {
				t.Fatal(e)
			}
			m.consume = func(context.Context, ObserveRequest) (Observation, error) {
				o := Observation{target, m.now().Format(time.RFC3339Nano), 10, "default"}
				switch kind {
				case "stale":
					o.Timestamp = m.now().Add(-time.Second).Format(time.RFC3339Nano)
				case "future":
					o.Timestamp = m.now().Add(time.Minute).Format(time.RFC3339Nano)
				case "wrong-source":
					o.PositionSource = "ip-address"
				case "wrong-coordinate":
					o.Position.Latitude += 1
				case "bad-accuracy":
					o.AccuracyMetres = 0
				case "config-changed":
					p.on = false
				case "consumer-error":
					return o, errors.New("private error")
				}
				return o, nil
			}
			if r, e := m.Observe(context.Background()); e == nil || r.ConsumerVerified {
				t.Fatal("unverified consumer accepted")
			}
		})
	}
}
func TestJournalStrictHashAndNoCredentials(t *testing.T) {
	m, _, s := fixtureManager()
	if _, e := m.Apply(context.Background(), ApplyRequest{Points: []Position{{0, 0}}}); e != nil {
		t.Fatal(e)
	}
	for _, canary := range []string{m.config.Password, m.config.Username, "csrf"} {
		if strings.Contains(string(s.data), canary) {
			t.Fatal("secret persisted")
		}
	}
	s.data = append(s.data, []byte(` {}`)...)
	if _, e := load(s, "S-1-5-21-1001"); e == nil {
		t.Fatal("trailing journal accepted")
	}
}
