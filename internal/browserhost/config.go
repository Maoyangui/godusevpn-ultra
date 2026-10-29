package browserhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// These types describe a proposed private profile. They do not configure a
// runtime, create a directory, launch a process, or prove network isolation.
// A future host must independently verify every requested setting and refuse
// navigation if its binding cannot be enforced. Inventory availability alone
// is never evidence that such a host exists.
const ProfileSchemaVersion = 1

const maxConfigBytes = 32 << 10

type ProfileConfig struct {
	SchemaVersion     int                `json:"schemaVersion"`
	EngineID          string             `json:"engineId"`
	ProfileID         string             `json:"profileId"`
	Identity          ProfileIdentity    `json:"identity"`
	Region            RegionConfig       `json:"region"`
	Viewport          ViewportConfig     `json:"viewport"`
	WebRTC            string             `json:"webrtc"`
	WebGL             string             `json:"webgl"`
	WebGPU            string             `json:"webgpu"`
	Graphics          *GraphicsConfig    `json:"graphics,omitempty"`
	Environment       *EnvironmentConfig `json:"environment,omitempty"`
	Hardware          *HardwareConfig    `json:"hardware,omitempty"`
	Permissions       PermissionConfig   `json:"permissions"`
	Cookies           string             `json:"cookies"`
	Binding           NetworkBinding     `json:"binding"`
	ExitCleanup       string             `json:"exitCleanup"`
	RandomizationSeed string             `json:"randomizationSeed,omitempty"`
}

// Generated UA is host-derived; manual UA preserves a user's printable ASCII
// text. Both keep Client Hints under the trusted runtime/environment contract.
// Native and the legacy exact independently observed snapshot remain supported.
type ProfileIdentity struct {
	Mode        string       `json:"mode"`
	UserAgent   string       `json:"userAgent,omitempty"`
	ClientHints *ClientHints `json:"clientHints,omitempty"`
}

type BrandVersion struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

type ClientHints struct {
	Brands          []BrandVersion `json:"brands"`
	FullVersionList []BrandVersion `json:"fullVersionList"`
	Platform        string         `json:"platform"`
	PlatformVersion string         `json:"platformVersion"`
	Architecture    string         `json:"architecture"`
	Bitness         string         `json:"bitness"`
	Mobile          bool           `json:"mobile"`
	Model           string         `json:"model,omitempty"`
}

// RuntimeContext must come from the trusted host, not from the stored profile
// JSON. Read() currently supplies only Engine; it does not supply Identity.
// Identity must be observed again after launch/update by a future host.
type RuntimeContext struct {
	Engine   Engine
	Identity *ObservedIdentity
}

type ObservedIdentity struct {
	RuntimeVersion string
	UserAgent      string
	ClientHints    ClientHints
}

type RegionConfig struct {
	PresetID        string   `json:"presetId"`
	Language        string   `json:"language"`
	Locale          string   `json:"locale"`
	Timezone        string   `json:"timezone"`
	AcceptLanguages []string `json:"acceptLanguages"`
}

// Viewport is the proposed content window size, not a fabricated monitor or
// device-pixel-ratio claim. Screen/GPU/font/Canvas/Audio spoofing is unsupported.
type ViewportConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PermissionConfig struct {
	Camera        string `json:"camera"`
	Microphone    string `json:"microphone"`
	Geolocation   string `json:"geolocation"`
	Notifications string `json:"notifications"`
	ClipboardRead string `json:"clipboardRead"`
}

// ReferenceID is an opaque product-owned binding ID, never a path, command,
// password or subscription. ProxyURL accepts only a literal loopback endpoint.
// No mode permits direct fallback. This is an intention, not a live guarantee.
type NetworkBinding struct {
	Mode                string `json:"mode"`
	ReferenceID         string `json:"referenceId"`
	ProxyURL            string `json:"proxyUrl,omitempty"`
	AllowDirectFallback bool   `json:"allowDirectFallback"`
}

type ConfigError struct {
	Field string
	Code  string
}

// Errors intentionally omit user-controlled values.
func (e *ConfigError) Error() string { return e.Field + ": " + e.Code }

func invalid(field, code string) error { return &ConfigError{Field: field, Code: code} }

