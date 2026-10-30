package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

type fakeEngineSession struct {
	mu                    sync.Mutex
	status                browserenginehost.Status
	done                  chan struct{}
	once                  sync.Once
	closes                atomic.Int32
	navigations           []string
	navigateErr, closeErr error
}

func (s *fakeEngineSession) ID() string { return s.status.SessionID }
func (s *fakeEngineSession) Status() browserenginehost.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}
func (s *fakeEngineSession) Done() <-chan struct{} { return s.done }
func (s *fakeEngineSession) Close() error {
	s.closes.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	s.once.Do(func() { s.status.Ready = false; s.status.CanNavigate = false; s.status.State = "closed"; close(s.done) })
	return nil
}
func (s *fakeEngineSession) Navigate(_ context.Context, url string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.navigations = append(s.navigations, url)
	return s.navigateErr
}

type fakeEngineGuard struct{}

func (fakeEngineGuard) Install(context.Context, browserengineguard.InstallRequest) (browserengineguard.Result, error) {
	return browserengineguard.Result{}, nil
}
func (fakeEngineGuard) Report(context.Context, string) (browserengineguard.Result, error) {
	return browserengineguard.Result{}, nil
}
func (fakeEngineGuard) Remove(context.Context, string) (browserengineguard.Result, error) {
	return browserengineguard.Result{}, nil
}
func engineFixture(t *testing.T) (*browserEngineController, *fakeEngineSession, *atomic.Int32) {
	t.Helper()
	session := &fakeEngineSession{status: browserenginehost.Status{SessionID: strings.Repeat("a", 64), EngineID: browserengine.Chrome, Version: "130.0.6723.58", State: "ready", Ready: true, CanNavigate: true, Detail: "synthetic ready only"}, done: make(chan struct{})}
	starts := &atomic.Int32{}
	c := newBrowserEngineController(context.Background(), browserEngineDependencies{manager: func(browserproxy.Config) (*browserengine.Manager, error) { return nil, nil }, guard: func() (browserenginehost.Guard, error) { return fakeEngineGuard{}, nil }, start: func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		starts.Add(1)
		return session, nil
	}})
	t.Cleanup(func() {
		_ = c.close(true)
		session.mu.Lock()
		session.closeErr = nil
		session.mu.Unlock()
		_ = session.Close()
	})
	return c, session, starts
}

const engineProxyFixture = `{"type":"socks5","host":"proxy.example.invalid","port":1080,"username":"synthetic-user","password":"SYNTHETIC_UPSTREAM_PASSWORD"}`
const engineConfigFixture = `{"engineId":"chrome-for-testing","version":"130.0.6723.58","profileId":"engine-profile","language":"en-US","timezone":"America/Los_Angeles","width":1280,"height":720}`

func TestEngineControllerOnlyFixedConfigAndProfileProxyReachHost(t *testing.T) {
	c, session, starts := engineFixture(t)
	var config browserenginehost.Config
	var proxy browserproxy.Config
	c.deps.manager = func(value browserproxy.Config) (*browserengine.Manager, error) { proxy = value; return nil, nil }
	c.deps.start = func(_ context.Context, _ *browserengine.Manager, value browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
		config = value
		starts.Add(1)
		return session, nil
	}
	result, err := c.launch(engineConfigFixture, engineProxyFixture)
	if err != nil || !result.Status.Ready || result.CanLaunch || starts.Load() != 1 {
		t.Fatal("verified fake host state not represented conservatively")
	}
	if config.ProfileID != "engine-profile" || config.Language != "en-US" || config.Timezone != "America/Los_Angeles" || config.Proxy != proxy || proxy.Password != "SYNTHETIC_UPSTREAM_PASSWORD" {
		t.Fatal("explicit proxy/config not passed internally")
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), "SYNTHETIC_UPSTREAM_PASSWORD") || strings.Contains(string(data), "proxy.example.invalid") {
		t.Fatal("profile proxy leaked into result")
	}
	if _, err := c.launch(engineConfigFixture, engineProxyFixture); err == nil {
		t.Fatal("second concurrent session started")
	}
	if c.navigate("https://example.invalid/page") != nil || len(session.navigations) != 1 {
		t.Fatal("explicit navigation failed")
	}
	if c.close(false) != nil {
		t.Fatal("fake close failed")
	}
	select {
	case <-session.Done():
	case <-time.After(time.Second):
		t.Fatal("session not closed")
	}
}

