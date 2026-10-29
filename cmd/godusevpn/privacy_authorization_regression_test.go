package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/identity"
	"github.com/Maoyangui/godusevpn/internal/privacyhost"
)

// Start rejects these inputs before opening a pipe, reading a token or showing
// UAC. Exercise the production pointer-to-interface boundary, which a fake
// returning an untyped nil does not cover.
func TestPrivacyLauncherKeepsRejectedConcreteClientNil(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"nil_context", nil},
		{"cancelled_context", ctx},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := launchPrivacyHelper(tc.ctx, "never-opened-synthetic-executable")
			if err == nil {
				t.Fatal("rejected start input returned no error")
			}
			if helper != nil {
				t.Fatal("nil concrete client became a non-nil helper interface")
			}
			if tc.ctx != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost cancellation: %v", err)
			}
		})
	}
}

func TestPrivacyHelperConcreteResultMatrix(t *testing.T) {
	client := new(privacyhost.Client) // Identity-only fixture; never used as a transport.
	failure := errors.New("synthetic launch failure")
	for _, tc := range []struct {
		name   string
		client *privacyhost.Client
		err    error
	}{
		{"uac_cancelled", nil, syscall.Errno(1223)},
		{"wrapped_uac_cancelled", nil, fmt.Errorf("launch: %w", syscall.Errno(1223))},
		{"failed", nil, failure},
		{"context_cancelled", nil, context.Canceled},
		{"missing_client_and_error", nil, nil},
		{"valid_identity", client, nil},
		{"partial_identity_with_error", client, failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			helper, err := privacyHelperFromClient(tc.client, tc.err)
			if (helper == nil) != (tc.client == nil) || err != tc.err {
				t.Fatal("pointer identity/error changed across interface boundary")
			}
			if tc.client != nil && helper != tc.client {
				t.Fatal("non-nil partial client was discarded before caller cleanup")
			}
		})
	}
}

type authorizationFixtureHelper struct {
	calls, closes, releases atomic.Int32
	done                    chan struct{}
	started                 chan struct{}
	closeOnce, startOnce    sync.Once
}

func newAuthorizationFixtureHelper(blockCall bool) *authorizationFixtureHelper {
	h := &authorizationFixtureHelper{done: make(chan struct{})}
	if blockCall {
		h.started = make(chan struct{})
	}
	return h
}

func (h *authorizationFixtureHelper) Call(ctx context.Context, _ string, _, _ any) error {
	h.calls.Add(1)
	if h.started == nil {
		return nil
	}
	h.startOnce.Do(func() { close(h.started) })
	select {
	case <-h.done:
		return net.ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *authorizationFixtureHelper) Close() error {
	h.closes.Add(1)
	h.closeOnce.Do(func() { h.releases.Add(1); close(h.done) })
	return nil
}

func awaitAuthorization(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("authorization lifecycle did not complete")
		return nil
	}
}

func TestPrivacyRejectedConcreteClientKeepsUIUsableAndExplicitRetry(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	for _, tc := range []struct {
		name, state string
		err         error
	}{
		{"uac_cancelled", "cancelled", syscall.Errno(1223)},
		{"wrapped_uac_cancelled", "cancelled", fmt.Errorf("launch: %w", syscall.Errno(1223))},
		{"launch_failed", "cancelled_or_unavailable", errors.New("synthetic launch failure")},
		{"missing_client", "cancelled_or_unavailable", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			launches := 0
			launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) {
				launches++
				var concrete *privacyhost.Client
				return privacyHelperFromClient(concrete, tc.err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := &App{ctx: ctx}
			a.startPrivacyHost(ctx)
			if a.privacyHost != nil || a.PrivacyHelperStatus().State != tc.state || ctx.Err() != nil || launches != 1 {
				t.Fatal("rejected helper changed GUI lifetime or claimed a session", a.PrivacyHelperStatus(), launches)
			}
			if err := a.hostCall("Apply", "never-sent-fixture", nil); err == nil || launches != 1 {
				t.Fatal("missing helper accepted a mutation or retried authorization")
			}
			h := newAuthorizationFixtureHelper(false)
			launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { launches++; return h, nil }
			state, err := a.PrivacyAuthorize()
			if err != nil || state.State != "connected" || a.privacyHost != h || launches != 2 {
				t.Fatal("explicit reauthorization did not establish one fresh session", state, err)
			}
			if _, err := a.PrivacyAuthorize(); err != nil || launches != 2 || h.calls.Load() != 0 {
				t.Fatal("connected helper was relaunched or mutated")
			}
			_ = h.Close()
		})
	}
}

