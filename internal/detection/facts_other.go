//go:build !windows

package detection

import "runtime"

func ReadFacts() Facts {
	return Facts{Platform: runtime.GOOS, Warnings: []string{"此版本仅支持 Windows 本地检查。"}}
}
