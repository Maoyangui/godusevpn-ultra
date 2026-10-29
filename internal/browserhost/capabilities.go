package browserhost

// ParameterCapability describes what the pure preview model accepts. "preview"
// is not runtime enforcement. No setting in this matrix is currently applied by
// an embedded browser host; registry inventory is not runtime behavior evidence.
type ParameterCapability struct {
	ID       string   `json:"id"`
	State    string   `json:"state"`
	Values   []string `json:"values,omitempty"`
	Enforced bool     `json:"enforced"`
	Detail   string   `json:"detail"`
}

type CapabilityMatrix struct {
	EngineID         string                `json:"engineId"`
	RuntimeAvailable bool                  `json:"runtimeAvailable"`
	RuntimeVersion   string                `json:"runtimeVersion,omitempty"`
	PreviewSupported bool                  `json:"previewSupported"`
	HostImplemented  bool                  `json:"hostImplemented"`
	CanLaunch        bool                  `json:"canLaunch"`
	RuntimeVerified  bool                  `json:"runtimeVerified"`
	Parameters       []ParameterCapability `json:"parameters"`
}

func CapabilitiesFor(runtime RuntimeContext) CapabilityMatrix {
	id := canonicalEngineID(runtime.Engine.ID)
	result := CapabilityMatrix{EngineID: id, RuntimeAvailable: runtime.Engine.Available}
	if versionPattern.MatchString(runtime.Engine.Version) {
		result.RuntimeVersion = runtime.Engine.Version
	}
	result.PreviewSupported = runtime.Engine.Available && previewEngine(id)
	if !result.PreviewSupported {
		result.Parameters = []ParameterCapability{{ID: "profile", State: "unsupported", Detail: "Runtime is unavailable or this engine has no supported profile model; CEF is not provided."}}
		return result
	}
	identityValues := []string{"native"}
	if id == "webview2" {
		identityValues = append(identityValues, "generated", "manual")
	}
	if runtime.Identity != nil && runtime.Identity.RuntimeVersion == runtime.Engine.Version && versionPattern.MatchString(runtime.Engine.Version) {
		candidate := ProfileIdentity{Mode: "observed-exact", UserAgent: runtime.Identity.UserAgent, ClientHints: &runtime.Identity.ClientHints}
		if validateIdentity(candidate, runtime) == nil {
			identityValues = append(identityValues, "observed-exact")
		}
	}
	result.Parameters = []ParameterCapability{
		{ID: "profileId", State: "preview", Detail: "Safe product-owned ID only; no user path. A future host must create and verify its private root."},
		{ID: "identity", State: "preview", Values: identityValues, Detail: "Native, host-generated or user-edited printable ASCII UA, plus legacy exact observed snapshots. HTTP/navigator UA must read back consistently; Client Hints remain derived from the live engine and selected environment. Inventory-only reduced UA is a plan, not an observed Chromium full version."},
		{ID: "environment", State: "host-readback-required", Values: []string{"native", "browser-emulation"}, Detail: "Paired Windows/macOS/Android/iOS browser-visible device presets with an editable numeric OS version. Actual Chromium version/brands must be read live; UA, CH, platform, DPR, mobile metrics and touch require pre-navigation frame readback and worker blocking. Does not change the OS, actual engine, fonts or physical device."},
		{ID: "hardware", State: "host-readback-required", Values: []string{"logical-processors"}, Detail: "Optional integer logical processor count 1..64, set through the official experimental Emulation.setHardwareConcurrencyOverride command and checked through navigator.hardwareConcurrency in owned frames and paused OOPIFs. Omission retains native behavior. CPU hardware, performance and RAM are unchanged; unavailable commands or failed readback must refuse navigation."},
		{ID: "region", State: "preview", Detail: "Only the finite language/locale/timezone/Accept-Language groups returned by RegionPresets; no OS changes."},
		{ID: "viewport", State: "preview", Detail: "Desktop content 800..3840 by 600..2160; explicit paired mobile emulation permits 320..3840 by 568..2160. DPR/touch emulation requires the environment host readback contract."},
		{ID: "webrtc", State: "preview", Values: []string{"runtime-default", "disable-non-proxied-udp"}, Detail: "Requested behavior only; no STUN/traffic test or guarantee of full WebRTC disablement."},
		{ID: "webgl", State: "preview", Values: []string{"runtime-default", "disabled"}, Detail: "Requested behavior only; neither GPU identity spoofing nor verified WebGL blocking."},
		{ID: "webgpu", State: "host-readback-required", Values: []string{"runtime-default", "disabled"}, Detail: "An explicit disable request requires host enforcement and independent frame/worker coverage before navigation; adapter identity spoofing is unsupported."},
		{ID: "graphics", State: "host-readback-required", Values: []string{"native", "webgl-metadata"}, Detail: "Only paired browser-visible WebGL vendor/renderer strings; WebGL1/2 and frames must match, workers are blocked, and WebGPU must be disabled. Native GPU/driver identity, features and image output remain real; no applied claim without live host readback."},
		{ID: "permissions", State: "preview", Values: []string{"ask", "block"}, Detail: "Camera, microphone, geolocation, notifications and clipboard read; blanket allow and unsupported device permissions are rejected."},
		{ID: "cookies", State: "preview", Values: []string{"runtime-default", "block-third-party"}, Detail: "Requested storage behavior only; no existing browser data is read or cleared."},
		{ID: "binding", State: "preview", Values: []string{"tunnel", "loopback-proxy"}, Detail: "Owned reference plus optional HTTP/SOCKS5 literal-loopback endpoint; direct fallback is forbidden. No live tunnel/proxy verification exists yet."},
		{ID: "exitCleanup", State: "preview", Values: []string{"retain-profile", "delete-owned-profile"}, Detail: "A future host must verify ownership and remove only its own data; this model performs no deletion."},
		{ID: "hardwareSpoofing", State: "unsupported", Detail: "Native GPU/driver names, Canvas, Audio, fonts, MAC, sensors and hardware identifiers are not synthesized. WebGL display strings are a separate limited surface."},
		{ID: "extraArguments", State: "unsupported", Detail: "Arbitrary command lines, executable paths, profile paths and remote-debugging fields are rejected."},
	}
	return result
}
