//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/identity"
)

func TestEngineGuardBridgeNoAttemptRequiresCurrentCompleteFrame(t *testing.T) {
	for _, mode := range []string{"current", "transport_error", "epoch_replaced", "cancelled_during_call"} {
		t.Run(mode, func(t *testing.T) {
			a, h, g := engineGuardBridgeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id := h.result.SessionID
			h.result = browserengineguard.Result{SessionID: id, Verified: true, State: "install_not_attempted", Diagnostic: &browserengineguard.Diagnostic{Stage: "owner_token_open", NativeCode: 5}}
			if mode == "transport_error" {
				h.err = errors.New("PRIVATE_PARTIAL_FRAME")
			}
			if mode == "epoch_replaced" {
				h.during = func() { a.privacyHostEpoch++ }
			}
			if mode == "cancelled_during_call" {
				h.during = cancel
			}
			result, err := g.Install(ctx, browserengineguard.InstallRequest{SessionID: id})
			if err == nil || browserengineguard.NoInstallWasAttempted(result, id) != (mode == "current") {
				t.Fatal("stale/partial frame certified no install", result, err)
			}
			if mode == "current" {
				d := browserengineguard.DiagnosticFromError(err)
				if d == nil || d.NativeCode != 5 || d.Stage != "owner_token_open" {
					t.Fatal("precise error discarded", err)
				}
			} else if result.Diagnostic == nil || result.Diagnostic.Stage != "guard_transport" || result.Diagnostic.NativeCode != 0 {
				t.Fatal("partial or obsolete diagnostic was trusted", result.Diagnostic)
			}
			if len(h.calls) != 1 {
				t.Fatal("failed mutation retried")
			}
		})
	}
}

type browserCapabilityFixture struct {
	*authorizationFixtureHelper
	capability   browserengineguard.Capability
	err          error
	observe      func()
	observations int
}

func (h *browserCapabilityFixture) BrowserGuardCapability(ctx context.Context) (browserengineguard.Capability, error) {
	if _, ok := ctx.Deadline(); !ok {
		panic("unbounded capability query")
	}
	h.observations++
	if h.observe != nil {
		h.observe()
	}
	return h.capability, h.err
}

func TestPrivacyBrowserGuardFactoryFailureDoesNotClaimHelperDisconnected(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra Windows helper")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	for _, mode := range []string{"available", "factory_denied", "invalid_capability", "transport_error", "cancelled_late"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h := &browserCapabilityFixture{authorizationFixtureHelper: newAuthorizationFixtureHelper(false), capability: browserengineguard.Capability{Available: true}}
			if mode == "factory_denied" {
				h.capability = browserengineguard.Capability{Diagnostic: &browserengineguard.Diagnostic{Stage: "owner_token_open", NativeCode: 5}}
			}
			if mode == "invalid_capability" {
				h.capability = browserengineguard.Capability{Diagnostic: &browserengineguard.Diagnostic{Stage: "PRIVATE_PATH"}}
			}
			if mode == "transport_error" {
				h.err = errors.New("PRIVATE_NONCE")
			}
			if mode == "cancelled_late" {
				h.observe = cancel
			}
			launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { return h, nil }
			a := &App{ctx: ctx}
			err := a.authorizePrivacyHost(ctx, true)
			state := a.PrivacyHelperStatus()
			if h.observations != 1 || h.calls.Load() != 0 {
				t.Fatal("capability check crossed policy boundary")
			}
			if mode == "cancelled_late" {
				if err == nil || a.privacyHost != nil || state.BrowserGuard != nil || h.closes.Load() != 1 {
					t.Fatal("late capability escaped shutdown")
				}
				return
			}
			defer h.Close()
			if err != nil || state.State != "connected" || state.BrowserGuard == nil || state.BrowserGuard.Available != (mode == "available") {
				t.Fatal("basic helper and browser guard conflated", state, err)
			}
			if mode == "available" {
				return
			}
			wantStage := "guard_transport"
			if mode == "factory_denied" {
				wantStage = "owner_token_open"
			}
			if state.BrowserGuard.Diagnostic.Stage != wantStage || strings.Contains(state.Detail, "PRIVATE") {
				t.Fatal("unsafe or missing factory diagnostic", state)
			}
			state.BrowserGuard.Diagnostic.Stage = "mutated"
			if a.PrivacyHelperStatus().BrowserGuard.Diagnostic.Stage != wantStage {
				t.Fatal("status consumer changed cached authority")
			}
			if guard, e := a.fixedBrowserEngineGuard(); e == nil || guard != nil || h.calls.Load() != 0 {
				t.Fatal("known unavailable guard reached browser launch")
			}
		})
	}
}
