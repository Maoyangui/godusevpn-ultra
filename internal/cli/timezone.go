package cli

import (
	"context"
	"errors"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"github.com/Maoyangui/godusevpn/internal/privacyhost"
	"github.com/Maoyangui/godusevpn/internal/timeprivacy"
	"os"
	"path/filepath"
	"time"
)

func timezoneRestoreWithBootstrap(elevated bool, ready func() bool, bootstrap func() (func(), error), restore func() (any, error)) (any, error) {
	if !elevated && !ready() {
		close, e := bootstrap()
		if close != nil {
			defer close()
		}
		if e != nil {
			return nil, e
		}
	}
	return restore()
}

func restoreCurrentUserTimezone(m timezoneManager) (any, error) {
	return timezoneRestoreWithBootstrap(privacyhost.IsElevated(), func() bool { ok, _ := maintenance.Ready(); return ok }, func() (func(), error) {
		exe, e := os.Executable()
		if e != nil {
			return nil, e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		h, e := privacyhost.StartMaintenance(ctx, filepath.Join(filepath.Dir(exe), "godusevpn.exe"))
		if e != nil {
			cancel()
			return nil, e
		}
		close := func() { h.Close(); cancel() }
		var status struct {
			Ready bool   `json:"ready"`
			Scope string `json:"scope"`
		}
		if e = h.Call(ctx, "MaintenanceReady", nil, &status); e != nil {
			return close, e
		}
		if !status.Ready || status.Scope != "synchronization_only" {
			return close, errors.New("维护助手状态无法确认；未恢复时区")
		}
		return close, nil
	}, func() (any, error) { return timezoneOperation("timezone-restore", m) })
}

type timezoneManager interface {
	Report() (timeprivacy.Report, error)
	Restore() (timeprivacy.Result, error)
}
type timezoneReport struct {
	timeprivacy.Report
	BackupScope string `json:"backupScope"`
	ScopeNote   string `json:"scopeNote"`
}
type timezoneResult struct {
	timeprivacy.Result
	BackupScope string `json:"backupScope"`
}

func timezoneOperation(action string, m timezoneManager) (any, error) {
	switch action {
	case "timezone-report":
		r, e := m.Report()
		return timezoneReport{r, "current_process_user", "当前账户备份；时区为共享系统设置，浏览器可独立覆盖。机器策略清理不包含此恢复。"}, e
	case "timezone-restore":
		r, e := m.Restore()
		return timezoneResult{r, "current_process_user"}, e
	}
	return nil, errors.New("仅支持 timezone-report 或 timezone-restore")
}
