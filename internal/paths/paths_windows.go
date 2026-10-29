// Package paths 各平台的文件位置。数据布局三端一致:
// settings.json、profiles/<id>.json、config.json、config.last-good.json、state.json、cache.db、logs/、rulesets/、diag/。
//
// Windows uses the ProgramData known folder, not the similarly named environment
// variable. A DATA override must pass the same privileged trust checks.
package paths

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// DataDir 服务数据目录。
func DataDir() string {
	if p := os.Getenv(DataEnvironment); p != "" {
		return p
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		// No relative-path or environment fallback: preparation rejects this path.
		return ""
	}
	return filepath.Join(base, AppName)
}

// ConfDir 设置文件目录;Windows 上与数据目录相同。
func ConfDir() string { return DataDir() }
