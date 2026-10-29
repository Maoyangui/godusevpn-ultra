//go:build !windows

package main

import "errors"

func (a *App) PrivacyOpenSettings(string) error {
	return errors.New("此版本仅支持 Windows 系统设置入口")
}
