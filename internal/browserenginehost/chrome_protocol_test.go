package browserenginehost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

func TestChromeDriverPausedSetupPrecedesResumeAndReadback(t *testing.T) {
	f := newDriverFixture(t, "chrome-for-testing")
	f.start()
	d := newChromeDriver(f.m, f.config, f.launch, nil)
	defer d.Close()
	if err := d.Configure(context.Background()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range f.snapshot() {
		if c.Method == "Runtime.runIfWaitingForDebugger" {
			for _, method := range []string{"Page.addScriptToEvaluateOnNewDocument", "Emulation.setTimezoneOverride", "Emulation.setLocaleOverride", "Emulation.setDeviceMetricsOverride", "Fetch.enable", "Network.setBypassServiceWorker"} {
				if !seen[method] {
					t.Fatal("target resumed before", method)
				}
			}
		}
		if c.Method == "Runtime.evaluate" && !seen["Runtime.runIfWaitingForDebugger"] {
			t.Fatal("readback before configured resume")
		}
		seen[c.Method] = true
	}
	create := f.waitMethod(t, "Target.createBrowserContext")
	if create.Params["proxyServer"] != "socks5://127.0.0.1:43123" || create.Params["proxyBypassList"] != "<-loopback>" {
		t.Fatal("wrong owned proxy")
	}
	if f.count("Browser.setPermission") != 5 || f.count("Browser.setDownloadBehavior") != 1 {
		t.Fatal("missing native denial")
	}
	preload := f.waitMethod(t, "Page.addScriptToEvaluateOnNewDocument")
	if preload.Params["source"] != protocolPreload || preload.Params["runImmediately"] != true {
		t.Fatal("preload not applied to existing initial realm")
	}
}

func TestChromeDriverAppliesAllEnvironmentsAndPreservesManualUA(t *testing.T) {
	for _, preset := range browserhost.EnvironmentPresets() {
		for _, mode := range []string{"generated", "manual", "native"} {
			t.Run(preset.Config.OS+"/"+mode, func(t *testing.T) {
				f := newDriverFixture(t, "chrome-for-testing")
				env := preset.Config
				f.config.Environment = &env
				f.config.Identity = browserhost.ProfileIdentity{Mode: mode}
				if mode == "manual" {
					f.config.Identity.UserAgent = "Exact edited UA/17"
				}
				f.config.Width = preset.RecommendedViewport.Width
				f.config.Height = preset.RecommendedViewport.Height
				originalUA := f.ua
				f.start()
				d := newChromeDriver(f.m, f.config, f.launch, nil)
				defer d.Close()
				if err := d.Configure(context.Background()); err != nil {
					t.Fatal(err)
				}
				override := f.waitMethod(t, "Emulation.setUserAgentOverride")
				if override.Params["platform"] != preset.NavigatorPlatform {
					t.Fatal("platform not applied")
				}
				if mode == "manual" && override.Params["userAgent"] != f.config.Identity.UserAgent {
					t.Fatal("manual text rewritten")
				}
				if mode == "native" && override.Params["userAgent"] != originalUA {
					t.Fatal("native UA changed when an independent environment was requested")
				}
				metadata := override.Params["userAgentMetadata"].(map[string]any)
				if metadata["platform"] != preset.Platform || metadata["platformVersion"] != preset.PlatformVersion || metadata["mobile"] != preset.Mobile || metadata["model"] != preset.Model {
					t.Fatal("wrong OS metadata")
				}
				if f.count("Fetch.fulfillRequest") != 2 {
					t.Fatal("both actual HTTP UA phases not verified")
				}
				if f.count("Emulation.setTouchEmulationEnabled") != 1 {
					t.Fatal("touch not applied")
				}
				for _, command := range f.snapshot() {
					if command.Method == "Page.navigate" && !strings.HasPrefix(command.Params["url"].(string), "https://godusevpn.invalid/owned/") {
						t.Fatal("Configure used real external request")
					}
				}
			})
		}
	}
}

func TestChromeDriverIdentityOrHTTPReadbackFailureNeverReady(t *testing.T) {
	for _, fault := range []string{"Emulation.setUserAgentOverride", "Emulation.setTouchEmulationEnabled", "http-ua", "ua-readback", "ch-readback", "touch-readback", "dpr-readback"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			env := browserhost.EnvironmentPresets()[2].Config
			f.config.Environment = &env
			f.config.Identity = browserhost.ProfileIdentity{Mode: "generated"}
			f.fault = fault
			f.mutate = func(c driverCommand, r map[string]any) {
				if c.Method != "Runtime.evaluate" || c.Params["expression"] != protocolReadback {
					return
				}
				raw := f.observation()
				switch fault {
				case "ua-readback":
					raw = strings.Replace(raw, `"userAgent":`, `"userAgent":"wrong","ignoredUA":`, 1)
				case "ch-readback":
					raw = strings.Replace(raw, `"platform":"Android"`, `"platform":"Windows"`, 1)
				case "touch-readback":
					raw = strings.Replace(raw, `"maxTouchPoints":5`, `"maxTouchPoints":0`, 1)
				case "dpr-readback":
					raw = strings.Replace(raw, `"devicePixelRatio":2.625`, `"devicePixelRatio":1`, 1)
				}
				r["result"] = map[string]any{"type": "string", "value": raw}
			}
			f.start()
			d := newChromeDriver(f.m, f.config, f.launch, nil)
			defer d.Close()
			if d.Configure(context.Background()) == nil {
				t.Fatal("failed identity reported ready")
			}
		})
	}
}

