package main

import (
	"errors"
	"path/filepath"

	"github.com/Maoyangui/godusevpn/internal/clipboardprivacy"
	"github.com/Maoyangui/godusevpn/internal/paths"
)

type clipboardBridge struct {
	manager       *clipboardprivacy.Manager
	save          func(clipboardprivacy.Options) error
	profileSecret func(string) (string, error)
	prefsError    bool
}

var errClipboardSecret = errors.New("CLIPBOARD_SECRET_UNAVAILABLE")

func (a *App) clipboardBridgeLocked() (*clipboardBridge, error) {
	if a.clipboardClosed || a.ctx == nil || a.ctx.Err() != nil {
		return nil, clipboardprivacy.ErrClosed
	}
	if a.clipboard == nil {
		path := filepath.Join(userConfigDir(), paths.AppName, "clipboard-privacy.json")
		options, err := clipboardprivacy.LoadOptions(path)
		a.clipboard = &clipboardBridge{
			manager: clipboardprivacy.New(clipboardprivacy.NewNativeBackend(), options), prefsError: err != nil,
			save: func(options clipboardprivacy.Options) error { return clipboardprivacy.SaveOptions(path, options) },
			profileSecret: func(id string) (string, error) {
				profiles, err := a.GetProfiles()
				if err != nil {
					return "", errClipboardSecret
				}
				for _, profile := range profiles {
					if profile.ID == id && profile.URL != "" {
						return profile.URL, nil
					}
				}
				return "", errClipboardSecret
			},
		}
	}
	return a.clipboard, nil
}

func (a *App) ClipboardPrivacyReport() clipboardprivacy.Report {
	a.clipboardMu.Lock()
	defer a.clipboardMu.Unlock()
	bridge, err := a.clipboardBridgeLocked()
	if err != nil {
		return clipboardprivacy.Report{Options: clipboardprivacy.DefaultOptions(), LastCode: "closed", Closed: true}
	}
	report := bridge.manager.Report()
	if bridge.prefsError {
		report.LastCode = "preferences_invalid_default_disabled"
	}
	return report
}

// ClipboardPrivacySave edits a separate UI preference; it never calls service
// settings, reconnects, or writes network/Windows privacy policy.
func (a *App) ClipboardPrivacySave(options clipboardprivacy.Options) (clipboardprivacy.Report, error) {
	a.clipboardMu.Lock()
	defer a.clipboardMu.Unlock()
	bridge, err := a.clipboardBridgeLocked()
	if err != nil {
		return clipboardprivacy.Report{}, err
	}
	if options.Validate() != nil {
		return bridge.manager.Report(), clipboardprivacy.ErrInvalid
	}
	if !bridge.manager.Report().Supported {
		return bridge.manager.Report(), clipboardprivacy.ErrUnsupported
	}
	if err := bridge.save(options); err != nil {
		return bridge.manager.Report(), clipboardprivacy.ErrUnavailable
	}
	bridge.prefsError = false
	return bridge.manager.Configure(options)
}

// Only an explicit profile ID is accepted from the UI. The secret is read from
// the existing authenticated service API and never returned, persisted or logged.
func (a *App) ClipboardPrivacyCopyProfile(profileID string) (clipboardprivacy.Report, error) {
	a.clipboardMu.Lock()
	bridge, err := a.clipboardBridgeLocked()
	a.clipboardMu.Unlock()
	if err != nil {
		return clipboardprivacy.Report{}, err
	}
	if !bridge.manager.Report().Supported {
		return bridge.manager.Report(), clipboardprivacy.ErrUnsupported
	}
	if profileID == "" || len(profileID) > 128 {
		return bridge.manager.Report(), errClipboardSecret
	}
	secret, err := bridge.profileSecret(profileID)
	if err != nil {
		return bridge.manager.Report(), errClipboardSecret
	}
	return bridge.manager.CopySecret(secret)
}
func (a *App) ClipboardPrivacyCancel() (clipboardprivacy.Report, error) {
	a.clipboardMu.Lock()
	bridge, err := a.clipboardBridgeLocked()
	a.clipboardMu.Unlock()
	if err != nil {
		return clipboardprivacy.Report{}, err
	}
	return bridge.manager.Cancel()
}
func (a *App) ClipboardPrivacyClearNow() (clipboardprivacy.Report, error) {
	a.clipboardMu.Lock()
	bridge, err := a.clipboardBridgeLocked()
	a.clipboardMu.Unlock()
	if err != nil {
		return clipboardprivacy.Report{}, err
	}
	return bridge.manager.ClearNow()
}
func (a *App) shutdownClipboard() {
	a.clipboardMu.Lock()
	a.clipboardClosed = true
	bridge := a.clipboard
	a.clipboardMu.Unlock()
	if bridge != nil {
		bridge.manager.Close()
	}
}
