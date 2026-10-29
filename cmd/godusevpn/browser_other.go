//go:build !windows

package main

import (
	"encoding/json"
	"errors"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

type browserController struct{}
type BrowserProfileDefaultsResult struct {
	Config             json.RawMessage                 `json:"config"`
	IdentityPreview    browserhost.IdentityPreview     `json:"identityPreview"`
	Regions            []browserhost.RegionConfig      `json:"regions"`
	GraphicsPresets    []browserhost.GraphicsConfig    `json:"graphicsPresets"`
	EnvironmentPresets []browserhost.EnvironmentPreset `json:"environmentPresets"`
	Status             string                          `json:"status"`
	Detail             string                          `json:"detail"`
	HostAvailable      bool                            `json:"hostAvailable"`
}
type BrowserProfilePreviewResult struct {
	Config                  json.RawMessage                                 `json:"config"`
	IdentityPreview         browserhost.IdentityPreview                     `json:"identityPreview"`
	Capabilities            browserhost.CapabilityMatrix                    `json:"capabilities"`
	Applied                 bool                                            `json:"applied"`
	CanLaunch               bool                                            `json:"canLaunch"`
	NeedsRuntimeReadback    bool                                            `json:"needsRuntimeReadback"`
	Warnings                []string                                        `json:"warnings"`
	GraphicsRequirements    *browserhost.GraphicsEnforcementRequirements    `json:"graphicsRequirements,omitempty"`
	EnvironmentRequirements *browserhost.EnvironmentEnforcementRequirements `json:"environmentRequirements,omitempty"`
	HardwareRequirements    *browserhost.HardwareEnforcementRequirements    `json:"hardwareRequirements,omitempty"`
	HostAvailable           bool                                            `json:"hostAvailable"`
	Status                  string                                          `json:"status"`
	Detail                  string                                          `json:"detail"`
}
type BrowserProfileLaunchResult struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (*App) BrowserProfileDefaults(string) (BrowserProfileDefaultsResult, error) {
	return BrowserProfileDefaultsResult{Regions: browserhost.RegionPresets(), GraphicsPresets: browserhost.GraphicsPresets(), EnvironmentPresets: browserhost.EnvironmentPresets(), Status: "unavailable", Detail: "本版独立浏览器主机只支持 Windows。"}, nil
}
func (*App) BrowserProfilePreview(string, string) (BrowserProfilePreviewResult, error) {
	return BrowserProfilePreviewResult{}, errors.New("BROWSER_UNSUPPORTED")
}
func (*App) BrowserProfileIdentity(string) (browserhost.IdentityPreview, error) {
	return browserhost.IdentityPreview{}, errors.New("BROWSER_UNSUPPORTED")
}
func (*App) BrowserProfileLaunch(string) (BrowserProfileLaunchResult, error) {
	return BrowserProfileLaunchResult{}, errors.New("BROWSER_UNSUPPORTED")
}
func (*App) BrowserProfileLaunchProxy(string, string) (BrowserProfileLaunchResult, error) {
	return BrowserProfileLaunchResult{}, errors.New("BROWSER_UNSUPPORTED")
}
func (*App) BrowserProfileClose() error { return nil }
func (*App) shutdownBrowser()           {}
