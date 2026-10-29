package privacydata

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/localstate"
)

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func validatePath(path string, destination bool) error {
	release, err := pinValidatedPath(path, destination)
	if err != nil {
		return err
	}
	defer release()
	return nil
}

// The parent walk precedes leaf I/O. Windows leases reject existing reparse
// points and hold every local parent against data-write/delete opens. Callers
// creating or opening a handle retain the lease until handle verification ends.
func pinValidatedPath(path string, destination bool) (func(), error) {
	if err := validatePathSyntax(path); err != nil {
		return nil, err
	}
	release, err := localstate.AcquireDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, errors.New("父目录无法安全访问；Windows 仅支持可确认的本地固定磁盘，不支持重解析点、网络盘或无法确认的目录")
	}
	if err := validatePathEntry(path, destination); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func validatePathSyntax(path string) error {
	if path == "" || !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 {
		return errors.New("请选择绝对路径的本地文件")
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\??\`) {
		return errors.New("不支持 UNC、设备或扩展设备路径")
	}
	for i, c := range path {
		if c == ':' && !(i == 1 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z'))) {
			return errors.New("不支持备用数据流或设备路径")
		}
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	for _, part := range parts {
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return errors.New("路径包含不明确的名称或父目录跳转")
		}
		base := strings.TrimRight(strings.ToUpper(strings.SplitN(part, ".", 2)[0]), " ")
		devicePort := false
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			suffix := base[3:]
			devicePort = strings.Contains("123456789¹²³", suffix) && len([]rune(suffix)) == 1
		}
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || devicePort {
			return errors.New("不支持 Windows 设备保留名称")
		}
	}
	return nil
}

func validatePathEntry(path string, destination bool) error {
	if err := validateLocalVolume(path); err != nil {
		return err
	}
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			if destination && p == filepath.Clean(path) && errors.Is(err, os.ErrNotExist) {
			} else {
				return errors.New("文件或父目录无法访问")
			}
		} else {
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("不支持符号链接或重解析点")
			}
			if err := rejectReparse(p); err != nil {
				return err
			}
			if p == filepath.Clean(path) {
				if destination {
					return errors.New("输出文件已存在，不会覆盖")
				}
				if !info.Mode().IsRegular() {
					return errors.New("只支持普通本地文件，不能选择目录或设备")
				}
			} else if !info.IsDir() {
				return errors.New("父路径不是目录")
			}
		}
		if filepath.Dir(p) == p {
			break
		}
	}
	return nil
}