func TestEngineControllerStartupURLReachesNativeHostAndRejectsUnsafeValues(t *testing.T) {
	c, session, starts := engineFixture(t)
	var observed browserenginehost.Config
	c.deps.start = func(_ context.Context, _ *browserengine.Manager, value browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
		observed = value
		starts.Add(1)
		return session, nil
	}
	valid := strings.Replace(engineConfigFixture, `"profileId":"engine-profile"`, `"profileId":"engine-profile","startupURL":"https://startup.invalid/"`, 1)
	if _, err := c.launch(valid, engineProxyFixture); err != nil {
		t.Fatal("valid startup URL refused", err)
	}
	if observed.StartupURL != "https://startup.invalid/" {
		t.Fatalf("startup URL dropped: %q", observed.StartupURL)
	}
	for _, value := range []string{
		`file:///C:/private`,
		`https://user:secret@example.invalid/`,
		"https://example.invalid/\r\nX-Test: injected",
	} {
		bad := strings.Replace(engineConfigFixture, `"profileId":"engine-profile"`, `"profileId":"engine-profile","startupURL":`+mustJSON(value), 1)
		if _, err := c.launch(bad, engineProxyFixture); err != errBrowserEngineConfig {
			t.Fatalf("unsafe startup URL accepted: %q", value)
		}
	}
}

