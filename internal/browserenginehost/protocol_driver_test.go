package browserenginehost

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

type driverCommand struct {
	ID      uint64         `json:"id"`
	Method  string         `json:"method"`
	Session string         `json:"sessionId"`
	Params  map[string]any `json:"params"`
}
type driverFixture struct {
	m               *memoryTransport
	config          Config
	launch          protocolLaunch
	mu              sync.Mutex
	commands        []driverCommand
	fault           string
	faultSession    string
	faultRequest    string
	mutate          func(driverCommand, map[string]any)
	attachedWaiting bool
	ua, platform    string
	oscpu           string
	hints           browserhost.ClientHints
	dpr             float64
	touch           int
	hardware        map[string]int
}

func newDriverFixture(t *testing.T, engine string) *driverFixture {
	t.Helper()
	c := Config{EngineID: engine, Version: "130.0.6723.1", ProfileID: "fixture", Language: "en-US", Timezone: "America/Los_Angeles", Width: 1280, Height: 720, Proxy: browserproxy.Config{Type: "socks5", Host: "fixture.invalid", Port: 1080}}
	if engine == "firefox" {
		c.Version = "147.0.1"
	}
	l := protocolLaunch{RelayPort: 43123, ProfileDirectory: `C:\fixture\private-profile`}
	l.Arguments = arguments(c, l.ProfileDirectory, l.RelayPort, 44123, 123, 124)
	f := &driverFixture{m: newMemoryTransport(), config: c, launch: l, attachedWaiting: true, ua: "Mozilla/5.0 Chrome/130.0.0.0 Safari/537.36", platform: "Win32", dpr: 1, hardware: map[string]int{}}
	if engine == "firefox" {
		f.ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:147.0) Gecko/20100101 Firefox/147.0"
		f.oscpu = "Windows NT 10.0; Win64; x64"
	}
	json.Unmarshal([]byte(driverNativeHints(c)), &f.hints)
	t.Cleanup(func() { f.m.Close() })
	return f
}
func (f *driverFixture) send(value any) {
	raw, _ := json.Marshal(value)
	select {
	case f.m.reads <- raw:
	case <-f.m.done:
	}
}
func (f *driverFixture) event(method string, params any, session string) {
	v := map[string]any{"method": method, "params": params}
	if session != "" {
		v["sessionId"] = session
	}
	f.send(v)
}
func (f *driverFixture) start() {
	if f.config.EngineID == "firefox" {
		if preset, _ := selectedEnvironment(f.config); preset != nil {
			f.platform = preset.NavigatorPlatform
			f.touch = preset.MaxTouchPoints
			f.oscpu = firefoxOSCPU(preset)
		}
	}
	go func() {
		for {
			select {
			case raw := <-f.m.writes:
				var c driverCommand
				if json.Unmarshal(raw, &c) != nil {
					return
				}
				f.mu.Lock()
				f.commands = append(f.commands, c)
				f.mu.Unlock()
				request, _ := c.Params["request"].(string)
				if c.Method == f.fault && (f.faultSession == "" || c.Session == f.faultSession) && (f.faultRequest == "" || strings.HasPrefix(request, f.faultRequest)) {
					f.send(map[string]any{"id": c.ID, "error": map[string]any{"code": -32601, "message": "fixture unsupported"}})
					continue
				}
				result := f.result(c)
				if f.mutate != nil {
					f.mutate(c, result)
				}
				f.send(map[string]any{"id": c.ID, "result": result})
			case <-f.m.done:
				return
			}
		}
	}()
}
func (f *driverFixture) result(c driverCommand) map[string]any {
	switch c.Method {
	case "Browser.getVersion":
		return map[string]any{"product": "Chrome/" + f.config.Version, "userAgent": "Mozilla/5.0 Chrome/130.0.0.0 Safari/537.36"}
	case "Browser.getBrowserCommandLine":
		return map[string]any{"arguments": append([]string{`C:\fixture\chrome.exe`}, f.launch.Arguments...)}
	case "Target.createBrowserContext":
		return map[string]any{"browserContextId": "owned-context"}
	case "Target.createTarget":
		f.event("Target.attachedToTarget", map[string]any{"sessionId": "main-session", "waitingForDebugger": f.attachedWaiting, "targetInfo": map[string]any{"targetId": "main-target", "type": "page", "browserContextId": "owned-context"}}, "")
		return map[string]any{"targetId": "main-target"}
	case "Target.closeTarget":
		return map[string]any{"success": true}
	case "Page.addScriptToEvaluateOnNewDocument":
		return map[string]any{"identifier": "preload"}
	case "Runtime.evaluate":
		if c.Params["expression"] == protocolHardwareReadback {
			count := f.hardware[c.Session]
			if count == 0 {
				count = 8
			}
			raw, _ := json.Marshal(map[string]int{"hardwareConcurrency": count})
			return map[string]any{"result": map[string]any{"type": "string", "value": string(raw)}}
		}
		if c.Params["expression"] == protocolNativeIdentity {
			raw, _ := json.Marshal(map[string]any{"userAgent": f.ua, "navigatorPlatform": f.platform, "maxTouchPoints": f.touch, "devicePixelRatio": f.dpr, "clientHints": json.RawMessage(driverNativeHints(f.config))})
			return map[string]any{"result": map[string]any{"type": "string", "value": string(raw)}}
		}
		if strings.HasPrefix(c.Params["expression"].(string), "JSON.stringify({ready:") {
			return map[string]any{"result": map[string]any{"type": "string", "value": `{"ready":true}`}}
		}
		return map[string]any{"result": map[string]any{"type": "string", "value": f.observation()}}
	case "Page.navigate":
		url, _ := c.Params["url"].(string)
		if strings.HasPrefix(url, "https://godusevpn.invalid/owned/") {
			ua := f.ua
			if f.fault == "http-ua" {
				ua += " mismatch"
			}
			f.event("Fetch.requestPaused", map[string]any{"requestId": url, "resourceType": "Document", "request": map[string]any{"url": url, "headers": map[string]string{"User-Agent": ua}}}, "main-session")
		}
		return map[string]any{"frameId": "main-frame"}
	case "Emulation.setUserAgentOverride":
		f.ua = c.Params["userAgent"].(string)
		f.platform = c.Params["platform"].(string)
		raw, _ := json.Marshal(c.Params["userAgentMetadata"])
		json.Unmarshal(raw, &f.hints)
		return map[string]any{}
	case "Emulation.setDeviceMetricsOverride":
		if value, _ := c.Params["deviceScaleFactor"].(float64); value > 0 {
			f.dpr = value
		}
		return map[string]any{}
	case "Emulation.setTouchEmulationEnabled":
		f.touch = 0
		if c.Params["enabled"] == true {
			f.touch = int(c.Params["maxTouchPoints"].(float64))
		}
		return map[string]any{}
	case "Emulation.setHardwareConcurrencyOverride":
		f.hardware[c.Session] = int(c.Params["hardwareConcurrency"].(float64))
		return map[string]any{}
	case "session.new":
		return map[string]any{"sessionId": "bidi-session", "capabilities": map[string]any{"browserName": "firefox", "browserVersion": f.config.Version, "userAgent": f.ua, "acceptInsecureCerts": false, "proxy": map[string]any{"proxyType": "manual", "socksProxy": "127.0.0.1:43123", "socksVersion": 5, "noProxy": []string{}}}}
	case "browsingContext.getTree":
		if c.Params["root"] != nil {
			return map[string]any{"contexts": []any{map[string]any{"context": "main-context", "children": []any{map[string]any{"context": "child-context"}}}}}
		}
		return map[string]any{"contexts": []any{map[string]any{"context": "launcher-blank", "url": "about:blank"}}}
	case "session.subscribe":
		for _, event := range c.Params["events"].([]any) {
			if event == "browsingContext.contextCreated" {
				f.event("browsingContext.contextCreated", map[string]any{"context": "launcher-blank", "parent": nil, "userContext": "default"}, "")
			}
		}
		return map[string]any{}
	case "browser.createUserContext":
		return map[string]any{"userContext": "owned-user-context"}
	case "browsingContext.create":
		f.event("browsingContext.contextCreated", map[string]any{"context": "main-context", "parent": nil, "userContext": "owned-user-context"}, "")
		return map[string]any{"context": "main-context"}
	case "script.addPreloadScript":
		return map[string]any{"script": "preload"}
	case "network.addIntercept":
		return map[string]any{"intercept": "fixed-intercept"}
	case "script.evaluate":
		if expression, ok := c.Params["expression"].(string); ok && strings.HasPrefix(expression, "(async function ultraGraphicsProbe(){") {
			cross := regexp.MustCompile(`https://godusevpn-graphics\.invalid/owned/[0-9a-f]{48}`).FindString(expression)
			if f.fault != "graphics-cross-request-missing" {
				ua := f.ua
				if f.fault == "graphics-cross-http-ua" {
					ua += " mismatch"
				}
				f.event("network.beforeRequestSent", map[string]any{"context": "child-context", "isBlocked": true, "request": map[string]any{"request": cross, "url": cross, "destination": "iframe", "headers": []any{map[string]any{"name": "User-Agent", "value": map[string]string{"type": "string", "value": ua}}}}}, "")
			}
			return map[string]any{"type": "success", "realm": "main-realm", "result": map[string]any{"type": "string", "value": fixtureGraphicsReadback(f.config)}}
		}
		if c.Params["expression"] == protocolNativeFirefoxIdentity {
			raw, _ := json.Marshal(map[string]any{"userAgent": f.ua, "navigatorPlatform": f.platform, "maxTouchPoints": f.touch, "devicePixelRatio": f.dpr, "oscpu": f.oscpu})
			return map[string]any{"type": "success", "realm": "main-realm", "result": map[string]any{"type": "string", "value": string(raw)}}
		}
		return map[string]any{"type": "success", "realm": "main-realm", "result": map[string]any{"type": "string", "value": f.observation()}}
	case "browsingContext.navigate":
		url, _ := c.Params["url"].(string)
		if strings.HasPrefix(url, "https://godusevpn.invalid/owned/") {
			ua := f.ua
			if f.fault == "http-ua" {
				ua += " mismatch"
			}
			f.event("network.beforeRequestSent", map[string]any{"context": "main-context", "isBlocked": true, "request": map[string]any{"request": url, "url": url, "destination": "document", "headers": []any{map[string]any{"name": "User-Agent", "value": map[string]string{"type": "string", "value": ua}}}}}, "")
		}
		return map[string]any{"url": c.Params["url"], "navigation": "navigation"}
	case "emulation.setUserAgentOverride":
		f.ua = c.Params["userAgent"].(string)
		return map[string]any{}
	case "browsingContext.setViewport":
		if dpr, _ := c.Params["devicePixelRatio"].(float64); dpr > 0 {
			f.dpr = dpr
		}
		return map[string]any{}
	}
	return map[string]any{}
}

