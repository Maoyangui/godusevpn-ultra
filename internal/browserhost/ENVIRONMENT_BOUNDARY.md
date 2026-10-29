# Browser-visible environment and custom proxy boundary

`ProfileConfig.environment` is optional. Missing or `{"mode":"native"}` retains
the old native profile and desktop viewport bounds. An explicit emulation uses:

```json
{"mode":"browser-emulation","os":"android","devicePresetId":"pixel-7","osVersion":"14.1"}
```

Only these device pairs are accepted:

| OS / preset | Default OS version | navigator.platform / CH platform | Architecture / bitness | DPR / touch | Recommended viewport |
| --- | --- | --- | --- | --- | --- |
| windows / windows-desktop | 10.0.0 | Win32 / Windows | x86 / 64 | 1 / 0 | 1280 × 720 |
| macos / macos-desktop | 13.0.0 | MacIntel / macOS | x86 / 64 | 1 / 0 | 1280 × 720 |
| android / pixel-7 | 13.0.0 | Linux armv8l / Android | arm / 64 | 2.625 / 5 | 412 × 915 |
| ios / iphone-14 | 16.0.0 | iPhone / iOS | arm / 64 | 3 / 5 | 390 × 844 |

The Android/iOS CH models are respectively `Pixel 7` and `iPhone`; desktops use
an empty model. OS version is editable as 1–3 numeric components, 0–999 per
component, no leading zero and positive major. It is padded to three components
for CH platformVersion. UA OS tokens use the same components (Windows uses the
normal major.minor NT token). No arbitrary UA, runtimeVersion, flags, scripts,
path, scale factor or platform/architecture fields are accepted from the UI.

The trusted live host must read the native UA and CH first. Chromium's full
version comes from the actual `Chromium` fullVersionList brand, cross-checked
against native Chrome UA and runtime major. Edge's complete version is not a
substitute. Emulated UA keeps that actual Chrome version; CH keeps native
Chromium/GREASE versions and removes the Edge brand to match. iOS is an iPhone
web-visible preset on the actual Chromium engine, not Safari, WebKit or native
iOS. Fonts, image rendering, codecs, sensors and physical hardware remain real
and may make the emulation detectable. This is not a complete anti-fingerprint
or anonymity guarantee.

Only mobile presets relax viewport bounds to 320–3840 × 568–2160. Desktop/native
remain 800–3840 × 600–2160. Recommendations are returned as data; previews do not
overwrite an existing viewport. Explicit seeded randomization retains the
chosen OS/device and chooses mobile dimensions for a mobile preset. It never
fabricates a browser version or inserts a UA without live runtime observation.

The actual host must apply UA/CH, navigator.platform, mobile metrics, DPR and
touch before navigation, verify its probe documents and main/same/cross-origin
frames, and block workers/service workers before they can observe a native
identity. Any mismatch refuses navigation. The pure readback validator is not
an IPC authentication mechanism; frontend-supplied readback must never authorize
launch. Pure previews remain `applied:false` and `canLaunch:false`. Source/fixture
checks do not replace real WebView2 and VPS validation.

`BrowserProfileLaunchProxy(configJSON, proxyJSON)` is a separate explicit action.
The Go bridge validates the proxy request, creates an app-owned loopback SOCKS5
relay with in-memory random authentication, and sends only that relay's port,
GUI PID and credentials to the authenticated C# host over direct stdin. The
upstream server/user/password do not enter the child command line, host frame,
result, profile config or logs. The frontend cannot supply binding/PID/trust or
relay credentials. A probe result cannot authorize launch.

The current custom-proxy bridge **also requires the same freshly authenticated
Ultra strict global tunnel and SCM service PID/port lease**. It does not enable
standalone browsing without the tunnel. Every service read failure, PID/port or
strict-state change, browser failure, UI close or launch failure closes the
relay and child stdin. There is no direct fallback. The host independently
checks that `relayPid` equals its authenticated GUI parent and owns the endpoint,
authenticates its upstream relay connection, and must restrict its WebView-side
relay to this session's actual WebView processes. These host behaviors need
their own implementation and real validation; the Go model alone proves none.

Official protocol fields:

- [Emulation.setUserAgentOverride](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/#method-setUserAgentOverride)
- [Emulation.setDeviceMetricsOverride](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/#method-setDeviceMetricsOverride)
- [Emulation.setTouchEmulationEnabled](https://chromedevtools.github.io/devtools-protocol/tot/Emulation/#method-setTouchEmulationEnabled)
