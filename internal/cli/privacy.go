package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/detection"
	"github.com/Maoyangui/godusevpn/internal/identity"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"github.com/Maoyangui/godusevpn/internal/privacyhost"
	"github.com/Maoyangui/godusevpn/internal/timeprivacy"
)

// The recovery CLI does not contact or change the tunnel service. Machine
// actions require explicit invocation and an authorised fixed-operation helper.
func privacyMain(args []string) int {
	if len(args) == 0 {
		Usage()
		return 2
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	m := privacy.New(filepath.Join(dir, identity.AppName, "privacy-user"))
	var result any
	var actionErr error
	remote := func(method string, param, out any) error {
		if privacyhost.IsElevated() {
			return privacyhost.ExecuteElevated(method, param, out)
		}
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		h, e := privacyhost.Start(ctx, filepath.Join(filepath.Dir(exe), "godusevpn.exe"))
		if e != nil {
			return e
		}
		defer h.Close()
		return h.Call(ctx, method, param, out)
	}
	find := func(id string) (bool, error) {
		r, e := m.Report()
		if e != nil {
			return false, e
		}
		for _, c := range r.Controls {
			if c.ID == id {
				return c.RequiresAdmin, nil
			}
		}
		return false, errors.New("未知隐私项目")
	}
	switch args[0] {
	case "timezone-report", "timezone-restore":
		if len(args) != 1 {
			actionErr = errors.New("时区命令不接受路径、账户或其他参数")
			break
		}
		d, e := currentTimezoneDirectory()
		if e != nil {
			actionErr = e
			break
		}
		if args[0] == "timezone-restore" {
			result, actionErr = restoreCurrentUserTimezone(timeprivacy.New(d))
		} else {
			result, actionErr = timezoneOperation(args[0], timeprivacy.New(d))
		}
	case "report":
		result, actionErr = readUserPrivacyReport(m.Report)
	case "detection":
		p, e := m.Report()
		actionErr = e
		if e == nil {
			f, fe := appfirewall.New("").Report()
			if fe != nil {
				f.Warnings = append(f.Warnings, "未取得应用规则安装证据")
			}
			result = userDetectionReport{detection.Build(detection.Input{ExecutionMode: "direct_cli", Policies: qualifyUserJournal(p), Firewall: f, Facts: detection.ReadFacts()}), currentUserJournalScope()}
		}
	case "apply", "restore":
		if len(args) != 2 {
			actionErr = errors.New("需要指定项目 ID")
			break
		}
		machine, e := find(args[1])
		if e != nil {
			actionErr = e
			break
		}
		var r privacy.ActionResult
		if machine {
			method := "Apply"
			if args[0] == "restore" {
				method = "Restore"
			}
			actionErr = remote(method, args[1], &r)
		} else if args[0] == "apply" {
			r, actionErr = m.Apply(args[1])
		} else {
			r, actionErr = m.Restore(args[1])
		}
		result = r
	case "restore-all":
		user, e := m.RestoreAll()
		var machine []privacy.ActionResult
		me := remote("RestoreAll", nil, &machine)
		result = append(user, machine...)
		actionErr = errors.Join(e, me)
	case "firewall-list":
		result, actionErr = appfirewall.New("").Report()
	case "firewall-remove":
		if len(args) != 2 {
			actionErr = errors.New("需要规则 ID")
			break
		}
		var r appfirewall.Result
		actionErr = remote("FirewallRemove", args[1], &r)
		result = r
	case "cleanup-machine", "uninstall-machine":
		if len(args) != 1 {
			actionErr = errors.New("机器维护不接受路径或其他参数")
			break
		}
		result, actionErr = machineMaintenance(args[0] == "uninstall-machine")
	default:
		actionErr = errors.New("未知隐私命令")
	}
	if result != nil {
		e := json.NewEncoder(os.Stdout).Encode(result)
		if e != nil && actionErr == nil {
			actionErr = e
		}
	}
	if actionErr != nil {
		fmt.Fprintln(os.Stderr, "未能完成全部操作:", actionErr)
		return 1
	}
	return 0
}