func driverNativeHints(c Config) string {
	major := strings.Split(c.Version, ".")[0]
	value := map[string]any{"brands": []map[string]string{{"brand": "Chromium", "version": major}, {"brand": "Google Chrome", "version": major}, {"brand": "Not=A?Brand", "version": "99"}}, "fullVersionList": []map[string]string{{"brand": "Chromium", "version": c.Version}, {"brand": "Google Chrome", "version": c.Version}, {"brand": "Not=A?Brand", "version": "99.0.0.0"}}, "platform": "Windows", "platformVersion": "15.0.0", "architecture": "x86", "bitness": "64", "mobile": false, "model": ""}
	raw, _ := json.Marshal(value)
	return string(raw)
}
func (f *driverFixture) observation() string {
	var value struct {
		Top    protocolObservation   `json:"top"`
		Frames []protocolObservation `json:"frames"`
	}
	json.Unmarshal([]byte(driverObservation(f.config)), &value)
	apply := func(o *protocolObservation) {
		count := f.hardware["main-session"]
		if count == 0 {
			count = 8
		}
		o.HardwareConcurrency = &count
		o.UserAgent = f.ua
		o.NavigatorPlatform = f.platform
		o.MaxTouchPoints = f.touch
		o.DevicePixelRatio = f.dpr
		h := f.hints
		o.ClientHints = &h
		o.OSCPU = f.oscpu
		if f.config.EngineID == "firefox" {
			o.ClientHints = nil
		}
		if o.Kind == "opaque-origin" {
			o.ClientHints = nil
		}
	}
	apply(&value.Top)
	for i := range value.Frames {
		apply(&value.Frames[i])
	}
	raw, _ := json.Marshal(value)
	return string(raw)
}
func (f *driverFixture) snapshot() []driverCommand {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]driverCommand(nil), f.commands...)
}
func (f *driverFixture) count(method string) int {
	n := 0
	for _, c := range f.snapshot() {
		if c.Method == method {
			n++
		}
	}
	return n
}
func (f *driverFixture) waitMethod(t *testing.T, method string) driverCommand {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		for _, c := range f.snapshot() {
			if c.Method == method {
				return c
			}
		}
		select {
		case <-deadline:
			t.Fatal("missing method", method)
		case <-tick.C:
		}
	}
}

