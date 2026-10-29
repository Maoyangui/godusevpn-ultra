package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
	"github.com/Maoyangui/godusevpn/internal/detection"
)

func TestOwnedBrowserObservationDoesNotCreateOrOperateBackend(t *testing.T) {
	a := &App{ctx: context.Background()}
	r := detection.Report{}
	attachOwnedBrowserObservation(&r, a)
	if a.browserEngine != nil || len(r.Findings) != 1 || r.Findings[0].DetectedAt == "" {
		t.Fatal("read created native backend or lost provenance", r)
	}
	c := newBrowserEngineController(a.ctx, browserEngineDependencies{
		manager: func(browserproxy.Config) (*browserengine.Manager, error) { panic("network/cache manager must not run") },
		guard:   func() (browserenginehost.Guard, error) { panic("helper must not run") },
		start: func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
			panic("native browser must not run")
		},
	})
	c.last = BrowserEngineLaunchResult{StartID: "SYNTHETIC_SESSION_MUST_NOT_EXPORT", Code: "SECRET_FAILURE_CODE", Detail: "SECRET_PROXY_OR_PATH", Status: browserenginehost.Status{State: "closed_filters_unknown", Detail: "SECRET_HOST_DETAIL", SessionID: "SECRET_PROFILE_ID"}}
	a.browserEngine = c
	r = detection.Report{}
	attachOwnedBrowserObservation(&r, a)
	encoded, err := json.Marshal(r)
	if err != nil || a.browserEngine != c || len(r.Findings) != 1 || r.Findings[0].Status != "unknown" || strings.Contains(string(encoded), "SECRET_") || strings.Contains(string(encoded), "SYNTHETIC_SESSION") {
		t.Fatal("read mutated ownership or exposed private controller data", string(encoded), err)
	}
	if c.current != nil || c.closed || c.last.Code != "SECRET_FAILURE_CODE" {
		t.Fatal("observation changed lifetime state")
	}
}
