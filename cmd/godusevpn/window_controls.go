package main

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var toggleWindowMaximize = runtime.WindowToggleMaximise
var readWindowMaximized = runtime.WindowIsMaximised

func windowContextReady(ctx context.Context) bool {
	// Wails terminates the process for a missing runtime context. Reject an
	// early/late call before invoking its runtime. Only lifecycle owns a.ctx.
	return ctx != nil && ctx.Err() == nil && ctx.Value("frontend") != nil
}

// ToggleMaximize uses the existing ordinary-user Wails window. The boolean is
// a fresh native state query after the toggle, not a frontend-predicted state.
// Other platforms use Wails' own window implementation and retain their own
// existing startup geometry/native window controls.
func (a *App) ToggleMaximize() (bool, error) {
	if !windowContextReady(a.ctx) {
		return false, errors.New("window_not_ready")
	}
	toggleWindowMaximize(a.ctx)
	return readWindowMaximized(a.ctx), nil
}

// IsMaximized lets the frontend synchronize its maximize/restore label after
// native window resizing or a titlebar double-click, without toggling again.
func (a *App) IsMaximized() (bool, error) {
	if !windowContextReady(a.ctx) {
		return false, errors.New("window_not_ready")
	}
	return readWindowMaximized(a.ctx), nil
}
