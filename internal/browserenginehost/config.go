package browserenginehost

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

var safeProfileID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
var chromeVersion = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$`)
var firefoxVersion = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(?:\.[0-9]{1,3})?(?:esr)?$`)

func validateConfig(c Config) error {
	if c.EngineID != browserengine.Chrome && c.EngineID != browserengine.Firefox || !safeProfileID.MatchString(c.ProfileID) || strings.HasSuffix(c.ProfileID, "-") || c.ExitCleanup != "" && c.ExitCleanup != "retain-profile" && c.ExitCleanup != "delete-owned-profile" || browserproxy.Validate(c.Proxy) != nil || validateIdentity(c) != nil || validateNativeHardware(c) != nil || ValidateGraphicsConfig(c) != nil {
		return ErrRefused
	}
	minimumWidth, minimumHeight := 800, 600
	if preset, err := selectedEnvironment(c); err != nil {
		return ErrRefused
	} else if preset != nil && preset.Mobile {
		minimumWidth, minimumHeight = 320, 568
	}
	if c.Width < minimumWidth || c.Width > 3840 || c.Height < minimumHeight || c.Height > 2160 {
		return ErrRefused
	}
	if c.EngineID == browserengine.Chrome && !chromeVersion.MatchString(c.Version) || c.EngineID == browserengine.Firefox && !firefoxVersion.MatchString(c.Version) {
		return ErrRefused
	}
	for _, reserved := range []string{"con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9"} {
		if c.ProfileID == reserved {
			return ErrRefused
		}
	}
	pairs := map[string]bool{"en-US/America/New_York": true, "en-US/America/Los_Angeles": true, "en-GB/Europe/London": true, "zh-CN/Asia/Shanghai": true, "ja-JP/Asia/Tokyo": true, "de-DE/Europe/Berlin": true}
	if !pairs[c.Language+"/"+c.Timezone] {
		return ErrRefused
	}
	return nil
}
func allowedNavigation(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && len(raw) <= 8192 && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.Opaque == "" && !strings.ContainsAny(raw, "\x00\r\n")
}
func arguments(c Config, profile string, relayPort, controlPort int, pipeIn, pipeOut uintptr) []string {
	if c.EngineID == browserengine.Firefox {
		return []string{"--no-remote", "--new-instance", "--profile", profile, "--remote-debugging-port", strconv.Itoa(controlPort), "--width", strconv.Itoa(c.Width), "--height", strconv.Itoa(c.Height), "about:blank"}
	}
	return []string{"--user-data-dir=" + profile, "--remote-debugging-pipe", fmt.Sprintf("--remote-debugging-io-pipes=%d,%d", pipeIn, pipeOut), "--enable-automation", "--no-startup-window", "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-background-networking", "--disable-component-update", "--disable-sync", "--disable-quic", "--disable-breakpad", "--proxy-server=socks5://127.0.0.1:" + strconv.Itoa(relayPort), "--proxy-bypass-list=<-loopback>", "--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1", "--force-webrtc-ip-handling-policy=disable_non_proxied_udp", "--lang=" + c.Language}
}
func chromePreferences(c Config) []byte {
	value := map[string]any{"intl": map[string]any{"accept_languages": c.Language}, "credentials_enable_service": false, "profile": map[string]any{"password_manager_enabled": false, "default_content_setting_values": map[string]int{"geolocation": 2, "media_stream_mic": 2, "media_stream_camera": 2, "notifications": 2, "clipboard": 2, "automatic_downloads": 2, "protocol_handlers": 2}}}
	data, _ := json.Marshal(value)
	return data
}
func firefoxPreferences(c Config, relayPort int) []byte {
	prefs := map[string]any{"network.proxy.type": 1, "network.proxy.socks": "127.0.0.1", "network.proxy.socks_port": relayPort, "network.proxy.socks_version": 5, "network.proxy.socks_remote_dns": true, "network.proxy.no_proxies_on": "", "network.proxy.failover_direct": false, "network.proxy.allow_hijacking_localhost": true, "network.trr.mode": 5, "network.dns.disablePrefetch": true, "network.prefetch-next": false, "network.http.speculative-parallel-limit": 0, "network.protocol-handler.external-default": false, "media.peerconnection.enabled": false, "permissions.default.camera": 2, "permissions.default.microphone": 2, "permissions.default.geo": 2, "permissions.default.desktop-notification": 2, "dom.serviceWorkers.enabled": false, "intl.locale.requested": c.Language, "intl.accept_languages": c.Language, "browser.startup.page": 0, "browser.startup.homepage": "about:blank", "browser.shell.checkDefaultBrowser": false, "browser.aboutwelcome.enabled": false, "browser.newtabpage.enabled": false, "signon.rememberSignons": false, "app.update.auto": false, "app.update.enabled": false, "toolkit.telemetry.enabled": false, "datareporting.healthreport.uploadEnabled": false}
	addFirefoxGraphicsPreferences(prefs, c)
	if preset, err := selectedEnvironment(c); err == nil && preset != nil {
		// These are real Gecko preferences in the newly owned profile, not
		// navigator property replacement. Startup still checks the actual native
		// values in each realm and refuses a version that ignores them.
		prefs["general.platform.override"] = preset.NavigatorPlatform
		prefs["general.oscpu.override"] = firefoxOSCPU(preset)
		prefs["dom.maxtouchpoints.testing.value"] = preset.MaxTouchPoints
		prefs["dom.w3c_touch_events.enabled"] = 0
		if preset.Mobile {
			prefs["dom.w3c_touch_events.enabled"] = 1
		}
		prefs["dom.meta-viewport.enabled"] = preset.Mobile
	}
	var out strings.Builder
	// Sorting makes the exact fixed generated file independently reviewable.
	keys := make([]string, 0, len(prefs))
	for key := range prefs {
		keys = append(keys, key)
	}
	sortNames(keys)
	for _, key := range keys {
		keyJSON, _ := json.Marshal(key)
		valueJSON, _ := json.Marshal(prefs[key])
		fmt.Fprintf(&out, "user_pref(%s, %s);\n", keyJSON, valueJSON)
	}
	return []byte(out.String())
}
func sortNames(values []string) {
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
