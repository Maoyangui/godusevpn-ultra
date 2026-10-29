package browserenginehost

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type firefoxDriver struct {
	client                  *protocolClient
	config                  Config
	launch                  protocolLaunch
	mu                      sync.Mutex
	userContext, contextID  string
	assigned                chan struct{}
	contexts                map[string]bool
	initial                 map[string]bool
	started, ready, closed  bool
	probeURL, probeUA       string
	probeObserved           chan error
	graphicsURL, graphicsUA string
	graphicsObserved        chan error
	graphicsEvidence        *GraphicsEvidence
}

func newFirefoxDriver(t protocolTransport, c Config, launch protocolLaunch, abort func()) protocolDriver {
	c = copyHardwareConfig(c)
	d := &firefoxDriver{config: c, launch: launch, assigned: make(chan struct{}), contexts: map[string]bool{}, initial: map[string]bool{}}
	initialized := make(chan struct{})
	d.client = newProtocol(t, abort, func(e protocolEvent) error { <-initialized; return d.event(e) })
	close(initialized)
	return d
}

func (d *firefoxDriver) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return d.client.call(ctx, method, params, "")
}

func (d *firefoxDriver) Configure(ctx context.Context) (err error) {
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
	proxy := map[string]any{"proxyType": "manual", "socksProxy": "127.0.0.1:" + strconv.Itoa(d.launch.RelayPort), "socksVersion": 5, "noProxy": []string{}}
	prompt := map[string]any{"default": "dismiss", "file": "dismiss"}
	raw, err := d.call(ctx, "session.new", map[string]any{"capabilities": map[string]any{"alwaysMatch": map[string]any{"browserName": "firefox", "browserVersion": strings.TrimSuffix(d.config.Version, "esr"), "acceptInsecureCerts": false, "proxy": proxy, "unhandledPromptBehavior": prompt}}})
	if err != nil {
		return err
	}
	if !verifyFirefoxCapabilities(raw, d.config, d.launch) {
		return ErrRefused
	}
	var capabilities struct {
		Capabilities struct {
			UA string `json:"userAgent"`
		} `json:"capabilities"`
	}
	if json.Unmarshal(raw, &capabilities) != nil || !printableUA(capabilities.Capabilities.UA, 2048) {
		return ErrRefused
	}
	// Keep the launcher blank tabs until a controlled replacement exists;
	// closing Firefox's last tab first can exit the browser.
	raw, err = d.call(ctx, "browsingContext.getTree", map[string]any{})
	if err != nil {
		return err
	}
	var tree struct {
		Contexts []struct {
			ID  string `json:"context"`
			URL string `json:"url"`
		} `json:"contexts"`
	}
	if json.Unmarshal(raw, &tree) != nil {
		return ErrRefused
	}
	for _, item := range tree.Contexts {
		if item.ID == "" || item.URL != "about:blank" {
			return ErrRefused
		}
		d.mu.Lock()
		d.initial[item.ID] = true
		d.mu.Unlock()
	}
	if _, err = d.call(ctx, "session.subscribe", map[string]any{"events": []string{"browsingContext.contextCreated", "browsingContext.contextDestroyed", "browsingContext.navigationStarted", "browsingContext.userPromptOpened", "browsingContext.downloadWillBegin", "network.beforeRequestSent", "network.responseStarted", "network.authRequired"}}); err != nil {
		return err
	}
	raw, err = d.call(ctx, "browser.createUserContext", map[string]any{"acceptInsecureCerts": false, "proxy": proxy, "unhandledPromptBehavior": prompt})
	if err != nil {
		return err
	}
	var uc struct {
		ID string `json:"userContext"`
	}
	if json.Unmarshal(raw, &uc) != nil || uc.ID == "" {
		return ErrRefused
	}
	d.mu.Lock()
	d.userContext = uc.ID
	d.mu.Unlock()
	viewport := map[string]any{"viewport": map[string]int{"width": d.config.Width, "height": d.config.Height}, "userContexts": []string{uc.ID}}
	preset, presetErr := selectedEnvironment(d.config)
	if presetErr != nil {
		return presetErr
	}
	if preset != nil {
		viewport["devicePixelRatio"] = preset.DeviceScaleFactor
	}
	commands := []struct {
		method string
		params any
	}{
		{"emulation.setLocaleOverride", map[string]any{"locale": d.config.Language, "userContexts": []string{uc.ID}}},
		{"emulation.setTimezoneOverride", map[string]any{"timezone": d.config.Timezone, "userContexts": []string{uc.ID}}},
		{"browsingContext.setViewport", viewport},
		{"browser.setDownloadBehavior", map[string]any{"downloadBehavior": map[string]string{"type": "denied"}, "userContexts": []string{uc.ID}}},
		{"script.addPreloadScript", map[string]any{"functionDeclaration": "()=>{" + protocolPreload + "}", "userContexts": []string{uc.ID}}},
		{"network.addIntercept", map[string]any{"phases": []string{"beforeRequestSent", "responseStarted", "authRequired"}}},
	}
	for _, command := range commands {
		if _, err = d.call(ctx, command.method, command.params); err != nil {
			return err
		}
	}
	raw, err = d.call(ctx, "browsingContext.create", map[string]any{"type": "tab", "userContext": uc.ID})
	if err != nil {
		return err
	}
	var created struct {
		ID string `json:"context"`
	}
	if json.Unmarshal(raw, &created) != nil || created.ID == "" {
		return ErrRefused
	}
	d.mu.Lock()
	d.contextID = created.ID
	d.contexts[created.ID] = true
	d.mu.Unlock()
	close(d.assigned)
	for _, item := range tree.Contexts {
		if _, err = d.call(ctx, "browsingContext.close", map[string]any{"context": item.ID, "promptUnload": false}); err != nil {
			return err
		}
	}
	if _, err = d.call(ctx, "session.subscribe", map[string]any{"events": []string{"script.realmCreated"}}); err != nil {
		return err
	}
	if err = d.ownedHTTPProbe(ctx, capabilities.Capabilities.UA); err != nil {
		return err
	}
	raw, err = d.call(ctx, "script.evaluate", map[string]any{"expression": protocolNativeFirefoxIdentity, "target": map[string]string{"context": created.ID}, "awaitPromise": false, "resultOwnership": "none"})
	if err != nil {
		return err
	}
	nativeJSON, err := firefoxEvaluation(raw)
	var native protocolObservation
	if err != nil || len(nativeJSON) > 8192 || uniqueProtocolJSON([]byte(nativeJSON)) != nil || json.Unmarshal([]byte(nativeJSON), &native) != nil || native.UserAgent != capabilities.Capabilities.UA || native.NavigatorPlatform == "" || native.OSCPU == "" || native.MaxTouchPoints < 0 || native.MaxTouchPoints > 32 || native.DevicePixelRatio <= 0 || native.DevicePixelRatio > 10 {
		return ErrRefused
	}
	ua, err := FirefoxIdentityPlan(d.config, native.UserAgent)
	if err != nil {
		return err
	}
	if identityMode(d.config) != "native" || preset != nil {
		if _, err = d.call(ctx, "emulation.setUserAgentOverride", map[string]any{"userAgent": ua, "userContexts": []string{uc.ID}}); err != nil {
			return err
		}
	}
	if err = d.ownedHTTPProbe(ctx, ua); err != nil {
		return err
	}
	// Preload scripts run for newly created Window realms, not worker realms.
	// Browser prefs disable service workers; fixed Window guards plus response
	// CSP reject other workers. A worker realm event is a final session revoke.
	raw, err = d.call(ctx, "script.evaluate", map[string]any{"expression": protocolReadback, "target": map[string]string{"context": created.ID}, "awaitPromise": true, "resultOwnership": "none"})
	if err != nil {
		return err
	}
	observation, err := firefoxEvaluation(raw)
	if err != nil || verifyProtocolReadback(observation, d.config) != nil || verifyFirefoxIdentityReadback(observation, ua, d.config, native) != nil {
		return ErrRefused
	}
	if graphicsRequested(d.config) {
		if err = d.configureGraphics(ctx, ua); err != nil {
			return err
		}
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

func (d *firefoxDriver) ownedHTTPProbe(ctx context.Context, expectedUA string) error {
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
	id := d.contextID
	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.probeURL = ""; d.probeUA = ""; d.probeObserved = nil; d.mu.Unlock() }()
	raw, err := d.call(ctx, "browsingContext.navigate", map[string]any{"context": id, "url": probeURL, "wait": "complete"})
	var result struct {
		URL string `json:"url"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.URL != probeURL {
		return ErrRefused
	}
	select {
	case err := <-observed:
		return err
	case <-ctx.Done():
		return ErrRefused
	case <-d.client.done:
		return ErrRefused
	case <-time.After(5 * time.Second):
		return ErrRefused
	}
}

func verifyFirefoxCapabilities(raw json.RawMessage, c Config, launch protocolLaunch) bool {
	var value struct {
		ID           string `json:"sessionId"`
		Capabilities struct {
			Name     string `json:"browserName"`
			Version  string `json:"browserVersion"`
			Insecure bool   `json:"acceptInsecureCerts"`
			Proxy    struct {
				Type    string   `json:"proxyType"`
				SOCKS   string   `json:"socksProxy"`
				Version int      `json:"socksVersion"`
				Bypass  []string `json:"noProxy"`
				PAC     string   `json:"proxyAutoconfigUrl"`
				HTTP    string   `json:"httpProxy"`
				SSL     string   `json:"sslProxy"`
			} `json:"proxy"`
		} `json:"capabilities"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	p := value.Capabilities.Proxy
	return value.ID != "" && value.Capabilities.Name == "firefox" && value.Capabilities.Version == strings.TrimSuffix(c.Version, "esr") && !value.Capabilities.Insecure && p.Type == "manual" && p.SOCKS == "127.0.0.1:"+strconv.Itoa(launch.RelayPort) && p.Version == 5 && len(p.Bypass) == 0 && p.PAC == "" && p.HTTP == "" && p.SSL == ""
}

func (d *firefoxDriver) waitAssigned(ctx context.Context) error {
	select {
	case <-d.assigned:
		return nil
	case <-ctx.Done():
		return ErrRefused
	case <-d.client.done:
		return ErrRefused
	}
}

func (d *firefoxDriver) ownedContext(ctx context.Context, id string) bool {
	if id == "" || d.waitAssigned(ctx) != nil {
		return false
	}
	d.mu.Lock()
	root := d.contextID
	known := d.contexts[id]
	d.mu.Unlock()
	if known || id == root {
		return true
	}
	raw, err := d.call(ctx, "browsingContext.getTree", map[string]any{"root": root})
	if err != nil {
		return false
	}
	var tree struct {
		Contexts []firefoxContext `json:"contexts"`
	}
	if json.Unmarshal(raw, &tree) != nil || len(tree.Contexts) != 1 || tree.Contexts[0].ID != root {
		return false
	}
	var contains func(firefoxContext) bool
	contains = func(n firefoxContext) bool {
		if n.ID == id {
			return true
		}
		for _, child := range n.Children {
			if contains(child) {
				return true
			}
		}
		return false
	}
	if !contains(tree.Contexts[0]) {
		return false
	}
	d.mu.Lock()
	if len(d.contexts) >= 1024 {
		d.mu.Unlock()
		return false
	}
	d.contexts[id] = true
	d.mu.Unlock()
	return true
}

type firefoxContext struct {
	ID       string           `json:"context"`
	Children []firefoxContext `json:"children"`
}

func (d *firefoxDriver) event(e protocolEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	switch e.Method {
	case "browsingContext.contextCreated":
		var p struct {
			ID     string  `json:"context"`
			Parent *string `json:"parent"`
			User   string  `json:"userContext"`
		}
		if json.Unmarshal(e.Params, &p) != nil || p.ID == "" {
			return ErrRefused
		}
		d.mu.Lock()
		initial := d.initial[p.ID]
		d.mu.Unlock()
		if initial {
			return nil
		}
		if d.waitAssigned(ctx) != nil {
			return ErrRefused
		}
		d.mu.Lock()
		root, user := d.contextID, d.userContext
		d.mu.Unlock()
		if p.User != user {
			return ErrRefused
		}
		if p.ID == root {
			return nil
		}
		if p.Parent != nil && d.ownedContext(ctx, *p.Parent) {
			d.mu.Lock()
			if len(d.contexts) >= 1024 {
				d.mu.Unlock()
				return ErrRefused
			}
			d.contexts[p.ID] = true
			d.mu.Unlock()
			return nil
		}
		_, err := d.call(ctx, "browsingContext.close", map[string]any{"context": p.ID, "promptUnload": false})
		return err
	case "browsingContext.contextDestroyed":
		var p struct {
			ID string `json:"context"`
		}
		if json.Unmarshal(e.Params, &p) != nil {
			return ErrRefused
		}
		d.mu.Lock()
		root := d.contextID
		d.mu.Unlock()
		if root != "" && p.ID == root {
			return ErrRefused
		}
	case "browsingContext.downloadWillBegin":
		return ErrRefused
	case "script.realmCreated":
		var p struct {
			Type    string `json:"type"`
			Context string `json:"context"`
		}
		if json.Unmarshal(e.Params, &p) != nil || p.Type != "window" {
			return ErrRefused
		}
		d.mu.Lock()
		initial := d.initial[p.Context]
		d.mu.Unlock()
		if initial {
			return nil
		}
		if !d.ownedContext(ctx, p.Context) {
			return ErrRefused
		}
	case "browsingContext.userPromptOpened":
		var p struct {
			Context string `json:"context"`
		}
		if json.Unmarshal(e.Params, &p) != nil || !d.ownedContext(ctx, p.Context) {
			return ErrRefused
		}
		_, err := d.call(ctx, "browsingContext.handleUserPrompt", map[string]any{"context": p.Context, "accept": false})
		return err
	case "browsingContext.navigationStarted":
		var p struct {
			URL     string `json:"url"`
			Context string `json:"context"`
		}
		if json.Unmarshal(e.Params, &p) != nil || !d.ownedContext(ctx, p.Context) || !allowedNavigation(p.URL) && p.URL != "about:blank" && p.URL != "about:srcdoc" && !strings.HasPrefix(p.URL, "data:text/html,") {
			return ErrRefused
		}
	case "network.beforeRequestSent", "network.responseStarted", "network.authRequired":
		return d.request(ctx, e)
	}
	return nil
}

func (d *firefoxDriver) request(ctx context.Context, e protocolEvent) error {
	var p struct {
		Context string `json:"context"`
		Blocked bool   `json:"isBlocked"`
		Request struct {
			ID          string `json:"request"`
			URL         string `json:"url"`
			Destination string `json:"destination"`
			Headers     []struct {
				Name  string `json:"name"`
				Value struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"value"`
			} `json:"headers"`
		} `json:"request"`
		Response struct {
			Headers []struct {
				Name  string `json:"name"`
				Value struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"value"`
			} `json:"headers"`
		} `json:"response"`
	}
	if json.Unmarshal(e.Params, &p) != nil || p.Request.ID == "" {
		return ErrRefused
	}
	if !p.Blocked {
		return nil
	}
	d.mu.Lock()
	probeURL, probeUA, observed, root := d.probeURL, d.probeUA, d.probeObserved, d.contextID
	graphicsURL, graphicsUA, graphicsObserved := d.graphicsURL, d.graphicsUA, d.graphicsObserved
	d.mu.Unlock()
	if graphicsURL != "" && p.Request.URL == graphicsURL && e.Method == "network.beforeRequestSent" {
		if p.Context == root || !d.ownedContext(ctx, p.Context) || (p.Request.Destination != "document" && p.Request.Destination != "iframe") || graphicsObserved == nil {
			return ErrGraphicsReadback
		}
		ua, count := "", 0
		for _, h := range p.Request.Headers {
			if strings.EqualFold(h.Name, "User-Agent") {
				count++
				if h.Value.Type != "string" {
					return ErrGraphicsReadback
				}
				ua = h.Value.Value
			}
		}
		if count != 1 || ua != graphicsUA {
			return ErrGraphicsReadback
		}
		err := d.fulfillGraphicsFrame(ctx, p.Request.ID)
		select {
		case graphicsObserved <- err:
		default:
			return ErrGraphicsReadback
		}
		return err
	}
	if probeURL != "" && p.Request.URL == probeURL && e.Method == "network.beforeRequestSent" {
		if p.Context != root || p.Request.Destination != "document" || observed == nil {
			return ErrRefused
		}
		ua, count := "", 0
		for _, h := range p.Request.Headers {
			if strings.EqualFold(h.Name, "User-Agent") {
				count++
				if h.Value.Type != "string" {
					return ErrRefused
				}
				ua = h.Value.Value
			}
		}
		if count != 1 || ua != probeUA {
			select {
			case observed <- ErrRefused:
			default:
			}
			return ErrRefused
		}
		html := `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>GodUseVPN Ultra</title><body></body>`
		header := func(name, value string) map[string]any {
			return map[string]any{"name": name, "value": map[string]string{"type": "string", "value": value}}
		}
		_, err := d.call(ctx, "network.provideResponse", map[string]any{"request": p.Request.ID, "statusCode": 200, "headers": []any{header("Content-Type", "text/html; charset=utf-8"), header("Cache-Control", "no-store"), header("Content-Security-Policy", workerCSP)}, "body": map[string]string{"type": "base64", "value": base64.StdEncoding.EncodeToString([]byte(html))}})
		select {
		case observed <- err:
		default:
			return ErrRefused
		}
		return err
	}
	if !allowedNavigation(p.Request.URL) || strings.Contains(strings.ToLower(p.Request.Destination), "worker") || !d.ownedContext(ctx, p.Context) {
		_, err := d.call(ctx, "network.failRequest", map[string]any{"request": p.Request.ID})
		return err
	}
	if e.Method == "network.authRequired" {
		_, err := d.call(ctx, "network.continueWithAuth", map[string]any{"request": p.Request.ID, "action": "cancel"})
		return err
	}
	if e.Method == "network.beforeRequestSent" {
		_, err := d.call(ctx, "network.continueRequest", map[string]any{"request": p.Request.ID})
		return err
	}
	headers := make([]map[string]any, 0, len(p.Response.Headers)+1)
	for _, h := range p.Response.Headers {
		if h.Value.Type != "string" && h.Value.Type != "base64" {
			return ErrRefused
		}
		if strings.EqualFold(h.Name, "Location") {
			location := h.Value.Value
			if h.Value.Type == "base64" {
				decoded, err := base64.StdEncoding.DecodeString(location)
				if err != nil {
					return ErrRefused
				}
				location = string(decoded)
			}
			base, parseErr := url.Parse(p.Request.URL)
			next, nextErr := url.Parse(location)
			if parseErr != nil || nextErr != nil || !allowedNavigation(base.ResolveReference(next).String()) {
				_, err := d.call(ctx, "network.failRequest", map[string]any{"request": p.Request.ID})
				return err
			}
		}
		if strings.EqualFold(h.Name, "Content-Disposition") && (h.Value.Type != "string" || strings.Contains(strings.ToLower(h.Value.Value), "attachment")) {
			_, err := d.call(ctx, "network.failRequest", map[string]any{"request": p.Request.ID})
			return err
		}
		headers = append(headers, map[string]any{"name": h.Name, "value": map[string]string{"type": h.Value.Type, "value": h.Value.Value}})
	}
	if p.Request.Destination == "document" || p.Request.Destination == "iframe" {
		headers = append(headers, map[string]any{"name": "Content-Security-Policy", "value": map[string]string{"type": "string", "value": workerCSP}})
	}
	_, err := d.call(ctx, "network.continueResponse", map[string]any{"request": p.Request.ID, "headers": headers})
	return err
}

func (d *firefoxDriver) Navigate(ctx context.Context, raw string) error {
	d.mu.Lock()
	ready, id := d.ready, d.contextID
	d.mu.Unlock()
	if !ready || !allowedNavigation(raw) {
		return ErrRefused
	}
	result, err := d.call(ctx, "browsingContext.navigate", map[string]any{"context": id, "url": raw, "wait": "complete"})
	var response struct {
		URL string `json:"url"`
	}
	if err != nil || json.Unmarshal(result, &response) != nil || !allowedNavigation(response.URL) {
		d.Close()
		return ErrRefused
	}
	return nil
}

func (d *firefoxDriver) Close() {
	d.mu.Lock()
	d.ready = false
	d.closed = true
	d.mu.Unlock()
	d.client.Close()
}
