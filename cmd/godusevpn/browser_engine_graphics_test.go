package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

func firefoxGraphicsRequest() string {
	base := strings.Replace(strings.Replace(engineConfigFixture, `"chrome-for-testing"`, `"firefox"`, 1), `"130.0.6723.58"`, `"147.0.1"`, 1)
	return `{"webgpu":"disabled","graphics":{"mode":"webgl-metadata","presetId":"manual","vendor":"Selected vendor","renderer":"Selected renderer"},` + base[1:]
}
func TestEngineControllerFirefoxGraphicsReachesNativeHostWithoutMutation(t *testing.T) {
	c, session, _ := engineFixture(t)
	session.status.EngineID = "firefox"
	session.status.Version = "147.0.1"
	var observed browserenginehost.Config
	c.deps.start = func(_ context.Context, _ *browserengine.Manager, v browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
		observed = v
		return session, nil
	}
	if _, err := c.launch(firefoxGraphicsRequest(), engineProxyFixture); err != nil {
		t.Fatal(err)
	}
	if observed.WebGPU != "disabled" || observed.Graphics == nil || observed.Graphics.Vendor != "Selected vendor" || observed.Graphics.Renderer != "Selected renderer" {
		t.Fatal("native graphics request dropped or mutated")
	}
}
func TestEngineControllerRejectsMalformedGraphicsAndUnimplementedChromeBeforeLaunch(t *testing.T) {
	valid := firefoxGraphicsRequest()
	for _, bad := range []string{
		strings.Replace(valid, `"graphics":{`, `"Graphics":{`, 1),
		strings.Replace(valid, `"webgpu":"disabled"`, `"webgpu":null`, 1),
		strings.Replace(valid, `"webgpu":"disabled"`, `"webgpu":""`, 1),
		strings.Replace(valid, `"webgpu":"disabled"`, `"webgpu":"runtime-default"`, 1),
		strings.Replace(valid, `"vendor":"Selected vendor"`, `"Vendor":"Selected vendor"`, 1),
		strings.Replace(valid, `"vendor":"Selected vendor"`, `"vendor":"Selected vendor","vendor":"other"`, 1),
		strings.Replace(valid, `"vendor":"Selected vendor"`, `"vendor":null`, 1),
		strings.Replace(valid, `"vendor":"Selected vendor"`, `"vendor":"bad\nvalue"`, 1),
		strings.Replace(valid, `"vendor":"Selected vendor"`, `"vendor":" Selected vendor"`, 1),
		strings.Replace(valid, `"renderer":"Selected renderer"`, `"renderer":"Selected renderer","script":"arbitrary"`, 1),
		strings.Replace(valid, `"firefox"`, `"chrome-for-testing"`, 1),
	} {
		if _, err := parseBrowserEngineRequest(bad); err == nil {
			t.Fatal("unsafe or unsupported graphics request accepted")
		}
	}
	var value map[string]any
	json.Unmarshal([]byte(valid), &value)
	for _, graphics := range []any{nil, map[string]any{}, map[string]any{"mode": "native", "vendor": "unexpected"}} {
		value["graphics"] = graphics
		raw, _ := json.Marshal(value)
		if _, err := parseBrowserEngineRequest(string(raw)); err == nil {
			t.Fatal("null/empty/invalid native graphics accepted")
		}
	}
}
func TestEngineControllerGraphicsReadbackFailureRetainsFixedCodeAndCleansLateOwner(t *testing.T) {
	c, session, _ := engineFixture(t)
	c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		return session, browserenginehost.ErrGraphicsReadback
	}
	if _, err := c.launch(firefoxGraphicsRequest(), engineProxyFixture); err == nil {
		t.Fatal("graphics failure reported ready")
	}
	result := c.status()
	if result.Code != "graphics_readback_failed" || result.Status.Ready || session.closes.Load() == 0 {
		t.Fatal("fixed code/failed startup cleanup missing", result)
	}
}
