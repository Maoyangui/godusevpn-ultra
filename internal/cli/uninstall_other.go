//go:build !windows

package cli

import "errors"

type uninstallPlan struct{}

func prepareUninstall() (*uninstallPlan, error) {
	return nil, errors.New("仅Windows支持此固定安装卸载入口")
}
func (*uninstallPlan) close()           {}
func (*uninstallPlan) verify() error    { return errors.New("unsupported") }
func (*uninstallPlan) uninstall() error { return errors.New("unsupported") }