func TestChromeDriverRefusesVersionArgumentsUnsupportedCommandsAndReadback(t *testing.T) {
	for _, fault := range []string{"wrong-version", "wrong-ua", "extra-argument", "unpaused", "Emulation.setTimezoneOverride", "Browser.setPermission", "missing-boundary", "viewport"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			if fault == "unpaused" {
				f.attachedWaiting = false
			}
			f.fault = fault
			f.mutate = func(c driverCommand, r map[string]any) {
				switch fault {
				case "wrong-version":
					if c.Method == "Browser.getVersion" {
						r["product"] = "Chrome/129.0.1.1"
					}
				case "wrong-ua":
					if c.Method == "Browser.getVersion" {
						r["userAgent"] = "Chrome/129.0.0.0"
					}
				case "extra-argument":
					if c.Method == "Browser.getBrowserCommandLine" {
						r["arguments"] = append(append([]string{`C:\fixture\chrome.exe`}, f.launch.Arguments...), "--no-proxy-server")
					}
				case "missing-boundary", "viewport":
					if c.Method == "Runtime.evaluate" && c.Params["expression"] == protocolReadback {
						value := f.observation()
						if fault == "missing-boundary" {
							value = strings.Replace(value, `"boundary":true`, `"boundary":false`, 1)
						} else {
							value = strings.ReplaceAll(value, `"width":1280`, `"width":800`)
						}
						r["result"] = map[string]any{"type": "string", "value": value}
					}
				}
			}
			f.start()
			aborted := make(chan struct{})
			d := newChromeDriver(f.m, f.config, f.launch, func() { close(aborted) })
			if d.Configure(context.Background()) == nil {
				t.Fatal("unsupported/inconsistent runtime reported configured")
			}
			awaitSignal(t, aborted)
		})
	}
}