var (
	safeIDPattern          = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
	versionPattern         = regexp.MustCompile(`^[1-9][0-9]{0,3}(?:\.[0-9]{1,5}){3}$`)
	platformVersionPattern = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,5}\.[0-9]{1,5}$`)
	seedPattern            = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
)

func safeProfileID(id string) bool {
	if !safeIDPattern.MatchString(id) || strings.HasSuffix(id, "-") {
		return false
	}
	switch id {
	case "con", "prn", "aux", "nul":
		return false
	}
	return !(len(id) == 4 && (strings.HasPrefix(id, "com") || strings.HasPrefix(id, "lpt")) && id[3] >= '1' && id[3] <= '9')
}

// canonicalEngineID accepts the existing Windows inventory's "webview" alias
// without changing the report contract. Serialized profile IDs are canonical.
func canonicalEngineID(id string) string {
	if id == "webview" {
		return "webview2"
	}
	return id
}

func previewEngine(id string) bool { return id == "webview2" || id == "edge" || id == "chrome" }

var regionPresets = []RegionConfig{
	{"en-us-pacific", "en-US", "en-US", "America/Los_Angeles", []string{"en-US", "en"}},
	{"en-us-eastern", "en-US", "en-US", "America/New_York", []string{"en-US", "en"}},
	{"en-gb", "en-GB", "en-GB", "Europe/London", []string{"en-GB", "en"}},
	{"de-de", "de-DE", "de-DE", "Europe/Berlin", []string{"de-DE", "de", "en"}},
	{"ja-jp", "ja-JP", "ja-JP", "Asia/Tokyo", []string{"ja-JP", "ja", "en"}},
	{"zh-cn", "zh-CN", "zh-CN", "Asia/Shanghai", []string{"zh-CN", "zh", "en"}},
}

// RegionPresets returns independent copies of the finite supported groups.
// Selecting a region does not change Windows or infer anything from an IP.
func RegionPresets() []RegionConfig {
	result := make([]RegionConfig, len(regionPresets))
	for i, region := range regionPresets {
		result[i] = region
		result[i].AcceptLanguages = append([]string(nil), region.AcceptLanguages...)
	}
	return result
}

func sameJSON(a, b any) bool {
	aa, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(aa, bb)
}

// DefaultProfile constructs a proposal; no setting is applied. Retention is
// explicit so a future host cannot silently delete a profile on exit.
func DefaultProfile(runtime RuntimeContext, profileID string, binding NetworkBinding) (ProfileConfig, error) {
	c := ProfileConfig{
		SchemaVersion: ProfileSchemaVersion, EngineID: canonicalEngineID(runtime.Engine.ID), ProfileID: profileID,
		Identity: ProfileIdentity{Mode: "native"}, Region: RegionPresets()[0], Viewport: ViewportConfig{1280, 720},
		WebRTC: "runtime-default", WebGL: "runtime-default", WebGPU: "runtime-default",
		Permissions: PermissionConfig{"ask", "ask", "ask", "ask", "ask"}, Cookies: "runtime-default",
		Binding: binding, ExitCleanup: "retain-profile",
	}
	return c, ValidateProfile(c, runtime)
}

// ValidateProfile checks a complete proposal against trusted runtime evidence.
// It is pure: no filesystem, registry, process, browser or network operations.
// Unsupported values fail instead of being silently dropped or normalized.
func ValidateProfile(c ProfileConfig, runtime RuntimeContext) error {
	if c.SchemaVersion != ProfileSchemaVersion {
		return invalid("schemaVersion", "unsupported_version")
	}
	if !previewEngine(c.EngineID) || c.EngineID != canonicalEngineID(runtime.Engine.ID) {
		return invalid("engineId", "unsupported_or_mismatched_engine")
	}
	if !runtime.Engine.Available {
		return invalid("engineId", "runtime_unavailable")
	}
	if !safeProfileID(c.ProfileID) {
		return invalid("profileId", "unsafe_id")
	}
	if c.RandomizationSeed != "" && !seedPattern.MatchString(c.RandomizationSeed) {
		return invalid("randomizationSeed", "unsupported_seed")
	}
	if err := validateIdentity(c.Identity, runtime); err != nil {
		return err
	}
	if err := validateEnvironment(c); err != nil {
		return err
	}
	if err := validateHardware(c); err != nil {
		return err
	}
	matched := false
	for _, region := range regionPresets {
		if sameJSON(c.Region, region) {
			matched = true
			break
		}
	}
	if !matched {
		return invalid("region", "unsupported_or_inconsistent_preset")
	}
	minWidth, minHeight := 800, 600
	if environmentMobile(c) {
		minWidth, minHeight = 320, 568
	}
	if c.Viewport.Width < minWidth || c.Viewport.Width > 3840 || c.Viewport.Height < minHeight || c.Viewport.Height > 2160 {
		return invalid("viewport", "unsupported_content_size")
	}
	if c.WebRTC != "runtime-default" && c.WebRTC != "disable-non-proxied-udp" {
		return invalid("webrtc", "unsupported_mode")
	}
	if c.WebGL != "runtime-default" && c.WebGL != "disabled" {
		return invalid("webgl", "unsupported_mode")
	}
	if c.WebGPU != "runtime-default" && c.WebGPU != "disabled" {
		return invalid("webgpu", "unsupported_mode")
	}
	if err := validateGraphics(c); err != nil {
		return err
	}
	for _, permission := range []struct{ field, value string }{
		{"camera", c.Permissions.Camera}, {"microphone", c.Permissions.Microphone},
		{"geolocation", c.Permissions.Geolocation}, {"notifications", c.Permissions.Notifications},
		{"clipboardRead", c.Permissions.ClipboardRead},
	} {
		if permission.value != "ask" && permission.value != "block" {
			return invalid("permissions."+permission.field, "unsupported_global_permission")
		}
	}
	if c.Cookies != "runtime-default" && c.Cookies != "block-third-party" {
		return invalid("cookies", "unsupported_mode")
	}
	if err := validateBinding(c.Binding); err != nil {
		return err
	}
	if c.ExitCleanup != "retain-profile" && c.ExitCleanup != "delete-owned-profile" {
		return invalid("exitCleanup", "unsupported_cleanup")
	}
	return nil
}

func safeHeader(value string, max int) bool {
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

func validateIdentity(identity ProfileIdentity, runtime RuntimeContext) error {
	if identity.Mode == "native" {
		if identity.UserAgent != "" || identity.ClientHints != nil {
			return invalid("identity", "native_identity_must_not_override_headers")
		}
		return nil
	}
	if identity.Mode == "generated" || identity.Mode == "manual" {
		if canonicalEngineID(runtime.Engine.ID) != "webview2" {
			return invalid("identity.mode", "requires_verified_webview2_host")
		}
		if identity.ClientHints != nil {
			return invalid("identity.clientHints", "host_derived_only")
		}
		if identity.Mode == "generated" {
			if identity.UserAgent != "" {
				return invalid("identity.userAgent", "generated_must_be_host_derived")
			}
		} else if !validManualUserAgent(identity.UserAgent) {
			return invalid("identity.userAgent", "printable_ascii_1_to_1024_required")
		}
		return nil
	}
	if identity.Mode != "observed-exact" {
		return invalid("identity.mode", "unsupported_identity_mode")
	}
	observed := runtime.Identity
	if !versionPattern.MatchString(runtime.Engine.Version) || observed == nil || observed.RuntimeVersion != runtime.Engine.Version {
		return invalid("identity", "independent_runtime_identity_required")
	}
	if !safeHeader(observed.UserAgent, 1024) || identity.UserAgent != observed.UserAgent || identity.ClientHints == nil || !sameJSON(*identity.ClientHints, observed.ClientHints) {
		return invalid("identity", "runtime_identity_mismatch")
	}
	major := strings.Split(runtime.Engine.Version, ".")[0]
	product := "Chrome/"
	brands := []string{"Google Chrome"}
	if c := canonicalEngineID(runtime.Engine.ID); c == "edge" || c == "webview2" {
		product = "Edg/"
		brands = []string{"Microsoft Edge"}
		if c == "webview2" {
			brands = append(brands, "Microsoft Edge WebView2")
		}
	}
	productVersion := ""
	productCount := 0
	for _, token := range strings.Fields(observed.UserAgent) {
		if strings.HasPrefix(token, product) {
			productCount++
			productVersion = strings.TrimPrefix(token, product)
		}
	}
	// Chromium may reduce the UA version. Even that exact token must have
	// been independently observed; a different full version is never accepted.
	if productCount != 1 || (productVersion != runtime.Engine.Version && productVersion != major+".0.0.0") {
		return invalid("identity", "runtime_version_mismatch")
	}
	hints := observed.ClientHints
	if hints.Platform != "Windows" || hints.Mobile || len(hints.Brands) == 0 || len(hints.Brands) > 8 || len(hints.FullVersionList) == 0 || len(hints.FullVersionList) > 8 {
		return invalid("identity.clientHints", "unsupported_runtime_hints")
	}
	for _, value := range []string{hints.PlatformVersion, hints.Architecture, hints.Bitness} {
		if !safeHeader(value, 64) {
			return invalid("identity.clientHints", "invalid_header_value")
		}
	}
	if !platformVersionPattern.MatchString(hints.PlatformVersion) || (hints.Architecture != "x86" && hints.Architecture != "arm") || (hints.Bitness != "32" && hints.Bitness != "64") {
		return invalid("identity.clientHints", "unsupported_platform_hints")
	}
	if hints.Model != "" && !safeHeader(hints.Model, 128) {
		return invalid("identity.clientHints", "invalid_model_value")
	}
	for index, list := range [][]BrandVersion{hints.Brands, hints.FullVersionList} {
		seen := map[string]bool{}
		matched := false
		for _, brand := range list {
			if !safeHeader(brand.Brand, 64) || !safeHeader(brand.Version, 32) || seen[brand.Brand] {
				return invalid("identity.clientHints", "invalid_header_value")
			}
			seen[brand.Brand] = true
			for _, expected := range brands {
				if brand.Brand == expected {
					want := major
					if index == 1 {
						want = runtime.Engine.Version
					}
					if brand.Version != want {
						return invalid("identity.clientHints", "runtime_version_mismatch")
					}
					matched = true
				}
			}
		}
		if !matched {
			return invalid("identity.clientHints", "runtime_brand_required")
		}
	}
	return nil
}

func validateBinding(binding NetworkBinding) error {
	if binding.AllowDirectFallback {
		return invalid("binding.allowDirectFallback", "direct_fallback_forbidden")
	}
	if !safeProfileID(binding.ReferenceID) {
		return invalid("binding.referenceId", "unsafe_id")
	}
	if binding.Mode == "tunnel" {
		if binding.ProxyURL != "" {
			return invalid("binding.proxyUrl", "unexpected_proxy")
		}
		return nil
	}
	if binding.Mode != "loopback-proxy" {
		return invalid("binding.mode", "unsupported_binding")
	}
	u, err := url.Parse(binding.ProxyURL)
	if err != nil || u == nil || u.String() != binding.ProxyURL || u.Opaque != "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || (u.Scheme != "http" && u.Scheme != "socks5") {
		return invalid("binding.proxyUrl", "unsafe_proxy_endpoint")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || (host != "127.0.0.1" && host != "::1") || port == "" || len(port) > 5 {
		return invalid("binding.proxyUrl", "literal_loopback_with_port_required")
	}
	n := 0
	for _, ch := range port {
		if ch < '0' || ch > '9' {
			return invalid("binding.proxyUrl", "invalid_port")
		}
		n = n*10 + int(ch-'0')
	}
	if n < 1 || n > 65535 || (len(port) > 1 && port[0] == '0') {
		return invalid("binding.proxyUrl", "invalid_port")
	}
	return nil
}

// DecodeProfile accepts one bounded JSON object. Unknown fields (including
// paths, extra arguments, remote-debugging and unsupported spoofing fields),
// duplicate keys, trailing values and incomplete configurations are rejected.
func DecodeProfile(data []byte, runtime RuntimeContext) (ProfileConfig, error) {
	var config ProfileConfig
	if len(data) == 0 || len(data) > maxConfigBytes {
		return config, invalid("json", "size_limit")
	}
	if err := rejectDuplicateKeys(data); err != nil {
		return config, invalid("json", "invalid_or_duplicate_fields")
	}
	// encoding/json matches struct fields case-insensitively and turns a null
	// pointer into omission. This newly optional field deliberately permits
	// only the exact spelling and a complete non-null object.
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) == nil {
		for name, raw := range fields {
			if strings.EqualFold(name, "hardware") && (name != "hardware" || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))) {
				return config, invalid("hardware", "exact_non_null_object_required")
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ProfileConfig{}, invalid("json", "invalid_or_unknown_fields")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ProfileConfig{}, invalid("json", "trailing_content")
	}
	if err := ValidateProfile(config, runtime); err != nil {
		return ProfileConfig{}, err
	}
	return config, nil
}

func EncodeProfile(config ProfileConfig, runtime RuntimeContext) ([]byte, error) {
	if err := ValidateProfile(config, runtime); err != nil {
		return nil, err
	}
	return json.Marshal(config)
}

func rejectDuplicateKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return errors.New("depth limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[strings.ToLower(name)] {
					return errors.New("duplicate or invalid field")
				}
				seen[strings.ToLower(name)] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		} else if delim == '[' {
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		} else {
			return errors.New("unexpected delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	return walk(0)
}

type ProfilePreview struct {
	Config                  ProfileConfig                       `json:"config"`
	Capabilities            CapabilityMatrix                    `json:"capabilities"`
	Applied                 bool                                `json:"applied"`
	CanLaunch               bool                                `json:"canLaunch"`
	NeedsRuntimeReadback    bool                                `json:"needsRuntimeReadback"`
	Warnings                []string                            `json:"warnings"`
	GraphicsRequirements    *GraphicsEnforcementRequirements    `json:"graphicsRequirements,omitempty"`
	EnvironmentRequirements *EnvironmentEnforcementRequirements `json:"environmentRequirements,omitempty"`
	HardwareRequirements    *HardwareEnforcementRequirements    `json:"hardwareRequirements,omitempty"`
	IdentityPreview         IdentityPreview                     `json:"identityPreview"`
}

func PreviewProfile(config ProfileConfig, runtime RuntimeContext) (ProfilePreview, error) {
	if err := ValidateProfile(config, runtime); err != nil {
		return ProfilePreview{}, err
	}
	// Detach mutable slices/pointers so a preview is an independent snapshot.
	data, err := json.Marshal(config)
	if err != nil {
		return ProfilePreview{}, err
	}
	var snapshot ProfileConfig
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return ProfilePreview{}, err
	}
	identityPreview, err := PreviewIdentity(snapshot, runtime)
	if err != nil {
		return ProfilePreview{}, err
	}
	preview := ProfilePreview{
		Config: snapshot, Capabilities: CapabilitiesFor(runtime), NeedsRuntimeReadback: true, GraphicsRequirements: GraphicsRequirements(snapshot), EnvironmentRequirements: EnvironmentRequirements(snapshot),
		IdentityPreview:      identityPreview,
		HardwareRequirements: HardwareRequirements(snapshot),
		Warnings: []string{
			"Configuration preview only; no browser host, setting, profile directory or network binding has been created.",
			"Language/locale/timezone groups are internally consistent preferences; they do not match an IP or guarantee anonymity.",
			"A future host must verify runtime identity, permissions, proxy/tunnel binding and cleanup ownership before offering navigation.",
		},
	}
	if snapshot.Hardware != nil {
		preview.Warnings = append(preview.Warnings, "Logical processor count changes only navigator.hardwareConcurrency through an experimental engine protocol command. The host must reject unsupported commands or mismatched frame readback; physical CPU, performance and memory remain native.")
	}
	if snapshot.Environment != nil && snapshot.Environment.Mode == "browser-emulation" && snapshot.Environment.OS != "windows" && snapshot.Graphics != nil && snapshot.Graphics.Mode == "webgl-metadata" {
		preview.Warnings = append(preview.Warnings, "Non-Windows environment retains manually chosen WebGL metadata. Windows D3D preset randomization is disabled; these display strings do not prove hardware or cross-surface coherence.")
	}
	return preview, nil
}

// SeededPreview chooses from finite desktop region/viewport groups. It never
// generates a UA, Client Hints, GPU, font or hardware identity. A seed gives
// reproducibility, not anonymity; use a new owned profile ID for a new identity.
func SeededPreview(runtime RuntimeContext, profileID, seed string, binding NetworkBinding) (ProfilePreview, error) {
	if !seedPattern.MatchString(seed) {
		return ProfilePreview{}, invalid("randomizationSeed", "unsupported_seed")
	}
	config, err := DefaultProfile(runtime, profileID, binding)
	if err != nil {
		return ProfilePreview{}, err
	}
	return RandomizedProfilePreview(config, runtime, seed)
}

// RandomizedProfilePreview retains explicit host settings. Graphics metadata
// is randomized only when the user has already selected that mode; native
// profiles never silently acquire a GPU string override or WebGPU block.
func RandomizedProfilePreview(config ProfileConfig, runtime RuntimeContext, seed string) (ProfilePreview, error) {
	if !seedPattern.MatchString(seed) {
		return ProfilePreview{}, invalid("randomizationSeed", "unsupported_seed")
	}
	if err := ValidateProfile(config, runtime); err != nil {
		return ProfilePreview{}, err
	}
	sum := sha256.Sum256([]byte("godusevpn-ultra/browser-profile/v1\x00" + config.EngineID + "\x00" + config.ProfileID + "\x00" + seed))
	regions := RegionPresets()
	config.Region = regions[binary.LittleEndian.Uint64(sum[:8])%uint64(len(regions))]
	viewports := []ViewportConfig{{1280, 720}, {1366, 768}, {1440, 900}, {1536, 864}, {1920, 1080}}
	if environmentMobile(config) {
		viewports = []ViewportConfig{{390, 844}, {412, 915}, {360, 800}, {414, 896}}
	}
	config.Viewport = viewports[binary.LittleEndian.Uint64(sum[8:16])%uint64(len(viewports))]
	config.RandomizationSeed = seed
	if config.Graphics != nil && config.Graphics.Mode == "webgl-metadata" {
		if config.Environment == nil || config.Environment.Mode == "native" || config.Environment.OS == "windows" {
			graphics := seededGraphics(config.ProfileID, seed)
			config.Graphics = &graphics
		}
		config.WebGPU = "disabled"
	}
	return PreviewProfile(config, runtime)
}
