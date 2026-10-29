package state

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// OnChange runs inline, exactly between publishing Connected and entering
// watch. This forces the old lost-kick ordering without scheduler sleeps.
func TestCheckNowFromConnectedNotificationIsNotDiscarded(t *testing.T) {
	checked := make(chan struct{}, 1)
	var once sync.Once
	var m *Machine
	m = New(Deps{
		Prepare:    func(context.Context) ([]byte, error) { return []byte("fixture"), nil },
		Start:      func([]byte) error { return nil },
		Stop:       func() error { return nil },
		Alive:      func() bool { return true },
		Health:     func(context.Context) error { checked <- struct{}{}; return nil },
		AliveEvery: time.Hour, HealthEvery: time.Hour,
		OnChange: func(s Snapshot) {
			if s.Status == Connected {
				once.Do(m.CheckNow)
			}
		},
	})
	m.Connect()
	defer m.Disconnect()
	select {
	case <-checked:
	case <-time.After(time.Second):
		t.Fatal("CheckNow issued by the Connected notification was discarded before watch")
	}
}

func TestPreviousSessionKickDoesNotReplaceFreshConnectedRequest(t *testing.T) {
	firstHealth := make(chan struct{})
	secondHealth := make(chan struct{}, 1)
	var healths, connections atomic.Int32
	var m *Machine
	m = New(Deps{
		Prepare: func(context.Context) ([]byte, error) { return []byte("fixture"), nil },
		Start:   func([]byte) error { return nil }, Stop: func() error { return nil },
		Alive: func() bool { return true },
		Health: func(ctx context.Context) error {
			if healths.Add(1) == 1 {
				close(firstHealth)
				<-ctx.Done()
				// Return directly from watch rather than selecting another kick,
				// leaving a definitely queued old-session token across Disconnect.
				return Errf(CodeCoreCrash, "fixture cancelled core")
			}
			secondHealth <- struct{}{}
			return nil
		},
		AliveEvery: time.Hour, HealthEvery: time.Hour,
		OnChange: func(s Snapshot) {
			if s.Status == Connected && connections.Add(1) == 2 {
				m.CheckNow()
			}
		},
	})
	m.Connect()
	defer m.Disconnect()
	waitUntil(t, func() bool { return m.Snapshot().Status == Connected }, "fixture did not connect")
	m.CheckNow()
	select {
	case <-firstHealth:
	case <-time.After(time.Second):
		t.Fatal("fixture health did not start")
	}
	m.CheckNow()
	m.Disconnect()
	if len(m.kick) != 1 {
		t.Fatal("fixture did not retain an old-session request")
	}
	m.Connect()
	select {
	case <-secondHealth:
	case <-time.After(time.Second):
		t.Fatal("old-session token replaced or discarded the fresh Connected request")
	}
	if healths.Load() != 2 {
		t.Fatal("stale request triggered an extra health check")
	}
}

func TestCheckNowRejectsPreConnectedRequestsBeforeEnqueue(t *testing.T) {
	observed := make(chan bool, 2)
	connected := make(chan struct{})
	var m *Machine
	m = New(Deps{
		Prepare: func(context.Context) ([]byte, error) { return []byte("fixture"), nil },
		Start:   func([]byte) error { return nil }, Stop: func() error { return nil },
		AliveEvery: time.Hour, HealthEvery: time.Hour,
		OnChange: func(s Snapshot) {
			if s.Status == Preparing || s.Status == Starting {
				m.CheckNow()
				observed <- len(m.kick) == 0
			}
			if s.Status == Connected {
				close(connected)
			}
		},
	})
	m.Connect()
	defer m.Disconnect()
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("fixture did not connect")
	}
	for i := 0; i < 2; i++ {
		if !<-observed {
			t.Fatal("pre-Connected request was enqueued")
		}
	}
}
