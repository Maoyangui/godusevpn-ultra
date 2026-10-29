package cli

import (
	"errors"
	"fmt"
	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"github.com/Maoyangui/godusevpn/internal/privacyhost"
)

type machineCall func(string, any, any) error

func cleanupMachine(call machineCall) (any, error) {
	var restored []privacy.ActionResult
	var fw appfirewall.Report
	re := call("RestoreAll", nil, &restored)
	fe := call("FirewallReport", nil, &fw)
	var removes []appfirewall.Result
	if fe == nil {
		for _, r := range fw.Rules {
			var v appfirewall.Result
			e := call("FirewallRemove", r.Rule.ID, &v)
			removes = append(removes, v)
			fe = errors.Join(fe, e)
		}
	}
	var after appfirewall.Report
	ve := call("FirewallReport", nil, &after)
	if ve == nil && (after.UnknownFilters > 0 || len(after.Rules) > 0) {
		ve = errors.New("仍有本产品或未知过滤器；保留恢复工具，卸载未完成")
	}
	var remaining privacy.Report
	pe := call("Report", nil, &remaining)
	if pe == nil {
		if !remaining.BackupHealthy {
			pe = errors.New("机器策略备份不可用；保留恢复工具")
		}
		for _, c := range remaining.Controls {
			if c.Managed {
				pe = errors.Join(pe, fmt.Errorf("机器策略仍待恢复: %s", c.ID))
			}
		}
	}
	return map[string]any{"restored": restored, "removed": removes}, errors.Join(re, fe, ve, pe)
}

func machineMaintenance(uninstall bool) (any, error) {
	if !privacyhost.IsElevated() {
		return nil, errors.New("机器清理/卸载需在管理员终端显式运行")
	}
	l, err := maintenance.Acquire()
	if err != nil {
		return nil, err
	}
	defer l.Close()
	var plan *uninstallPlan
	if uninstall {
		plan, err = prepareUninstall()
		if err != nil {
			return nil, err
		}
		defer plan.close()
	}
	var verify, remove func() error
	if uninstall {
		verify, remove = plan.verify, plan.uninstall
	}
	return runMachineWindow(l.Check, verify, func() (any, error) {
		return cleanupMachine(func(method string, p, out any) error { return privacyhost.ExecuteElevatedWithLease(l, method, p, out) })
	}, remove)
}

func runMachineWindow(check, verify func() error, cleanup func() (any, error), remove func() error) (any, error) {
	if err := check(); err != nil {
		return nil, err
	}
	if verify != nil {
		if err := verify(); err != nil {
			return nil, err
		}
	}
	r, err := cleanup()
	if err != nil {
		return r, err
	}
	if err = check(); err != nil {
		return r, err
	}
	if verify != nil {
		if err = verify(); err != nil {
			return r, err
		}
	}
	if remove != nil {
		if err = remove(); err != nil {
			return r, err
		}
	}
	return r, nil
}
