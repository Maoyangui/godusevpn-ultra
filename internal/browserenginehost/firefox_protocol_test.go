package browserenginehost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

func TestFirefoxDriverScopedOverridesAndLastTabOrdering(t *testing.T) {
	f := newDriverFixture(t, "firefox")
	f.start()
	d := newFirefoxDriver(f.m, f.config, f.launch, nil)
	defer d.Close()
	if err := d.Configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range f.snapshot() {
		if c.Method == "browsingContext.create" {
			for _, method := range []string{"emulation.setLocaleOverride", "emulation.setTimezoneOverride", "browsingContext.setViewport", "browser.setDownloadBehavior", "script.addPreloadScript", "network.addIntercept"} {
				if !seen[method] {
					t.Fatal("new tab before override", method)
				}
			}
		}
		if c.Method == "browsingContext.close" && !seen["browsingContext.create"] {
			t.Fatal("closed Firefox last tab before replacement")
		}
		if c.Method == "emulation.setLocaleOverride" || c.Method == "emulation.setTimezoneOverride" || c.Method == "browsingContext.setViewport" {
			v, _ := json.Marshal(c.Params["userContexts"])
			if string(v) != `["owned-user-context"]` {
				t.Fatal("emulation not user-context scoped")
			}
		}
		seen[c.Method] = true
	}
	if !seen["script.evaluate"] || !seen["browsingContext.close"] {
		t.Fatal("missing readback or blank cleanup")
	}
}

func TestFirefoxDriverOfficialUAAndNativeEnvironmentReadback(t *testing.T) {
	for _, preset := range browserhost.EnvironmentPresets() {
		for _, mode := range []string{"generated", "manual", "native"} {
			t.Run(preset.Config.OS+"/"+mode, func(t *testing.T) {
				f := newDriverFixture(t, "firefox")
				env := preset.Config
				f.config.Environment = &env
				f.config.Identity = browserhost.ProfileIdentity{Mode: mode}
				if mode == "manual" {
					f.config.Identity.UserAgent = "Exact edited Gecko text/19"
				}
				f.config.Width = preset.RecommendedViewport.Width
				f.config.Height = preset.RecommendedViewport.Height
				originalUA := f.ua
				f.start()
				d := newFirefoxDriver(f.m, f.config, f.launch, nil)
				defer d.Close()
				if err := d.Configure(context.Background()); err != nil {
					t.Fatal(err)
				}
				override := f.waitMethod(t, "emulation.setUserAgentOverride")
				if mode == "manual" && override.Params["userAgent"] != f.config.Identity.UserAgent {
					t.Fatal("manual UA changed")
				}
				if mode == "native" && override.Params["userAgent"] != originalUA {
					t.Fatal("native UA changed when an independent environment was requested")
				}
				encoded, _ := json.Marshal(override.Params["userContexts"])
				if string(encoded) != `["owned-user-context"]` {
					t.Fatal("UA not scoped to owned context")
				}
				viewport := f.waitMethod(t, "browsingContext.setViewport")
				if viewport.Params["devicePixelRatio"] != preset.DeviceScaleFactor {
					t.Fatal("DPR not applied")
				}
				if f.count("network.provideResponse") != 2 {
					t.Fatal("HTTP UA not read before and after override")
				}
			})
		}
	}
}

func TestFirefoxDriverIdentityFailureRefusesSession(t *testing.T) {
	for _, fault := range []string{"emulation.setUserAgentOverride", "network.provideResponse", "http-ua", "ua-readback", "platform-readback", "touch-readback", "dpr-readback", "oscpu-readback"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			env := browserhost.EnvironmentPresets()[2].Config
			f.config.Environment = &env
			f.config.Identity = browserhost.ProfileIdentity{Mode: "generated"}
			f.fault = fault
			f.mutate = func(c driverCommand, r map[string]any) {
				if c.Method != "script.evaluate" || c.Params["expression"] != protocolReadback {
					return
				}
				raw := f.observation()
				switch fault {
				case "ua-readback":
					raw = strings.Replace(raw, `"userAgent":`, `"userAgent":"wrong","ignoredUA":`, 1)
				case "platform-readback":
					raw = strings.Replace(raw, `"navigatorPlatform":"Linux armv8l"`, `"navigatorPlatform":"Win32"`, 1)
				case "touch-readback":
					raw = strings.Replace(raw, `"maxTouchPoints":5`, `"maxTouchPoints":0`, 1)
				case "dpr-readback":
					raw = strings.Replace(raw, `"devicePixelRatio":2.625`, `"devicePixelRatio":1`, 1)
				case "oscpu-readback":
					raw = strings.Replace(raw, `"oscpu":"Linux armv8l"`, `"oscpu":"Windows NT 10.0"`, 1)
				}
				r["result"] = map[string]any{"type": "string", "value": raw}
			}
			f.start()
			d := newFirefoxDriver(f.m, f.config, f.launch, nil)
			defer d.Close()
			if d.Configure(context.Background()) == nil {
				t.Fatal("unverified identity reported ready")
			}
		})
	}
}

