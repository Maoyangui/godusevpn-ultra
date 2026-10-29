//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
)

var privacySettingPages = map[string]string{
	"privacy": "privacy", "location": "privacy-location", "privacy-location": "privacy-location",
	"camera": "privacy-webcam", "microphone": "privacy-microphone", "notifications": "notifications",
	"background": "privacy-backgroundapps", "diagnostics": "privacy-feedback", "advertising": "privacy-general", "privacy-general": "privacy-general",
	"activity": "privacy-activityhistory", "language": "regionlanguage", "time": "dateandtime", "dateandtime": "dateandtime",
	"wifi": "network-wifi", "network-wifi": "network-wifi", "browser": "defaultapps", "firewall": "windowsdefender", "windowsdefender": "windowsdefender",
	// Windows exposes the input-personalisation page as privacy-speechtyping;
	// privacy-speech is not a valid ms-settings URI on current Windows 10/11.
	"privacy-speech": "privacy-speechtyping", "clipboard": "clipboard",
}

func (a *App) PrivacyOpenSettings(id string) error {
	page, ok := privacySettingPages[id]
	if !ok {
		return errors.New("没有可用的固定设置入口，请查看项目说明")
	}
	return shellExec("open", "ms-settings:"+page, "", windows.SW_SHOWNORMAL)
}