func TestPrivacyPartialHelperFailureClosesExactlyOnce(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	h := newAuthorizationFixtureHelper(false)
	launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) {
		return h, errors.New("synthetic failure after partial launch")
	}
	a := &App{ctx: context.Background()}
	if _, err := a.PrivacyAuthorize(); err == nil || a.privacyHost != nil || h.closes.Load() != 1 || h.releases.Load() != 1 || h.calls.Load() != 0 {
		t.Fatal("partial failed helper was leaked, retained or used")
	}
}

func TestPrivacyLateAuthorizationIsRejectedAfterCancelOrShutdown(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	for _, mode := range []string{"cancel", "shutdown", "cancel_and_shutdown"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, finish := make(chan struct{}), make(chan struct{})
			h := newAuthorizationFixtureHelper(false)
			launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { close(entered); <-finish; return h, nil }
			// A zero tray is not ready, so actual shutdown performs no native tray
			// call. All optional controllers are nil and stay unopened.
			a := &App{ctx: context.Background(), tray: &trayUI{}}
			done := make(chan error, 1)
			go func() { done <- a.authorizePrivacyHost(ctx, true) }()
			<-entered
			if mode != "shutdown" {
				cancel()
			}
			if mode != "cancel" {
				a.shutdown(context.Background())
			}
			close(finish)
			if err := awaitAuthorization(t, done); err == nil || a.privacyHost != nil || h.closes.Load() != 1 || h.releases.Load() != 1 || h.calls.Load() != 0 {
				t.Fatal("late helper escaped cancellation/exit cleanup", err)
			}
			want := "closed"
			if mode == "cancel" {
				want = "unavailable"
			}
			if a.PrivacyHelperStatus().State != want {
				t.Fatal("late completion overwrote lifecycle state", a.PrivacyHelperStatus())
			}
			retries := 0
			fresh := newAuthorizationFixtureHelper(false)
			launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { retries++; return fresh, nil }
			_, err := a.PrivacyAuthorize()
			if mode == "cancel" {
				if err != nil || retries != 1 || a.privacyHost != fresh {
					t.Fatal("explicit retry after request cancellation was unavailable", err)
				}
				a.shutdown(context.Background())
			} else if err == nil || retries != 0 {
				t.Fatal("closed GUI relaunched a privileged helper")
			}
		})
	}
}

func TestPrivacyShutdownRacesAuthorizationCompletionWithoutResurrection(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	for i := 0; i < 64; i++ {
		entered, finish := make(chan struct{}), make(chan struct{})
		h := newAuthorizationFixtureHelper(false)
		launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { close(entered); <-finish; return h, nil }
		a := &App{ctx: context.Background(), tray: &trayUI{}}
		done, stopped := make(chan error, 1), make(chan struct{})
		go func() { done <- a.authorizePrivacyHost(context.Background(), true) }()
		<-entered
		go func() { <-finish; a.shutdown(context.Background()); close(stopped) }()
		close(finish)
		_ = awaitAuthorization(t, done) // Either lock winner is permitted.
		<-stopped
		if a.PrivacyHelperStatus().State != "closed" || a.privacyHost != nil || h.closes.Load() != 1 || h.releases.Load() != 1 || h.calls.Load() != 0 {
			t.Fatal("concurrent exit retained/reopened a helper", i)
		}
	}
}

func TestPrivacyShutdownWithInFlightCallKeepsClosedState(t *testing.T) {
	h := newAuthorizationFixtureHelper(true)
	a := &App{ctx: context.Background(), tray: &trayUI{}, privacyHost: h, privacyHostState: "connected"}
	done := make(chan error, 1)
	go func() { done <- a.hostCall("Report", nil, nil) }()
	<-h.started
	a.shutdown(context.Background())
	if err := awaitAuthorization(t, done); err == nil || a.PrivacyHelperStatus().State != "closed" || a.privacyHost != nil || h.calls.Load() != 1 || h.releases.Load() != 1 {
		t.Fatal("in-flight call retried, leaked, or overwrote the closed GUI", err)
	}
}
