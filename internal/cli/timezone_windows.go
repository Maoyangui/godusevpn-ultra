package cli

import (
	"errors"
	"github.com/Maoyangui/godusevpn/internal/identity"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

func currentTimezoneDirectory() (string, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", errors.New("当前账户身份无法确认")
	}
	sid := u.User.Sid.String()
	if sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" {
		return "", errors.New("时区恢复需在原用户终端运行，不能使用服务账号")
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_RoamingAppData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", errors.New("当前令牌的用户目录无法确认")
	}
	if legacy, e := os.UserConfigDir(); e != nil || !strings.EqualFold(filepath.Clean(legacy), filepath.Clean(base)) {
		return "", errors.New("APPDATA覆盖与当前令牌目录不同；无法安全确认原账户备份，未尝试其他路径")
	}
	return filepath.Join(base, identity.AppName, "privacy-time"), nil
}
