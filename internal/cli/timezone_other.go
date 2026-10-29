//go:build !windows

package cli

import "errors"

func currentTimezoneDirectory() (string, error) {
	return "", errors.New("此版本仅支持Windows时区恢复")
}