func mustJSON(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
func TestEngineControllerRejectsArgumentsPathsIdentitiesAndUnsupportedEnvironment(t *testing.T) {
	c, _, starts := engineFixture(t)
	for _, field := range []string{`"path":"C:/arbitrary.exe"`, `"extraArguments":["--no-sandbox"]`, `"jobHandle":1`, `"mainPid":1`, `"relayPort":1`, `"sid":"S-1-5-18"`, `"root":"C:/Users/other"`, `"environment":{"os":"ios"}`, `"graphics":{"mode":"webgl-metadata"}`, `"proxy":{"type":"direct"}`, `"VERSION":"131.0.1.1"`} {
		if _, err := c.launch("{"+field+","+engineConfigFixture[1:], engineProxyFixture); err != errBrowserEngineConfig {
			t.Fatal("unsupported native-host configuration accepted")
		}
	}
	for _, replace := range [][2]string{{`"profileId":"engine-profile"`, `"profileId":"../other"`}, {`"profileId":"engine-profile"`, `"profileId":"com1"`}, {`"width":1280`, `"width":390`}, {`"language":"en-US"`, `"language":"ja-JP"`}, {`"version":"130.0.6723.58"`, `"version":"../escape"`}} {
		if _, err := c.launch(strings.Replace(engineConfigFixture, replace[0], replace[1], 1), engineProxyFixture); err != errBrowserEngineConfig {
			t.Fatal("unsafe/mixed configuration accepted")
		}
	}
	if _, err := c.launch(engineConfigFixture, `{"type":"direct"}`); err != errBrowserEngineConfig {
		t.Fatal("missing proxy fallback accepted")
	}
	if starts.Load() != 0 {
		t.Fatal("invalid input reached host")
	}
}
func TestEngineControllerIdentityAndEnvironmentRemainExact(t *testing.T) {
	for _, mode := range []string{"generated", "manual"} {
		t.Run(mode, func(t *testing.T) {
			c, session, _ := engineFixture(t)
			var observed browserenginehost.Config
			c.deps.start = func(_ context.Context, _ *browserengine.Manager, value browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
				observed = value
				return session, nil
			}
			identity := `{"mode":"generated"}`
			if mode == "manual" {
				identity = `{"mode":"manual","userAgent":"Mozilla/5.0 (manual fixture) Chrome/130.0.0.0"}`
			}
			fields := `"identity":` + identity + `,"environment":{"mode":"browser-emulation","os":"ios","devicePresetId":"iphone-14","osVersion":"17.2"},`
			data := "{" + fields + strings.Replace(strings.Replace(engineConfigFixture[1:], `"width":1280`, `"width":390`, 1), `"height":720`, `"height":844`, 1)
			if _, err := c.launch(data, engineProxyFixture); err != nil {
				t.Fatal("valid identity/environment refused")
			}
			if observed.Identity.Mode != mode || observed.Environment == nil || observed.Environment.OS != "ios" || observed.Environment.OSVersion != "17.2" || observed.Width != 390 || observed.Height != 844 {
				t.Fatal("visible fields silently changed or dropped")
			}
			if mode == "manual" && observed.Identity.UserAgent != "Mozilla/5.0 (manual fixture) Chrome/130.0.0.0" {
				t.Fatal("manual UA changed")
			}
		})
	}
	for _, field := range []string{
		`"identity":{"mode":"manual","userAgent":"one","userAgent":"two"}`,
		`"identity":{"Mode":"native"}`,
		`"identity":{"mode":null}`,
		`"identity":{"mode":"manual","userAgent":"bad\r\nHeader: value"}`,
		`"identity":{"mode":"manual","userAgent":"fixture","clientHints":{}}`,
		`"environment":{"mode":"native","mode":"browser-emulation"}`,
		`"environment":null`,
	} {
		if _, err := parseBrowserEngineRequest("{" + field + "," + engineConfigFixture[1:]); err == nil {
			t.Fatal("ambiguous identity/environment accepted")
		}
	}
}
func TestEngineControllerRefusesUnknownGuardAndRuntimeReadback(t *testing.T) {
	for _, kind := range []string{"no-guard", "manager", "not-ready", "wrong-session", "wrong-version", "partial-error"} {
		t.Run(kind, func(t *testing.T) {
			c, session, starts := engineFixture(t)
			switch kind {
			case "no-guard":
				c.deps.guard = func() (browserenginehost.Guard, error) { return nil, errors.New("SYNTHETIC_SECRET") }
			case "manager":
				c.deps.manager = func(browserproxy.Config) (*browserengine.Manager, error) { return nil, errors.New("SYNTHETIC_SECRET") }
			case "not-ready":
				session.status.CanNavigate = false
			case "wrong-session":
				session.status.SessionID = "invalid"
			case "wrong-version":
				session.status.Version = "131.0.1.1"
			case "partial-error":
				c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
					starts.Add(1)
					return session, errors.New("SYNTHETIC_SECRET")
				}
			}
			if _, err := c.launch(engineConfigFixture, engineProxyFixture); err != errBrowserEngineRefused || strings.Contains(err.Error(), "SYNTHETIC") {
				t.Fatal("unknown state accepted or private error leaked")
			}
			if starts.Load() > 0 && session.closes.Load() == 0 {
				t.Fatal("failed launch left a host unclosed")
			}
		})
	}
}
func TestEngineControllerShutdownDuringStartClosesLateSession(t *testing.T) {
	c, session, _ := engineFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		close(entered)
		<-release
		return session, nil
	}
	result := make(chan error, 1)
	go func() { _, err := c.launch(engineConfigFixture, engineProxyFixture); result <- err }()
	<-entered
	c.close(true)
	if c.status().Status.State != "cancelling" {
		t.Fatal("cancelled launch misreported")
	}
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("late host accepted after shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("late start stuck")
	}
	if session.closes.Load() == 0 {
		t.Fatal("late host escaped shutdown")
	}
}
func TestEngineControllerNeverNavigatesProtocolsOrUnknownClosingTree(t *testing.T) {
	c, session, _ := engineFixture(t)
	if _, err := c.launch(engineConfigFixture, engineProxyFixture); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"javascript:alert(1)", "file:///C:/secret", "data:text/html,x", "mailto:a@example.invalid", "https://user:secret@example.invalid/", "https://example.invalid/\n"} {
		if c.navigate(url) == nil {
			t.Fatal("external/native protocol accepted")
		}
	}
	session.navigateErr = errors.New("SYNTHETIC_SECRET")
	session.closeErr = errors.New("unknown tree still active")
	if c.navigate("https://example.invalid/") != errBrowserEngineRefused {
		t.Fatal("native navigation failure accepted")
	}
	if status := c.status(); status.Status.Ready || status.Status.CanNavigate || status.Status.State != "closing_or_unknown" {
		t.Fatal("unknown close claimed ready/closed")
	}
	if _, err := c.launch(engineConfigFixture, engineProxyFixture); err == nil {
		t.Fatal("unknown active tree was forgotten")
	}
}
