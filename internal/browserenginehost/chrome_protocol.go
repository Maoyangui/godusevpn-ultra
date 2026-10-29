package browserenginehost

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type chromeDriver struct {
	client                         *protocolClient
	config                         Config
	launch                         protocolLaunch
	mu                             sync.Mutex
	contextID, targetID, sessionID string
	assigned                       chan struct{}
	configured                     chan error
	ready                          bool
	started                        bool
	closed                         bool
	identity                       *ChromeIdentity
	native                         protocolObservation
	probeURL, probeUA              string
	probeObserved                  chan error
}

func newChromeDriver(t protocolTransport, c Config, launch protocolLaunch, abort func()) protocolDriver {
	c = copyHardwareConfig(c)
	d := &chromeDriver{config: c, launch: launch, assigned: make(chan struct{}), configured: make(chan error, 1)}
	initialized := make(chan struct{})
	d.client = newProtocol(t, abort, func(e protocolEvent) error { <-initialized; return d.event(e) })
	close(initialized)
	return d
}

func (d *chromeDriver) call(ctx context.Context, method string, params any, session string) (json.RawMessage, error) {
	return d.client.call(ctx, method, params, session)
}

func (d *chromeDriver) Configure(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			d.Close()
		}
	}()
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return ErrRefused
	}
	d.started = true
	d.mu.Unlock()
	if validateProtocolLaunch(d.config, d.launch) != nil {
		return ErrRefused
	}
	raw, err := d.call(ctx, "Browser.getVersion", map[string]any{}, "")
	if err != nil {
		return err
	}
	var version struct {
		Product   string `json:"product"`
		UserAgent string `json:"userAgent"`
	}
	if json.Unmarshal(raw, &version) != nil || version.Product != "Chrome/"+d.config.Version || !nativeChromeUAMajor(version.UserAgent, d.config.Version) {
		return ErrRefused
	}
	raw, err = d.call(ctx, "Browser.getBrowserCommandLine", map[string]any{}, "")
	if err != nil {
		return err
	}
	var command struct {
		Arguments []string `json:"arguments"`
	}
	if json.Unmarshal(raw, &command) != nil || !verifiedChromeArguments(command.Arguments, d.launch, d.config) {
		return ErrRefused
	}
	if _, err = d.call(ctx, "Target.setAutoAttach", chromeAutoAttach(), ""); err != nil {
		return err
	}
	raw, err = d.call(ctx, "Target.createBrowserContext", map[string]any{"disposeOnDetach": true, "proxyServer": ownedProxyURL(d.launch.RelayPort), "proxyBypassList": "<-loopback>"}, "")
	if err != nil {
		return err
	}
	var browserContext struct {
		ID string `json:"browserContextId"`
	}
	if json.Unmarshal(raw, &browserContext) != nil || browserContext.ID == "" {
		return ErrRefused
	}
	d.mu.Lock()
	d.contextID = browserContext.ID
	d.mu.Unlock()
	if _, err = d.call(ctx, "Browser.setDownloadBehavior", map[string]any{"behavior": "deny", "browserContextId": browserContext.ID, "eventsEnabled": true}, ""); err != nil {
		return err
	}
	for _, name := range []string{"camera", "microphone", "geolocation", "notifications", "clipboard-read"} {
		if _, err = d.call(ctx, "Browser.setPermission", map[string]any{"permission": map[string]any{"name": name}, "setting": "denied", "browserContextId": browserContext.ID}, ""); err != nil {
			return err
		}
	}
	raw, err = d.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank", "browserContextId": browserContext.ID, "newWindow": true, "width": d.config.Width, "height": d.config.Height}, "")
	if err != nil {
		return err
	}
	var target struct {
		ID string `json:"targetId"`
	}
	if json.Unmarshal(raw, &target) != nil || target.ID == "" {
		return ErrRefused
	}
	d.mu.Lock()
	d.targetID = target.ID
	d.mu.Unlock()
	close(d.assigned)
	select {
	case err = <-d.configured:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ErrRefused
	case <-d.client.done:
		return ErrRefused
	case <-time.After(10 * time.Second):
		return ErrRefused
	}
	d.mu.Lock()
	session := d.sessionID
	d.mu.Unlock()
	if err = d.ownedHTTPProbe(ctx, version.UserAgent); err != nil {
		return err
	}
	raw, err = d.call(ctx, "Runtime.evaluate", map[string]any{"expression": protocolNativeIdentity, "awaitPromise": true, "returnByValue": true}, session)
	if err != nil {
		return err
	}
	nativeJSON, err := chromeEvaluation(raw)
	if err != nil {
		return err
	}
	var native struct {
		UserAgent string          `json:"userAgent"`
		Platform  string          `json:"navigatorPlatform"`
		Touch     int             `json:"maxTouchPoints"`
		DPR       float64         `json:"devicePixelRatio"`
		Hints     json.RawMessage `json:"clientHints"`
	}
	if len(nativeJSON) > 32768 || uniqueProtocolJSON([]byte(nativeJSON)) != nil || json.Unmarshal([]byte(nativeJSON), &native) != nil || native.UserAgent != version.UserAgent || native.Platform != "Win32" || native.Touch < 0 || native.Touch > 32 || native.DPR <= 0 || native.DPR > 10 {
		return ErrRefused
	}
	plan, err := ChromeIdentityPlan(d.config, native.UserAgent, native.Hints)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.identity = &plan
	d.native = protocolObservation{MaxTouchPoints: native.Touch, DevicePixelRatio: native.DPR}
	d.mu.Unlock()
	if err = d.applyIdentity(ctx, session, plan, true); err != nil {
		return err
	}
	if err = d.ownedHTTPProbe(ctx, plan.UserAgent); err != nil {
		return err
	}
	raw, err = d.call(ctx, "Runtime.evaluate", map[string]any{"expression": protocolReadback, "awaitPromise": true, "returnByValue": true}, session)
	if err != nil {
		return err
	}
	observation, err := chromeEvaluation(raw)
	if err != nil || verifyProtocolReadback(observation, d.config) != nil || verifyChromeIdentityReadback(observation, plan, d.native) != nil {
		return ErrRefused
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return ErrRefused
	}
	d.ready = true
	d.mu.Unlock()
	return nil
}

