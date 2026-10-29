package main

import "github.com/Maoyangui/godusevpn/internal/centerguide"

// PrivacyModuleGuides is static local help. It does not inspect the system,
// request an administrator, read secrets, or manufacture live protection state.
func (a *App) PrivacyModuleGuides() centerguide.Catalog { return centerguide.Guides() }
