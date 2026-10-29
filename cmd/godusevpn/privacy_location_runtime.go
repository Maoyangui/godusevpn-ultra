package main

import "github.com/Maoyangui/godusevpn/internal/locationadapter"

// PrivacyLocationProviderRuntime reports only the local Windows build gate
// for the optional official GeolocationProvider path. It does not request
// location, enable a provider, or claim that a coordinate can be written.
func (a *App) PrivacyLocationProviderRuntime() locationadapter.RuntimeSupport {
	return locationadapter.RuntimeSupportStatus()
}
