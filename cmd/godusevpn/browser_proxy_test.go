//go:build windows

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

	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
	"github.com/Maoyangui/godusevpn/internal/ipc"
)

type fakeBrowserRelay struct {
	port   int
	auth   browserproxy.RelayCredentials
	closed atomic.Bool
	once   sync.Once
}

func (r *fakeBrowserRelay) Port() int                                  { return r.port }
func (r *fakeBrowserRelay) Credentials() browserproxy.RelayCredentials { return r.auth }
func (r *fakeBrowserRelay) Close()                                     { r.once.Do(func() { r.closed.Store(true) }) }
func installFakeBrowserRelay(f *browserFixture) *fakeBrowserRelay {
	r := &fakeBrowserRelay{port: 38122, auth: browserproxy.RelayCredentials{Username: "ultra-browser", Password: strings.Repeat("a", 64)}}
	f.controller.deps.relay = func(context.Context, browserproxy.Config) (browserProfileRelay, error) { return r, nil }
	f.controller.deps.selfPID = func() uint32 { return 4343 }
	return r
}

const syntheticBrowserProxy = `{"type":"socks5","host":"proxy.example.invalid","port":1080,"username":"synthetic-user","password":"SYNTHETIC_UPSTREAM_PASSWORD"}`

func TestBrowserCustomProxyUsesOnlyInternalRelayAndKeepsStrictLease(t *testing.T) {
	f := newBrowserFixture(t)
	relay := installFakeBrowserRelay(f)
	var received browserproxy.Config
	f.controller.deps.relay = func(ctx context.Context, config browserproxy.Config) (browserProfileRelay, error) {
		received = config
		return relay, nil
	}
	result, err := f.controller.launchProxy(browserFixtureJSON(t, f), syntheticBrowserProxy)
	if err != nil || result.Status != "ready" {
		t.Fatal("fake proxy launch refused")
	}
	var frame struct {
		ServicePID    uint32                    `json:"servicePid"`
		ProxyPort     int                       `json:"proxyPort"`
		RelayPID      uint32                    `json:"relayPid"`
		RelayUsername string                    `json:"relayUsername"`
		RelayPassword string                    `json:"relayPassword"`
		Profile       browserhost.ProfileConfig `json:"profile"`
	}
	data := <-f.frames
	if json.Unmarshal(data, &frame) != nil {
		t.Fatal("invalid frame")
	}
	if frame.ServicePID != 4242 || frame.RelayPID != 4343 || frame.ProxyPort != 38122 || frame.Profile.Binding.ReferenceID != "profile-relay" || frame.Profile.Binding.ProxyURL != "socks5://127.0.0.1:38122" || frame.Profile.Binding.AllowDirectFallback || frame.RelayUsername != relay.auth.Username || frame.RelayPassword != relay.auth.Password {
		t.Fatal("frame used untrusted/incomplete relay identity")
	}
	if received.Host != "proxy.example.invalid" || received.Password != "SYNTHETIC_UPSTREAM_PASSWORD" || strings.Contains(string(data), "SYNTHETIC_UPSTREAM_PASSWORD") || strings.Contains(string(data), "proxy.example.invalid") {
		t.Fatal("upstream credentials escaped into host frame")
	}
	output, _ := json.Marshal(result)
	if strings.Contains(string(output), relay.auth.Password) || strings.Contains(string(output), "SYNTHETIC_UPSTREAM") || !strings.Contains(result.Detail, "严格隧道叠加") {
		t.Fatal("launch result leaked credentials or omitted stage restriction")
	}
	f.mu.Lock()
	f.view.ServicePID++
	f.mu.Unlock()
	f.ticks <- time.Now()
	waitBrowserClosed(t, f)
	if !relay.closed.Load() {
		t.Fatal("strict lease failure left custom relay open")
	}
}
func TestBrowserCustomProxyRefusesUntrustedInputBeforeRelayStart(t *testing.T) {
	f := newBrowserFixture(t)
	var calls atomic.Int32
	f.controller.deps.relay = func(context.Context, browserproxy.Config) (browserProfileRelay, error) {
		calls.Add(1)
		return nil, errors.New("SYNTHETIC_PRIVATE_ERROR")
	}
	f.controller.deps.selfPID = func() uint32 { return 4343 }
	config := browserFixtureJSON(t, f)
	for _, input := range []string{`{}`, `{"type":"direct"}`, `{"type":"socks5","host":"proxy.example.invalid","port":1080,"relayPid":1}`, strings.Replace(syntheticBrowserProxy, `"port":1080`, `"port":1080,"PORT":1081`, 1)} {
		if _, err := f.controller.launchProxy(config, input); err != errBrowserConfig {
			t.Fatal("invalid proxy reached runtime")
		}
	}
	for _, field := range []string{`"relayPid":1`, `"relayUsername":"x"`, `"relayPassword":"x"`, `"binding":{"mode":"direct"}`} {
		if _, err := f.controller.launchProxy("{"+field+","+config[1:], syntheticBrowserProxy); err == nil {
			t.Fatal("frontend relay identity injection accepted")
		}
	}
	if calls.Load() != 0 || f.starts.Load() != 0 {
		t.Fatal("invalid configuration started relay/host")
	}
	f.readErr = ipc.ErrUntrustedService
	if _, err := f.controller.launchProxy(config, syntheticBrowserProxy); err == nil || calls.Load() != 0 || f.starts.Load() != 0 {
		t.Fatal("probe/config bypassed strict service identity")
	}
}
func TestBrowserCustomProxyClosesRelayOnStartAndProtocolFailure(t *testing.T) {
	for _, kind := range []string{"start", "protocol", "invalid-auth", "invalid-port", "factory-error", "explicit-close"} {
		t.Run(kind, func(t *testing.T) {
			f := newBrowserFixture(t)
			relay := installFakeBrowserRelay(f)
			switch kind {
			case "start":
				f.controller.deps.start = func(context.Context, string) (*browserHostProcess, error) {
					return nil, errors.New("SYNTHETIC_PRIVATE_ERROR")
				}
			case "protocol":
				f.response = `{"type":"error","code":"configuration_refused"}`
			case "invalid-auth":
				relay.auth.Password = "weak"
			case "invalid-port":
				relay.port = 0
			case "factory-error":
				f.controller.deps.relay = func(context.Context, browserproxy.Config) (browserProfileRelay, error) {
					return relay, errors.New("SYNTHETIC_PRIVATE_ERROR")
				}
			}
			_, err := f.controller.launchProxy(browserFixtureJSON(t, f), syntheticBrowserProxy)
			if kind == "explicit-close" {
				if err != nil {
					t.Fatal(err)
				}
				f.controller.close(false)
			} else if err == nil || strings.Contains(err.Error(), "SYNTHETIC_PRIVATE") {
				t.Fatal("failure accepted or raw error exposed")
			}
			if !relay.closed.Load() {
				t.Fatal("failed/closed run retained relay")
			}
		})
	}
}
func TestBrowserCustomProxyStopDuringRelayCreationClosesLateRelay(t *testing.T) {
	f := newBrowserFixture(t)
	relay := installFakeBrowserRelay(f)
	entered := make(chan struct{})
	release := make(chan struct{})
	f.controller.deps.relay = func(context.Context, browserproxy.Config) (browserProfileRelay, error) {
		close(entered)
		<-release
		return relay, nil
	}
	data := browserFixtureJSON(t, f)
	result := make(chan error, 1)
	go func() { _, err := f.controller.launchProxy(data, syntheticBrowserProxy); result <- err }()
	<-entered
	f.controller.close(true)
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("launch after shutdown")
		}
	case <-time.After(time.Second):
		t.Fatal("launch stuck after relay completion")
	}
	if !relay.closed.Load() || f.starts.Load() != 0 {
		t.Fatal("late relay escaped shutdown")
	}
}
func TestBrowserDefaultsExposeEnvironmentPresetsWithoutOverwritingViewport(t *testing.T) {
	f := newBrowserFixture(t)
	d, err := f.controller.defaults("fixture-profile")
	if err != nil || len(d.EnvironmentPresets) != 4 {
		t.Fatal("environment templates unavailable")
	}
	var config map[string]any
	json.Unmarshal(d.Config, &config)
	config["environment"] = map[string]any{"mode": "browser-emulation", "os": "android", "devicePresetId": "pixel-7", "osVersion": "14.1"}
	config["viewport"] = map[string]any{"width": 360, "height": 780}
	data, _ := json.Marshal(config)
	preview, err := f.controller.preview(string(data), "")
	if err != nil || preview.EnvironmentRequirements == nil || preview.Applied || (!browserenginehost.NativeLaunchEnabled() && preview.CanLaunch) {
		t.Fatal("environment preview missing requirements")
	}
	var result browserhost.ProfileConfig
	json.Unmarshal(preview.Config, &result)
	if result.Viewport != (browserhost.ViewportConfig{Width: 360, Height: 780}) || result.Environment.OSVersion != "14.1" {
		t.Fatal("preview clobbered user environment draft")
	}
}
