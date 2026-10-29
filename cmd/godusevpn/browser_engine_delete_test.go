package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

func TestEngineControllerDeleteRefusesEveryRetainedOwner(t *testing.T) {
	for _, state := range []string{"starting", "cancelling", "ready", "closed_filters_unknown", "closing_or_unknown", "closed-close-error", "closed-close-pending"} {
		t.Run(state, func(t *testing.T) {
			c := newBrowserEngineController(context.Background(), browserEngineDependencies{})
			r := &browserEngineRun{id: strings.Repeat("a", 64), done: make(chan struct{})}
			if state == "starting" {
				r.starting = true
			} else {
				done := make(chan struct{})
				close(done)
				s := &fakeEngineSession{status: browserenginehost.Status{State: state}, done: done}
				if strings.HasPrefix(state, "closed-") {
					s.status.State = "closed"
				}
				r.session = s
			}
			if state == "closed-close-error" {
				r.closeErr = errors.New("fixed fake close error")
			}
			if state == "closed-close-pending" {
				r.closePending = true
			}
			c.current = r
			called := false
			_, err := c.deleteVersion(browserengine.Chrome, "156.0.8078.47", func(context.Context, string, string) (browserengine.DeleteReceipt, error) {
				called = true
				return browserengine.DeleteReceipt{}, nil
			})
			if err != browserengine.ErrBusy || called || c.current != r {
				t.Fatal(err, called)
			}
		})
	}
}
func TestEngineControllerDeleteMaintenanceBlocksBeginAndKeepsStatusNonblocking(t *testing.T) {
	c, _, starts := engineFixture(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := c.deleteVersion(browserengine.Chrome, "156.0.8078.47", func(_ context.Context, id, version string) (browserengine.DeleteReceipt, error) {
			close(entered)
			<-release
			return browserengine.DeleteReceipt{EngineID: id, Version: version, Deleted: true}, nil
		})
		done <- err
	}()
	<-entered
	if _, err := c.beginLaunch(engineConfigFixture, engineProxyFixture); err == nil || starts.Load() != 0 {
		t.Fatal(err, starts.Load())
	}
	if _, err := c.deleteVersion(browserengine.Chrome, "156.0.8078.48", func(context.Context, string, string) (browserengine.DeleteReceipt, error) {
		t.Fatal("parallel delete")
		return browserengine.DeleteReceipt{}, nil
	}); err != browserengine.ErrBusy {
		t.Fatal(err)
	}
	statusDone := make(chan struct{})
	go func() { _ = c.status(); close(statusDone) }()
	select {
	case <-statusDone:
	case <-time.After(time.Second):
		t.Fatal("Status blocked on delete filesystem operation")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := c.launch(engineConfigFixture, engineProxyFixture); err != nil {
		t.Fatal("maintenance did not release", err)
	}
}
func TestEngineControllerDeleteFailureReleasesMaintenanceWithoutDroppingOwner(t *testing.T) {
	c, _, _ := engineFixture(t)
	_, err := c.deleteVersion(browserengine.Chrome, "156.0.8078.47", func(context.Context, string, string) (browserengine.DeleteReceipt, error) {
		return browserengine.DeleteReceipt{}, browserengine.ErrDeletePartial
	})
	if err != browserengine.ErrDeletePartial {
		t.Fatal(err)
	}
	if _, err := c.launch(engineConfigFixture, engineProxyFixture); err != nil {
		t.Fatal(err)
	}
	if _, err := c.deleteVersion(browserengine.Chrome, "156.0.8078.47", func(context.Context, string, string) (browserengine.DeleteReceipt, error) {
		t.Fatal("active run deleted")
		return browserengine.DeleteReceipt{}, nil
	}); err != browserengine.ErrBusy {
		t.Fatal(err)
	}
}
