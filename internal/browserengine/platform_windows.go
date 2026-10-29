//go:build windows

package browserengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

func defaultDirectory() (string, error) {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return "", ErrUnsupported
	}
	sid := user.User.Sid.String()
	if sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" {
		return "", ErrUnsupported
	}
	var elevated, used uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used) != nil || used != 4 || elevated != 0 {
		return "", ErrUnsupported
	}
	// Namespace validation belongs to acquireStore before the first file I/O;
	// the shell must not verify a redirected UNC folder during this lookup.
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DONT_VERIFY)
	if err != nil {
		return "", ErrUnsupported
	}
	return filepath.Join(base, "GodUseVPN Ultra", "BrowserEngines"), nil
}
func acquireStore(dir string, create bool) (func(), error) {
	return localstate.AcquireDirectory(dir, create)
}

// Bind the opened os.Root object to the expected fixed directory even if a
// writable parent lease allowed an in-place reparse change during acquisition.
func verifyRootIdentity(root *os.Root, expected string) error {
	file, err := root.Open(".")
	if err != nil {
		return ErrStore
	}
	defer file.Close()
	handle := windows.Handle(file.Fd())
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrStore
	}
	var actual, long [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(handle, &actual[0], uint32(len(actual)), 0)
	if err != nil || n >= uint32(len(actual)) {
		return ErrStore
	}
	pointer, err := windows.UTF16PtrFromString(expected)
	if err != nil {
		return ErrStore
	}
	length, err := windows.GetLongPathName(pointer, &long[0], uint32(len(long)))
	if err != nil || length >= uint32(len(long)) {
		return ErrStore
	}
	if !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(actual[:n]), `\\?\`), windows.UTF16ToString(long[:length])) {
		return ErrStore
	}
	return nil
}

// Reads VERSIONINFO resources only. No browser process or DLL code is loaded.
// Cache integrity is not an Authenticode/publisher-trust execution decision.
func verifyBinaryVersion(path, version string) error {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size < 52 || size > 1<<20 {
		return ErrIntegrity
	}
	data := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])) != nil {
		return ErrIntegrity
	}
	var info *windows.VS_FIXEDFILEINFO
	var length uint32
	if windows.VerQueryValue(unsafe.Pointer(&data[0]), `\`, unsafe.Pointer(&info), &length) != nil || info == nil || length < uint32(unsafe.Sizeof(*info)) || info.Signature != 0xfeef04bd {
		return ErrIntegrity
	}
	actual := fmt.Sprintf("%d.%d.%d.%d", info.ProductVersionMS>>16, info.ProductVersionMS&65535, info.ProductVersionLS>>16, info.ProductVersionLS&65535)
	parts := strings.Split(strings.TrimSuffix(version, "esr"), ".")
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number > 65535 {
			return ErrIntegrity
		}
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	// Normalize leading zeros in selected metadata, rather than accepting a
	// different on-disk version that happens to share a major version.
	for i, part := range parts {
		number, _ := strconv.Atoi(part)
		parts[i] = strconv.Itoa(number)
	}
	if actual != strings.Join(parts, ".") {
		return ErrIntegrity
	}
	return nil
}
