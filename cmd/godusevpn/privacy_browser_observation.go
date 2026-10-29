package main

import (
	"time"

	"github.com/Maoyangui/godusevpn/internal/detection"
)

// Reading detection must not create a controller or call the native protocol,
// helper, filesystem, network, or an installed browser. Copy bounded state only.
func attachOwnedBrowserObservation(r *detection.Report, a *App) {
	in := detection.OwnedBrowser{}
	if a != nil {
		a.browserEngineMu.Lock()
		controller := a.browserEngine
		a.browserEngineMu.Unlock()
		if controller != nil {
			result := controller.status()
			in.Tracked = result.StartID != ""
			in.Engine, in.State = result.Status.EngineID, result.Status.State
			in.Ready, in.CanNavigate = result.Status.Ready, result.Status.CanNavigate
			if g := result.Status.Graphics; g != nil && g.Implementation == "gecko-native-preferences" && g.WebGPU == "native-pref-disabled" && g.WorkerExecutionBlocked && !g.WorkerAPIRuntimeReadback {
				in.NativeGraphicsRecord = true
				in.GraphicsWindowScopes, in.GraphicsContexts, in.OffscreenAbsentScopes = g.WindowScopesVerified, g.WebGLContextsVerified, g.OffscreenAPIAbsentScopes
			}
		}
	}
	in.ReadAt = time.Now()
	detection.AppendOwnedBrowser(r, in)
}