func TestChromeDriverWorkersNeverResumeAndExtraPageClosesWhilePaused(t *testing.T) {
	for _, kind := range []string{"worker", "shared_worker", "service_worker", "worklet", "foreign-context", "extra-page"} {
		t.Run(kind, func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			f.start()
			aborted := make(chan struct{})
			d := newChromeDriver(f.m, f.config, f.launch, func() { close(aborted) })
			defer d.Close()
			if d.Configure(context.Background()) != nil {
				t.Fatal("configure failed")
			}
			before := f.count("Runtime.runIfWaitingForDebugger")
			targetType, contextID := kind, "owned-context"
			if kind == "foreign-context" {
				targetType = "page"
				contextID = "foreign"
			}
			if kind == "extra-page" {
				targetType = "page"
			}
			f.event("Target.attachedToTarget", map[string]any{"sessionId": "new-session", "waitingForDebugger": true, "targetInfo": map[string]any{"targetId": "new-target", "type": targetType, "browserContextId": contextID}}, "")
			if kind == "extra-page" {
				f.waitMethod(t, "Target.closeTarget")
			} else {
				awaitSignal(t, aborted)
			}
			if f.count("Runtime.runIfWaitingForDebugger") != before {
				t.Fatal("untrusted target resumed")
			}
		})
	}
}

func TestChromeDriverResponseBoundaryPreservesHeadersAndRejectsExternalRedirect(t *testing.T) {
	for _, mode := range []string{"document", "script", "external-location", "attachment"} {
		t.Run(mode, func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			f.start()
			d := newChromeDriver(f.m, f.config, f.launch, nil).(*chromeDriver)
			defer d.Close()
			if d.Configure(context.Background()) != nil {
				t.Fatal("configure")
			}
			resource := "Document"
			headers := []map[string]string{{"name": "Content-Security-Policy", "value": "script-src 'self'"}, {"name": "Set-Cookie", "value": "private=fixture"}}
			if mode == "script" {
				resource = "Script"
			}
			if mode == "external-location" {
				headers = append(headers, map[string]string{"name": "Location", "value": "custom-os:handler"})
			}
			if mode == "attachment" {
				headers = append(headers, map[string]string{"name": "Content-Disposition", "value": "attachment; filename=x.exe"})
			}
			raw, _ := json.Marshal(map[string]any{"requestId": "request", "resourceType": resource, "request": map[string]string{"url": "https://example.invalid/page"}, "responseStatusCode": 200, "responseHeaders": headers})
			if err := d.request(context.Background(), protocolEvent{Method: "Fetch.requestPaused", SessionID: "main-session", Params: raw}); err != nil {
				t.Fatal(err)
			}
			if mode == "external-location" || mode == "attachment" {
				f.waitMethod(t, "Fetch.failRequest")
				if f.count("Fetch.continueResponse") != 0 {
					t.Fatal("unsafe response continued")
				}
				return
			}
			response := f.waitMethod(t, "Fetch.continueResponse")
			encoded, _ := json.Marshal(response.Params)
			if !strings.Contains(string(encoded), "script-src 'self'") || !strings.Contains(string(encoded), "Set-Cookie") {
				t.Fatal("original response headers lost")
			}
			if strings.Contains(string(encoded), workerCSP) != (mode == "document") {
				t.Fatal("wrong CSP scope")
			}
		})
	}
}

func TestChromeDriverStrictCommandLineAndReducedNativeUA(t *testing.T) {
	f := newDriverFixture(t, "chrome-for-testing")
	actual := append([]string{`C:\fixture\chrome.exe`}, f.launch.Arguments...)
	if !verifiedChromeArguments(actual, f.launch, f.config) || !nativeChromeUAMajor("Mozilla/5.0 Chrome/130.0.0.0 Safari/537.36", f.config.Version) {
		t.Fatal("native reduced UA / owned args rejected")
	}
	for _, arg := range []string{"--no-proxy-server", "--proxy-pac-url=https://example.invalid", "--no-sandbox", "--disable-web-security", "--proxy-server=socks5://127.0.0.1:1"} {
		l := f.launch
		l.Arguments = append(append([]string(nil), l.Arguments...), arg)
		a := append([]string{"exe"}, l.Arguments...)
		if verifiedChromeArguments(a, l, f.config) {
			t.Fatal("accepted extra unsafe argument")
		}
	}
	if nativeChromeUAMajor("Chrome/130.0.0.0 Chrome/129.0.0.0", f.config.Version) {
		t.Fatal("duplicate UA product")
	}
}
