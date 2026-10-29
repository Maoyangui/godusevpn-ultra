//go:build !windows

package privacydata

import (
	"errors"
	"os"
)

func validateLocalVolume(string) error { return nil }
func rejectReparse(string) error       { return nil }
func openSource(path string) (*os.File, error) {
	if err := validatePath(path, false); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("源文件无法读取")
	}
	opened, err := f.Stat()
	current, lerr := os.Lstat(path)
	if err != nil || lerr != nil || !opened.Mode().IsRegular() || !os.SameFile(opened, current) {
		f.Close()
		return nil, errors.New("源文件在打开期间发生变化")
	}
	return f, nil
}
func openDestination(path string) (*os.File, func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, errors.New("无法新建输出文件；已存在的文件不会覆盖")
	}
	created, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, errors.New("无法验证新建输出")
	}
	discard := func() error {
		current, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !os.SameFile(created, current) {
			return errors.New("输出已被外部替换，拒绝删除")
		}
		return os.Remove(path)
	}
	return f, discard, nil
}

func openPrivateDestination(path string) (*os.File, func() error, error) {
	return openDestination(path)
}