func driverObservation(c Config) string {
	observation := func(kind string) protocolObservation {
		offsets, _ := protocolTimezoneOffsets(c.Timezone)
		return protocolObservation{Language: c.Language, Locale: c.Language, Timezone: c.Timezone, JanuaryOffset: offsets[0], JulyOffset: offsets[1], Width: c.Width, Height: c.Height, Boundary: true, WorkerBlocked: true, Kind: kind}
	}
	value := map[string]any{"top": observation(""), "frames": []protocolObservation{observation("same-origin"), observation("opaque-origin")}}
	raw, _ := json.Marshal(value)
	return string(raw)
}

func TestProtocolDriverReadbackRejectsMissingOrConflictingEvidence(t *testing.T) {
	f := newDriverFixture(t, "chrome-for-testing")
	if verifyProtocolReadback(driverObservation(f.config), f.config) != nil {
		t.Fatal("valid observation rejected")
	}
	for _, raw := range []string{`{}`, `null`, strings.Replace(driverObservation(f.config), `"workerBlocked":true`, `"workerBlocked":false`, 1), strings.Replace(driverObservation(f.config), `"en-US"`, `"de-DE"`, 1), strings.ReplaceAll(driverObservation(f.config), `"width":1280`, `"width":800`), strings.Replace(driverObservation(f.config), `"januaryOffset":480`, `"januaryOffset":0`, 1), strings.Replace(driverObservation(f.config), `"opaque-origin"`, `"same-origin"`, 1), strings.Replace(driverObservation(f.config), `"top":`, `"top":{},"top":`, 1)} {
		if verifyProtocolReadback(raw, f.config) == nil {
			t.Fatal("accepted inconsistent readback")
		}
	}
}

func TestProtocolDriverOnlyHTTPAndNoNavigationBeforeReadiness(t *testing.T) {
	for _, engine := range []string{"chrome-for-testing", "firefox"} {
		f := newDriverFixture(t, engine)
		f.start()
		var d protocolDriver
		if engine == "firefox" {
			d = newFirefoxDriver(f.m, f.config, f.launch, nil)
		} else {
			d = newChromeDriver(f.m, f.config, f.launch, nil)
		}
		if d.Navigate(context.Background(), "https://example.invalid") == nil {
			t.Fatal("navigation before Configure")
		}
		if err := d.Configure(context.Background()); err != nil {
			t.Fatal(engine, err)
		}
		for _, url := range []string{"file:///C:/x", "javascript:1", "data:text/html,x", "mailto:x@example.invalid", "https://user:secret@example.invalid", "https://example.invalid\n"} {
			if d.Navigate(context.Background(), url) == nil {
				t.Fatal("accepted disallowed navigation", engine)
			}
		}
		if err := d.Navigate(context.Background(), "https://example.invalid/test"); err != nil {
			t.Fatal(engine, err)
		}
		d.Close()
		if d.Navigate(context.Background(), "https://example.invalid") == nil {
			t.Fatal("navigation after Close")
		}
	}
}
