package browserenginehost

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type startupOwnerFixture struct {
	status     Status
	afterClose Status
	done       chan struct{}
	finish     bool
	closeErr   error
	closeCalls int
	closed     bool
}

func (s *startupOwnerFixture) Close() error {
	s.closeCalls++
	s.status = s.afterClose
	s.closed = true
	if s.finish {
		close(s.done)
	}
	return s.closeErr
}
func (s *startupOwnerFixture) Status() Status {
	if !s.closed {
		panic("startup ownership was judged before closing")
	}
	return s.status
}
func (s *startupOwnerFixture) Done() <-chan struct{} { return s.done }

func TestFailedStartupKeepsOwnerUnlessCloseIsFullyConfirmed(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   Status
		finish   bool
		nilDone  bool
		closeErr error
		retain   bool
	}{
		{name: "confirmed", status: Status{State: "closed"}, finish: true},
		{name: "unknown filters after zero Job", status: Status{State: "closed_filters_unknown"}, finish: true, retain: true},
		{name: "unknown Job", status: Status{State: "closing_or_unknown"}, retain: true},
		{name: "close still running", status: Status{State: "closing"}, retain: true},
		{name: "unknown state", finish: true, retain: true},
		{name: "closed without Done", status: Status{State: "closed"}, retain: true},
		{name: "closed with nil Done", status: Status{State: "closed"}, nilDone: true, retain: true},
		{name: "error despite closed state", status: Status{State: "closed"}, finish: true, closeErr: errors.New("close unresolved"), retain: true},
		{name: "contradictory readiness", status: Status{State: "closed", Ready: true}, finish: true, retain: true},
		{name: "contradictory navigation", status: Status{State: "closed", CanNavigate: true}, finish: true, retain: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := &startupOwnerFixture{status: Status{State: "closed"}, afterClose: test.status, done: make(chan struct{}), finish: test.finish, closeErr: test.closeErr}
			if test.nilDone {
				owner.done = nil
			}
			if got := retainFailedStartup(owner); got != test.retain {
				t.Fatalf("owner retention = %v, expected %v", got, test.retain)
			}
			if owner.closeCalls != 1 {
				t.Fatalf("close attempts = %d", owner.closeCalls)
			}
		})
	}
	if retainFailedStartup(nil) {
		t.Fatal("no owner was present")
	}
}

func TestFailedStartupRetainsActualLifetimeWhenFilterRemovalIsUnknown(t *testing.T) {
	// This is the real lifetime/session implementation with pure fake resources,
	// not a native Job, listener, helper, WFP operation or browser process.
	var calls []string
	life := fixtureLifetime(&calls, nil, ErrRefused)
	life.session.close = life.close
	if !retainFailedStartup(life.session) {
		t.Fatal("failed startup discarded the owner of unknown persistent filters")
	}
	if life.session.Status().State != "closed_filters_unknown" {
		t.Fatal("filter uncertainty was hidden")
	}
	select {
	case <-life.session.Done():
	default:
		t.Fatal("fake process tree had not finished closing")
	}
	if strings.Count(strings.Join(calls, ","), "remove-and-verify-absent") != 1 {
		t.Fatal("filter mutation was retried", calls)
	}
}

func TestFailedStartupReleasesActualLifetimeOnlyAfterConfirmedCleanup(t *testing.T) {
	var calls []string
	life := fixtureLifetime(&calls, nil, nil)
	life.session.close = life.close
	if retainFailedStartup(life.session) {
		t.Fatal("fully closed fake resources were not released")
	}
	if life.session.Status().State != "closed" || len(calls) != 9 {
		t.Fatal("cleanup did not finish before owner release", calls)
	}
}

func TestStartupCheckpointRefusesCancellationBeforeNextSideEffect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	if startupCheckpoint(ctx) != nil {
		t.Fatal("live startup context refused")
	}
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, stopped := range []context.Context{nil, ctx, expired} {
		if !errors.Is(startupCheckpoint(stopped), ErrRefused) {
			t.Fatal("nil/cancelled/expired startup was allowed")
		}
	}
}
