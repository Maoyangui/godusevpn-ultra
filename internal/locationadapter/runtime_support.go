package locationadapter

// RuntimeSupport describes the narrow OS gate for the optional official
// Windows GeolocationProvider path. It is intentionally separate from the
// component transaction controller: a build number can only say that the
// documented WinRT surface may exist. It cannot grant Microsoft LAF, manifest
// runFullTrust, provider ownership, or mutation atomicity.
type RuntimeSupport struct {
	Platform             string `json:"platform"`
	Build                uint32 `json:"build"`
	ProviderBuildFloor   uint32 `json:"providerBuildFloor"`
	ProviderAPIAvailable bool   `json:"providerApiAvailable"`
	RequiresLAF          bool   `json:"requiresLaf"`
	RequiresRunFullTrust bool   `json:"requiresRunFullTrust"`
	Status               string `json:"status"`
	Reason               string `json:"reason"`
}

// RuntimeSupportStatus only performs a bounded local OS-version lookup. It
// never enables a provider, requests location permission, or changes policy.
func RuntimeSupportStatus() RuntimeSupport { return runtimeSupportStatus() }

const providerBuildFloor uint32 = 23504

func providerBuildSupported(build uint32) bool { return build >= providerBuildFloor }
