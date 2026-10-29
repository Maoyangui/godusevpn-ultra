package browserhost

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"
)

// GraphicsConfig controls only browser-visible WebGL metadata. It never
// renames native hardware, alters driver capabilities, or changes a WebGPU
// adapter identity. Nil retains the native behavior of older profiles.
type GraphicsConfig struct {
	Mode     string `json:"mode"`
	PresetID string `json:"presetId,omitempty"`
	Vendor   string `json:"vendor,omitempty"`
	Renderer string `json:"renderer,omitempty"`
}

var graphicsPresets = []GraphicsConfig{
	{Mode: "webgl-metadata", PresetID: "intel-uhd-620-d3d11", Vendor: "Google Inc. (Intel)", Renderer: "ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{Mode: "webgl-metadata", PresetID: "nvidia-rtx-3060-d3d11", Vendor: "Google Inc. (NVIDIA)", Renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{Mode: "webgl-metadata", PresetID: "amd-rx-6600-d3d11", Vendor: "Google Inc. (AMD)", Renderer: "ANGLE (AMD, AMD Radeon RX 6600 Direct3D11 vs_5_0 ps_5_0, D3D11)"},
}

// GraphicsPresets are paired display-string templates, not hardware emulation
// profiles. Features, limits, image output and native driver names remain real.
func GraphicsPresets() []GraphicsConfig {
	return append([]GraphicsConfig(nil), graphicsPresets...)
}

func validGraphicsString(value string, max int) bool {
	if value == "" || len(value) > max || value != strings.TrimSpace(value) {
		return false
	}
	for _, ch := range value {
		if ch < 0x20 || ch > 0x7e {
			return false
		}
	}
	return true
}

func validateGraphics(config ProfileConfig) error {
	if config.Graphics != nil && config.Graphics.Mode == "webgl-metadata" && config.EngineID != "webview2" {
		return invalid("graphics", "metadata_requires_verified_webview2_host")
	}
	return ValidateGraphicsOptions(config.Graphics, config.WebGL, config.WebGPU, config.Environment)
}

// ValidateGraphicsOptions validates display strings, not engine support. Each
// host separately enforces its engine capability and actual runtime readback.
func ValidateGraphicsOptions(graphics *GraphicsConfig, webgl, webgpu string, environment *EnvironmentConfig) error {
	if graphics == nil {
		return nil
	}
	if graphics.Mode == "native" {
		if graphics.PresetID != "" || graphics.Vendor != "" || graphics.Renderer != "" {
			return invalid("graphics", "native_mode_must_not_override_metadata")
		}
		return nil
	}
	if graphics.Mode != "webgl-metadata" {
		return invalid("graphics.mode", "unsupported_mode")
	}
	if webgl != "runtime-default" {
		return invalid("graphics", "metadata_requires_webgl_enabled")
	}
	if webgpu != "disabled" {
		return invalid("webgpu", "metadata_requires_webgpu_disabled")
	}
	if !validGraphicsString(graphics.Vendor, 128) {
		return invalid("graphics.vendor", "printable_ascii_1_to_128_required")
	}
	if !validGraphicsString(graphics.Renderer, 256) {
		return invalid("graphics.renderer", "printable_ascii_1_to_256_required")
	}
	if environment != nil && environment.Mode == "browser-emulation" && environment.OS != "windows" && graphics.PresetID != "manual" {
		return invalid("graphics.presetId", "windows_d3d_preset_requires_windows_environment")
	}
	if graphics.PresetID == "manual" {
		return nil
	}
	for _, preset := range graphicsPresets {
		if *graphics == preset {
			return nil
		}
	}
	return invalid("graphics.presetId", "unknown_or_inconsistent_preset")
}

func seededGraphics(profileID, seed string) GraphicsConfig {
	sum := sha256.Sum256([]byte("godusevpn-ultra/webgl-metadata/v1\x00" + profileID + "\x00" + seed))
	return graphicsPresets[binary.LittleEndian.Uint64(sum[:8])%uint64(len(graphicsPresets))]
}

// GraphicsEnforcementRequirements is a pre-navigation host contract, not an
// applied/verified receipt. The actual host must install fixed code using JSON
// encoded values, inspect its own runtime, and close navigation on any failure.
type GraphicsEnforcementRequirements struct {
	BeforeNavigation             bool     `json:"beforeNavigation"`
	WebGLMetadata                bool     `json:"webglMetadata"`
	WebGPUDisabled               bool     `json:"webgpuDisabled"`
	Contexts                     []string `json:"contexts"`
	WebGLAPIs                    []string `json:"webglApis,omitempty"`
	WorkersBlockedBeforeRun      bool     `json:"workersBlockedBeforeRun"`
	ServiceWorkerBlocked         bool     `json:"serviceWorkerBlocked"`
	CloseOnReadbackMismatch      bool     `json:"closeOnReadbackMismatch"`
	ChangesNativeHardware        bool     `json:"changesNativeHardware"`
	ChangesWebGPUAdapterIdentity bool     `json:"changesWebgpuAdapterIdentity"`
}

func GraphicsRequirements(config ProfileConfig) *GraphicsEnforcementRequirements {
	metadata := config.Graphics != nil && config.Graphics.Mode == "webgl-metadata"
	if !metadata && config.WebGPU != "disabled" {
		return nil
	}
	result := &GraphicsEnforcementRequirements{
		BeforeNavigation: true, WebGLMetadata: metadata, WebGPUDisabled: config.WebGPU == "disabled",
		Contexts:                []string{"main-frame", "same-origin-frame", "cross-origin-frame"},
		WorkersBlockedBeforeRun: true, ServiceWorkerBlocked: true, CloseOnReadbackMismatch: true,
	}
	if metadata {
		result.WebGLAPIs = []string{"webgl1", "webgl2"}
	}
	return result
}

type WebGLMetadataReadback struct {
	Vendor   string `json:"vendor"`
	Renderer string `json:"renderer"`
}

type GraphicsContextReadback struct {
	Context         string                 `json:"context"`
	WebGL1          *WebGLMetadataReadback `json:"webgl1,omitempty"`
	WebGL2          *WebGLMetadataReadback `json:"webgl2,omitempty"`
	WebGPUChecked   bool                   `json:"webgpuChecked"`
	WebGPUAvailable bool                   `json:"webgpuAvailable"`
}

// Readback must be collected by the trusted live host for this profile and
// runtime before navigation. This data validator does not authenticate IPC and
// must never authorize a frontend-provided "verified" boolean or old receipt.
type GraphicsReadback struct {
	ProfileID                        string                    `json:"profileId"`
	EngineID                         string                    `json:"engineId"`
	RuntimeVersion                   string                    `json:"runtimeVersion"`
	BeforeNavigation                 bool                      `json:"beforeNavigation"`
	WorkerConstructorsBlocked        bool                      `json:"workerConstructorsBlocked"`
	WorkerTargetsBlockedBeforeRun    bool                      `json:"workerTargetsBlockedBeforeRun"`
	ServiceWorkerRegistrationBlocked bool                      `json:"serviceWorkerRegistrationBlocked"`
	Contexts                         []GraphicsContextReadback `json:"contexts"`
}

func ValidateGraphicsReadback(config ProfileConfig, runtime RuntimeContext, readback GraphicsReadback) error {
	if err := ValidateProfile(config, runtime); err != nil {
		return err
	}
	required := GraphicsRequirements(config)
	if required == nil {
		return invalid("graphicsReadback", "no_graphics_override_requested")
	}
	if readback.ProfileID != config.ProfileID || readback.EngineID != config.EngineID || !versionPattern.MatchString(runtime.Engine.Version) || readback.RuntimeVersion != runtime.Engine.Version {
		return invalid("graphicsReadback", "profile_or_runtime_identity_mismatch")
	}
	if !readback.BeforeNavigation || !readback.WorkerConstructorsBlocked || !readback.WorkerTargetsBlockedBeforeRun || !readback.ServiceWorkerRegistrationBlocked {
		return invalid("graphicsReadback", "pre_navigation_or_worker_coverage_missing")
	}
	if len(readback.Contexts) != len(required.Contexts) {
		return invalid("graphicsReadback.contexts", "incomplete_context_coverage")
	}
	seen := map[string]bool{}
	for _, observed := range readback.Contexts {
		known := false
		for _, name := range required.Contexts {
			if name == observed.Context {
				known = true
			}
		}
		if !known || seen[observed.Context] {
			return invalid("graphicsReadback.contexts", "unknown_or_duplicate_context")
		}
		seen[observed.Context] = true
		if required.WebGPUDisabled && (!observed.WebGPUChecked || observed.WebGPUAvailable) {
			return invalid("graphicsReadback.webgpu", "not_independently_disabled")
		}
		if required.WebGLMetadata {
			for _, value := range []*WebGLMetadataReadback{observed.WebGL1, observed.WebGL2} {
				if value == nil || value.Vendor != config.Graphics.Vendor || value.Renderer != config.Graphics.Renderer {
					return invalid("graphicsReadback.webgl", "metadata_readback_mismatch")
				}
			}
		}
	}
	return nil
}
