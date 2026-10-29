package main

import (
	"context"
	"testing"
)

func TestWindowMaximizeAndRestoreReadActualStateWithoutNativeCalls(t *testing.T) {
	oldToggle, oldRead := toggleWindowMaximize, readWindowMaximized
	t.Cleanup(func() { toggleWindowMaximize, readWindowMaximized = oldToggle, oldRead })
	ctx := context.WithValue(context.Background(), "frontend", "synthetic-runtime-context")
	state, toggles, reads := false, 0, 0
	toggleWindowMaximize = func(got context.Context) {
		if got != ctx {
			t.Fatal("foreign runtime context")
		}
		toggles++
		state = !state
	}
	readWindowMaximized = func(got context.Context) bool {
		if got != ctx {
			t.Fatal("foreign runtime context")
		}
		reads++
		return state
	}
	a := &App{ctx: ctx}
	for _, want := range []bool{true, false} {
		got, err := a.ToggleMaximize()
		if err != nil || got != want {
			t.Fatal("actual state was not read after toggle", got, err)
		}
	}
	state = true // Simulate the user maximizing through native window controls.
	if got, err := a.IsMaximized(); err != nil || !got || toggles != 2 || reads != 3 {
		t.Fatal("query toggled or ignored actual native state")
	}
}

func TestWindowControlsRefuseMissingAndCancelledLifecycleContext(t *testing.T) {
	oldToggle, oldRead := toggleWindowMaximize, readWindowMaximized
	t.Cleanup(func() { toggleWindowMaximize, readWindowMaximized = oldToggle, oldRead })
	toggleWindowMaximize = func(context.Context) { t.Fatal("invalid context reached runtime") }
	readWindowMaximized = func(context.Context) bool { t.Fatal("invalid context reached runtime"); return false }
	cancelled, cancel := context.WithCancel(context.WithValue(context.Background(), "frontend", true))
	cancel()
	for _, ctx := range []context.Context{nil, context.Background(), cancelled} {
		a := &App{ctx: ctx}
		if _, err := a.ToggleMaximize(); err == nil || err.Error() != "window_not_ready" {
			t.Fatal("invalid toggle allowed")
		}
		if _, err := a.IsMaximized(); err == nil || err.Error() != "window_not_ready" {
			t.Fatal("invalid query allowed")
		}
	}
}