func TestFirefoxDriverRefusesWrongProxyVersionOrUnsupportedEmulation(t *testing.T) {
	for _, fault := range []string{"wrong-proxy", "wrong-version", "insecure", "emulation.setTimezoneOverride", "emulation.setLocaleOverride", "browser.setDownloadBehavior", "network.addIntercept", "readback"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			f.fault = fault
			f.mutate = func(c driverCommand, r map[string]any) {
				if c.Method == "session.new" {
					capabilities := r["capabilities"].(map[string]any)
					switch fault {
					case "wrong-proxy":
						capabilities["proxy"].(map[string]any)["socksProxy"] = "127.0.0.1:1"
					case "wrong-version":
						capabilities["browserVersion"] = "146.0"
					case "insecure":
						capabilities["acceptInsecureCerts"] = true
					}
				}
				if fault == "readback" && c.Method == "script.evaluate" && c.Params["expression"] == protocolReadback {
					r["result"] = map[string]any{"type": "string", "value": strings.Replace(f.observation(), `"timeZone":"America/Los_Angeles"`, `"timeZone":"UTC"`, 1)}
				}
			}
			f.start()
			aborted := make(chan struct{})
			d := newFirefoxDriver(f.m, f.config, f.launch, func() { close(aborted) })
			if d.Configure(context.Background()) == nil {
				t.Fatal("unsupported/unverified profile accepted")
			}
			awaitSignal(t, aborted)
		})
	}
}

func TestFirefoxDriverWorkerRealmRevokesSession(t *testing.T) {
	for _, kind := range []string{"dedicated-worker", "shared-worker", "service-worker", "worklet"} {
		t.Run(kind, func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			f.start()
			aborted := make(chan struct{})
			d := newFirefoxDriver(f.m, f.config, f.launch, func() { close(aborted) })
			defer d.Close()
			if d.Configure(context.Background()) != nil {
				t.Fatal("configure")
			}
			f.event("script.realmCreated", map[string]string{"realm": "worker-realm", "type": kind}, "")
			awaitSignal(t, aborted)
			if d.Navigate(context.Background(), "https://example.invalid") == nil {
				t.Fatal("navigation after unexpected worker")
			}
		})
	}
}

func TestFirefoxDriverHTTPBoundaryCancelsAuthAndExternalRedirects(t *testing.T) {
	for _, mode := range []string{"document", "script", "external-location", "base64-location", "attachment", "worker", "auth", "foreign-context"} {
		t.Run(mode, func(t *testing.T) {
			f := newDriverFixture(t, "firefox")
			f.start()
			d := newFirefoxDriver(f.m, f.config, f.launch, nil).(*firefoxDriver)
			defer d.Close()
			if d.Configure(context.Background()) != nil {
				t.Fatal("configure")
			}
			destination, contextID := "document", "main-context"
			if mode == "script" {
				destination = "script"
			}
			if mode == "worker" {
				destination = "worker"
			}
			if mode == "foreign-context" {
				contextID = "foreign"
			}
			headers := []any{map[string]any{"name": "Content-Security-Policy", "value": map[string]string{"type": "string", "value": "script-src 'self'"}}}
			switch mode {
			case "external-location":
				headers = append(headers, map[string]any{"name": "Location", "value": map[string]string{"type": "string", "value": "custom-os:handler"}})
			case "base64-location":
				headers = append(headers, map[string]any{"name": "Location", "value": map[string]string{"type": "base64", "value": "Y3VzdG9tLW9zOmhhbmRsZXI="}})
			case "attachment":
				headers = append(headers, map[string]any{"name": "Content-Disposition", "value": map[string]string{"type": "string", "value": "attachment; filename=x.exe"}})
			}
			raw, _ := json.Marshal(map[string]any{"context": contextID, "isBlocked": true, "request": map[string]any{"request": "request", "url": "https://example.invalid/page", "destination": destination}, "response": map[string]any{"headers": headers}})
			method := "network.responseStarted"
			if mode == "auth" {
				method = "network.authRequired"
			}
			if mode == "worker" {
				method = "network.beforeRequestSent"
			}
			if err := d.request(context.Background(), protocolEvent{Method: method, Params: raw}); err != nil {
				t.Fatal(err)
			}
			if mode == "auth" {
				c := f.waitMethod(t, "network.continueWithAuth")
				if c.Params["action"] != "cancel" {
					t.Fatal("authentication not canceled")
				}
				return
			}
			if mode == "external-location" || mode == "base64-location" || mode == "attachment" || mode == "worker" || mode == "foreign-context" {
				f.waitMethod(t, "network.failRequest")
				if f.count("network.continueResponse") != 0 {
					t.Fatal("unsafe response continued")
				}
				return
			}
			c := f.waitMethod(t, "network.continueResponse")
			encoded, _ := json.Marshal(c.Params)
			if !strings.Contains(string(encoded), "script-src 'self'") || strings.Contains(string(encoded), workerCSP) != (mode == "document") {
				t.Fatal("headers replaced or wrong CSP scope")
			}
		})
	}
}