func (d *chromeDriver) applyIdentity(ctx context.Context, session string, plan ChromeIdentity, top bool) error {
	if plan.Override {
		// Include model even when empty: the native ClientHints storage type uses
		// omitempty, whereas CDP must actively clear a previous device model.
		h := plan.ClientHints
		metadata := map[string]any{"brands": h.Brands, "fullVersionList": h.FullVersionList, "platform": h.Platform, "platformVersion": h.PlatformVersion, "architecture": h.Architecture, "bitness": h.Bitness, "mobile": h.Mobile, "model": h.Model}
		if _, err := d.call(ctx, "Emulation.setUserAgentOverride", map[string]any{"userAgent": plan.UserAgent, "acceptLanguage": d.config.Language, "platform": plan.NavigatorPlatform, "userAgentMetadata": metadata}, session); err != nil {
			return err
		}
	}
	if top {
		if _, err := d.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": d.config.Width, "height": d.config.Height, "deviceScaleFactor": plan.DeviceScaleFactor, "mobile": plan.Mobile}, session); err != nil {
			return err
		}
	}
	if plan.MaxTouchPoints >= 0 {
		params := map[string]any{"enabled": plan.MaxTouchPoints > 0}
		if plan.MaxTouchPoints > 0 {
			params["maxTouchPoints"] = plan.MaxTouchPoints
		}
		if _, err := d.call(ctx, "Emulation.setTouchEmulationEnabled", params, session); err != nil {
			return err
		}
	}
	return nil
}

