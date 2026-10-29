package main

import "github.com/Maoyangui/godusevpn/internal/regionprivacy"

type privacyRegionManager interface {
	Report() (regionprivacy.Report, error)
	Apply(string) (regionprivacy.Result, error)
	Restore() (regionprivacy.Result, error)
}

// The manager derives its current ordinary process identity itself. It never
// routes a per-user operation through the elevated privacy helper.
var newPrivacyRegionManager = func() privacyRegionManager { return regionprivacy.New() }

func (a *App) PrivacyRegionReport() (regionprivacy.Report, error) {
	return newPrivacyRegionManager().Report()
}
func (a *App) PrivacyRegionApply(code string) (regionprivacy.Result, error) {
	a.invalidatePrivacyEvidence()
	defer a.invalidatePrivacyEvidence()
	return newPrivacyRegionManager().Apply(code)
}
func (a *App) PrivacyRegionRestore() (regionprivacy.Result, error) {
	a.invalidatePrivacyEvidence()
	defer a.invalidatePrivacyEvidence()
	return newPrivacyRegionManager().Restore()
}