func (d *chromeDriver) ownedHTTPProbe(ctx context.Context, expectedUA string) error {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ErrRefused
	}
	probeURL := "https://godusevpn.invalid/owned/" + hex.EncodeToString(nonce[:])
	observed := make(chan error, 1)
	d.mu.Lock()
	d.probeURL = probeURL
	d.probeUA = expectedUA
	d.probeObserved = observed
	session := d.sessionID
	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.probeURL = ""; d.probeUA = ""; d.probeObserved = nil; d.mu.Unlock() }()
	raw, err := d.call(ctx, "Page.navigate", map[string]any{"url": probeURL}, session)
	var result struct {
		Frame    string `json:"frameId"`
		Failure  string `json:"errorText"`
		Download bool   `json:"isDownload"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.Frame == "" || result.Failure != "" || result.Download {
		return ErrRefused
	}
	select {
	case err := <-observed:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ErrRefused
	case <-d.client.done:
		return ErrRefused
	case <-time.After(5 * time.Second):
		return ErrRefused
	}
	encoded, _ := json.Marshal(probeURL)
	// A navigate acknowledgement is not a document-commit acknowledgement.
	// Poll only fixed protocol reads until the owned document is fully loaded.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, err = d.call(ctx, "Runtime.evaluate", map[string]any{"expression": "JSON.stringify({ready:location.href===" + string(encoded) + "&&document.readyState==='complete'})", "returnByValue": true}, session)
		if err != nil {
			return err
		}
		value, err := chromeEvaluation(raw)
		var state struct {
			Ready bool `json:"ready"`
		}
		if err != nil || json.Unmarshal([]byte(value), &state) != nil {
			return ErrRefused
		}
		if state.Ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrRefused
		case <-d.client.done:
			return ErrRefused
		case <-deadline.C:
			return ErrRefused
		case <-ticker.C:
		}
	}
}

func nativeChromeUAMajor(ua, version string) bool {
	// UA reduction intentionally zeros the minor/build/patch. The authoritative
	// product version above stays exact; the native UA need only agree on major.
	major, _, _ := strings.Cut(version, ".")
	count := 0
	for _, token := range strings.Fields(ua) {
		if strings.HasPrefix(token, "Chrome/") {
			count++
			actual := strings.TrimPrefix(token, "Chrome/")
			m, _, _ := strings.Cut(actual, ".")
			if m != major || !chromeVersion.MatchString(actual) {
				return false
			}
		}
	}
	return count == 1
}

func chromeAutoAttach() map[string]any {
	// Match Chromium's documented default exclusion of non-executing browser
	// and tab containers; every page, frame and worker target stays included.
	return map[string]any{"autoAttach": true, "waitForDebuggerOnStart": true, "flatten": true, "filter": []any{map[string]any{"type": "browser", "exclude": true}, map[string]any{"type": "tab", "exclude": true}, map[string]any{}}}
}

func verifiedChromeArguments(actual []string, launch protocolLaunch, c Config) bool {
	// The OS-verified executable is the only un-compared element. Reject extra
	// arguments as well as duplicate proxy/debug switches, not just the last one.
	if len(actual) != len(launch.Arguments)+1 || !reflect.DeepEqual(actual[1:], launch.Arguments) {
		return false
	}
	required := map[string]string{"--user-data-dir": launch.ProfileDirectory, "--proxy-server": ownedProxyURL(launch.RelayPort), "--proxy-bypass-list": "<-loopback>", "--lang": c.Language, "--remote-debugging-pipe": "", "--enable-automation": "", "--no-startup-window": "", "--disable-quic": "", "--force-webrtc-ip-handling-policy": "disable_non_proxied_udp", "--no-first-run": "", "--no-default-browser-check": "", "--disable-extensions": "", "--disable-background-networking": "", "--disable-component-update": "", "--disable-sync": "", "--disable-breakpad": "", "--host-resolver-rules": "MAP * ~NOTFOUND, EXCLUDE 127.0.0.1", "--remote-debugging-io-pipes": ""}
	seen := map[string]bool{}
	for _, arg := range launch.Arguments {
		name, value, _ := strings.Cut(arg, "=")
		if seen[name] {
			return false
		}
		seen[name] = true
		want, ok := required[name]
		if !ok {
			return false
		}
		if name == "--remote-debugging-io-pipes" {
			a, b, found := strings.Cut(value, ",")
			in, inErr := strconv.ParseUint(a, 10, 64)
			out, outErr := strconv.ParseUint(b, 10, 64)
			if !found || inErr != nil || outErr != nil || in == 0 || out == 0 || in == out {
				return false
			}
		} else if value != want {
			return false
		}
	}
	for name := range required {
		if !seen[name] {
			return false
		}
	}
	return true
}

func (d *chromeDriver) attached(ctx context.Context, e protocolEvent) error {
	var p struct {
		SessionID string `json:"sessionId"`
		Waiting   bool   `json:"waitingForDebugger"`
		Info      struct {
			ID      string `json:"targetId"`
			Type    string `json:"type"`
			Context string `json:"browserContextId"`
		} `json:"targetInfo"`
	}
	if json.Unmarshal(e.Params, &p) != nil || p.SessionID == "" || p.Info.ID == "" || !p.Waiting {
		return ErrRefused
	}
	select {
	case <-d.assigned:
	case <-d.client.done:
		return ErrRefused
	case <-ctx.Done():
		return ErrRefused
	}
	d.mu.Lock()
	ownContext, mainTarget := d.contextID, d.targetID
	d.mu.Unlock()
	if p.Info.Context != ownContext || p.Info.Type != "page" && p.Info.Type != "iframe" {
		return ErrRefused
	}
	if p.Info.Type == "page" && p.Info.ID != mainTarget {
		raw, err := d.call(ctx, "Target.closeTarget", map[string]any{"targetId": p.Info.ID}, "")
		var result struct {
			Success bool `json:"success"`
		}
		if err != nil || json.Unmarshal(raw, &result) != nil || !result.Success {
			return ErrRefused
		}
		return nil
	}
	commands := []struct {
		method string
		params any
	}{
		{"Target.setAutoAttach", chromeAutoAttach()},
		{"Page.enable", map[string]any{}}, {"Runtime.enable", map[string]any{}}, {"Network.enable", map[string]any{}},
		{"Network.setBypassServiceWorker", map[string]any{"bypass": true}},
		{"Emulation.setLocaleOverride", map[string]any{"locale": d.config.Language}},
		{"Emulation.setTimezoneOverride", map[string]any{"timezoneId": d.config.Timezone}},
		{"Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": protocolPreload, "runImmediately": true}},
		{"Page.setInterceptFileChooserDialog", map[string]any{"enabled": true}},
		{"Fetch.enable", map[string]any{"patterns": []any{map[string]any{"urlPattern": "*", "requestStage": "Request"}}, "handleAuthRequests": true}},
	}
	if p.Info.Type == "page" {
		commands = append(commands, struct {
			method string
			params any
		}{"Emulation.setDeviceMetricsOverride", map[string]any{"width": d.config.Width, "height": d.config.Height, "deviceScaleFactor": 0, "mobile": false}})
	}
	for _, command := range commands {
		if _, err := d.call(ctx, command.method, command.params, p.SessionID); err != nil {
			return err
		}
	}
	if d.config.Hardware != nil {
		count := d.config.Hardware.LogicalProcessors
		if _, err := d.call(ctx, "Emulation.setHardwareConcurrencyOverride", map[string]any{"hardwareConcurrency": count}, p.SessionID); err != nil {
			return err
		}
		// A successful command alone is not evidence that this renderer exposes
		// the requested native value. Keep every main/OOPIF target paused until
		// this read succeeds. Missing execution context refuses the session.
		raw, err := d.call(ctx, "Runtime.evaluate", map[string]any{"expression": protocolHardwareReadback, "returnByValue": true}, p.SessionID)
		if err != nil {
			return err
		}
		value, err := chromeEvaluation(raw)
		if err != nil || verifyHardwareConcurrency(value, count) != nil {
			return ErrRefused
		}
	}
	d.mu.Lock()
	plan := d.identity
	d.mu.Unlock()
	if plan != nil {
		if err := d.applyIdentity(ctx, p.SessionID, *plan, p.Info.Type == "page"); err != nil {
			return err
		}
	}
	if _, err := d.call(ctx, "Runtime.runIfWaitingForDebugger", map[string]any{}, p.SessionID); err != nil {
		return err
	}
	if p.Info.ID == mainTarget {
		d.mu.Lock()
		if d.sessionID != "" {
			d.mu.Unlock()
			return ErrRefused
		}
		d.sessionID = p.SessionID
		d.mu.Unlock()
		select {
		case d.configured <- nil:
		default:
			return ErrRefused
		}
	}
	return nil
}

func (d *chromeDriver) event(e protocolEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch e.Method {
	case "Target.attachedToTarget":
		return d.attached(ctx, e)
	case "Target.targetCrashed", "Inspector.detached", "Browser.downloadWillBegin", "Page.fileChooserOpened":
		return ErrRefused
	case "Target.detachedFromTarget":
		var p struct {
			Session string `json:"sessionId"`
		}
		if json.Unmarshal(e.Params, &p) != nil {
			return ErrRefused
		}
		d.mu.Lock()
		main := d.sessionID
		d.mu.Unlock()
		if main != "" && p.Session == main {
			return ErrRefused
		}
	case "Fetch.authRequired":
		var p struct {
			ID string `json:"requestId"`
		}
		if json.Unmarshal(e.Params, &p) != nil || p.ID == "" {
			return ErrRefused
		}
		_, err := d.call(ctx, "Fetch.continueWithAuth", map[string]any{"requestId": p.ID, "authChallengeResponse": map[string]any{"response": "CancelAuth"}}, e.SessionID)
		return err
	case "Fetch.requestPaused":
		return d.request(ctx, e)
	case "Page.frameRequestedNavigation":
		var p struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(e.Params, &p) != nil || !allowedNavigation(p.URL) && p.URL != "about:blank" && !strings.HasPrefix(p.URL, "data:text/html,") {
			return ErrRefused
		}
	case "Page.javascriptDialogOpening":
		_, err := d.call(ctx, "Page.handleJavaScriptDialog", map[string]any{"accept": false}, e.SessionID)
		return err
	}
	return nil
}

func (d *chromeDriver) request(ctx context.Context, e protocolEvent) error {
	var p struct {
		ID      string `json:"requestId"`
		Type    string `json:"resourceType"`
		Request struct {
			URL     string                     `json:"url"`
			Headers map[string]json.RawMessage `json:"headers"`
		} `json:"request"`
		Code    *int `json:"responseStatusCode"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"responseHeaders"`
		Failure string `json:"responseErrorReason"`
	}
	if json.Unmarshal(e.Params, &p) != nil || p.ID == "" {
		return ErrRefused
	}
	d.mu.Lock()
	probeURL, probeUA, probeObserved, mainSession := d.probeURL, d.probeUA, d.probeObserved, d.sessionID
	d.mu.Unlock()
	if probeURL != "" && p.Request.URL == probeURL {
		if p.Code != nil || p.Type != "Document" || e.SessionID != mainSession || probeObserved == nil {
			return ErrRefused
		}
		ua, count := "", 0
		for name, value := range p.Request.Headers {
			if strings.EqualFold(name, "User-Agent") {
				count++
				if json.Unmarshal(value, &ua) != nil {
					return ErrRefused
				}
			}
		}
		if count != 1 || ua != probeUA {
			select {
			case probeObserved <- ErrRefused:
			default:
			}
			return ErrRefused
		}
		html := `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>GodUseVPN Ultra</title><body></body>`
		_, err := d.call(ctx, "Fetch.fulfillRequest", map[string]any{"requestId": p.ID, "responseCode": 200, "responseHeaders": []map[string]string{{"name": "Content-Type", "value": "text/html; charset=utf-8"}, {"name": "Cache-Control", "value": "no-store"}, {"name": "Content-Security-Policy", "value": workerCSP}}, "body": base64.StdEncoding.EncodeToString([]byte(html))}, e.SessionID)
		select {
		case probeObserved <- err:
		default:
			return ErrRefused
		}
		return err
	}
	if !allowedNavigation(p.Request.URL) || p.Type == "Worker" || p.Type == "ServiceWorker" || p.Type == "SharedWorker" {
		_, err := d.call(ctx, "Fetch.failRequest", map[string]any{"requestId": p.ID, "errorReason": "BlockedByClient"}, e.SessionID)
		return err
	}
	if p.Failure != "" {
		_, err := d.call(ctx, "Fetch.continueRequest", map[string]any{"requestId": p.ID}, e.SessionID)
		return err
	}
	if p.Code == nil {
		_, err := d.call(ctx, "Fetch.continueRequest", map[string]any{"requestId": p.ID, "interceptResponse": true}, e.SessionID)
		return err
	}
	headers := make([]map[string]string, 0, len(p.Headers)+1)
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, "Location") {
			base, parseErr := url.Parse(p.Request.URL)
			next, nextErr := url.Parse(h.Value)
			if parseErr != nil || nextErr != nil || !allowedNavigation(base.ResolveReference(next).String()) {
				_, err := d.call(ctx, "Fetch.failRequest", map[string]any{"requestId": p.ID, "errorReason": "BlockedByClient"}, e.SessionID)
				return err
			}
		}
		if strings.EqualFold(h.Name, "Content-Disposition") && strings.Contains(strings.ToLower(h.Value), "attachment") {
			_, err := d.call(ctx, "Fetch.failRequest", map[string]any{"requestId": p.ID, "errorReason": "BlockedByClient"}, e.SessionID)
			return err
		}
		headers = append(headers, map[string]string{"name": h.Name, "value": h.Value})
	}
	if p.Type == "Document" {
		headers = append(headers, map[string]string{"name": "Content-Security-Policy", "value": workerCSP})
	}
	_, err := d.call(ctx, "Fetch.continueResponse", map[string]any{"requestId": p.ID, "responseCode": *p.Code, "responseHeaders": headers}, e.SessionID)
	return err
}

func (d *chromeDriver) Navigate(ctx context.Context, raw string) error {
	d.mu.Lock()
	ready, session := d.ready, d.sessionID
	d.mu.Unlock()
	if !ready || !allowedNavigation(raw) {
		return ErrRefused
	}
	result, err := d.call(ctx, "Page.navigate", map[string]any{"url": raw}, session)
	var value struct {
		Frame    string `json:"frameId"`
		Failure  string `json:"errorText"`
		Download bool   `json:"isDownload"`
	}
	if err != nil || json.Unmarshal(result, &value) != nil || value.Frame == "" || value.Failure != "" || value.Download {
		d.Close()
		return ErrRefused
	}
	return nil
}

func (d *chromeDriver) Close() {
	d.mu.Lock()
	d.ready = false
	d.closed = true
	d.mu.Unlock()
	d.client.Close()
}
