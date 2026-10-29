//go:build windows && (amd64 || arm64)

package locationportalgate

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

const providerServiceName = "godusevpn-ultra"

// BFE disables persistent filters on startup when the provider has no service
// association or that service is not AutoStart. Bind only the existing fixed
// installed service; this package never installs/enables/starts that service.
// The SCM command and protected pinned image are code identity, not a publisher
// signature or proof that BFE restart/network behavior has passed a VM test.
type installedServiceLease struct {
	manager, service windows.Handle
	expected         string
	held             []windows.Handle
	releaseDirectory func()
}

func fixedProviderService(value *uint16) bool {
	if value == nil {
		return false
	}
	// BFE returns a terminated service-name allocation. Bound reads to the only
	// exact accepted literal instead of scanning an arbitrary-length string.
	for i, want := range []byte(providerServiceName) {
		if *(*uint16)(unsafe.Add(unsafe.Pointer(value), i*2)) != uint16(want) {
			return false
		}
	}
	return *(*uint16)(unsafe.Add(unsafe.Pointer(value), len(providerServiceName)*2)) == 0
}

func sameASCIIPath(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if x >= 'a' && x <= 'z' {
			x -= 'a' - 'A'
		}
		if y >= 'a' && y <= 'z' {
			y -= 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func fixedInstalledServiceConfig(start, kind uint32, account, command, expected string) bool {
	if start != windows.SERVICE_AUTO_START || kind != windows.SERVICE_WIN32_OWN_PROCESS || !sameASCIIPath(account, "LocalSystem") || expected == "" {
		return false
	}
	// Installer emits the first form. Quoting the fixed service argument has
	// the same meaning; all additional arguments/expansion/alternate paths fail.
	end := 0
	if strings.HasSuffix(command, `" service`) {
		end = len(command) - len(`" service`)
	} else if strings.HasSuffix(command, `" "service"`) {
		end = len(command) - len(`" "service"`)
	}
	return end > 1 && command[0] == '"' && sameASCIIPath(command[1:end], expected)
}

func configString(buffer []byte, pointer *uint16) (string, error) {
	if len(buffer) < 2 || pointer == nil {
		return "", ErrUnknown
	}
	begin := uintptr(unsafe.Pointer(&buffer[0]))
	position := uintptr(unsafe.Pointer(pointer))
	if position < begin || position-begin >= uintptr(len(buffer)) || (position-begin)%2 != 0 {
		return "", ErrUnknown
	}
	remaining := (len(buffer) - int(position-begin)) / 2
	if remaining > 32768 {
		return "", ErrUnknown
	}
	text := unsafe.Slice(pointer, remaining)
	for i, value := range text {
		if value == 0 {
			return windows.UTF16ToString(text[:i]), nil
		}
	}
	return "", ErrUnknown
}

func (l *installedServiceLease) verify() error {
	if l == nil || l.service == 0 || l.expected == "" || len(l.held) != 1 {
		return ErrUnknown
	}
	var needed uint32
	err := windows.QueryServiceConfig(l.service, nil, 0, &needed)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || needed < uint32(unsafe.Sizeof(windows.QUERY_SERVICE_CONFIG{})) || needed > 65536 {
		return ErrUnknown
	}
	buffer := make([]byte, needed)
	if windows.QueryServiceConfig(l.service, (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buffer[0])), uint32(len(buffer)), &needed) != nil {
		return ErrUnknown
	}
	config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buffer[0]))
	command, commandErr := configString(buffer, config.BinaryPathName)
	account, accountErr := configString(buffer, config.ServiceStartName)
	valid := commandErr == nil && accountErr == nil && fixedInstalledServiceConfig(config.StartType, config.ServiceType, account, command, l.expected)
	runtime.KeepAlive(buffer)
	if !valid {
		return ErrUnknown
	}
	return nil
}

func acquireInstalledService() (_ *installedServiceLease, err error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DONT_VERIFY)
	if err != nil {
		return nil, ErrUnavailable
	}
	root := filepath.Join(base, "GodUseVPN Ultra")
	l := &installedServiceLease{expected: filepath.Join(root, "godusevpn-svc.exe")}
	complete := false
	defer func() {
		if !complete {
			l.close()
		}
	}()
	// The shared directory lease rejects UNC/device/ADS/SUBST/network mappings
	// before file I/O and pins ancestors without following reparse points.
	l.releaseDirectory, err = localstate.AcquireDirectory(root, false)
	if err != nil {
		return nil, ErrUnavailable
	}
	volume := filepath.VolumeName(root)
	current := volume + `\`
	if err = checkInstalledDirectory(current, true, false); err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(root, current), `\`) {
		current = filepath.Join(current, part)
		if err = checkInstalledDirectory(current, current != root, current == root); err != nil {
			return nil, err
		}
	}
	handle, err := windows.CreateFile(windows.StringToUTF16Ptr(l.expected), windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_READ_DATA, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	l.held = append(l.held, handle)
	var info windows.ByHandleFileInformation
	kind, kindErr := windows.GetFileType(handle)
	if windows.GetFileInformationByHandle(handle, &info) != nil || kindErr != nil || kind != windows.FILE_TYPE_DISK || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 || info.FileSizeHigh != 0 || info.FileSizeLow == 0 {
		return nil, ErrUnavailable
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || checkInstalledCodeDescriptor(sd, false, false) != nil {
		return nil, ErrUnavailable
	}
	l.manager, err = windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, ErrUnavailable
	}
	l.service, err = windows.OpenService(l.manager, windows.StringToUTF16Ptr(providerServiceName), windows.SERVICE_QUERY_CONFIG)
	if err != nil || l.verify() != nil {
		return nil, ErrUnavailable
	}
	complete = true
	return l, nil
}

func checkInstalledDirectory(path string, ancestor, protected bool) error {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_LIST_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return ErrUnavailable
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrUnavailable
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return ErrUnavailable
	}
	return checkInstalledCodeDescriptor(sd, ancestor, protected)
}

func trustedCodeSID(sid string, ancestor bool) bool {
	return sid == "S-1-5-18" || sid == "S-1-5-32-544" || ancestor && sid == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
}

func checkInstalledCodeDescriptor(sd *windows.SECURITY_DESCRIPTOR, ancestor, protected bool) error {
	if sd == nil {
		return ErrUnavailable
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trustedCodeSID(owner.String(), ancestor) {
		return ErrUnavailable
	}
	control, _, err := sd.Control()
	if err != nil || protected && control&windows.SE_DACL_PROTECTED == 0 {
		return ErrUnavailable
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return ErrUnavailable
	}
	mask := uint32(0x500d0040)
	if !ancestor {
		mask |= 0x116
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil {
			return ErrUnavailable
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 || ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || uint32(ace.Mask)&mask != 0 && !trustedCodeSID((*windows.SID)(unsafe.Pointer(&ace.SidStart)).String(), ancestor) {
			return ErrUnavailable
		}
	}
	return nil
}

func (l *installedServiceLease) close() {
	if l.service != 0 {
		_ = windows.CloseServiceHandle(l.service)
		l.service = 0
	}
	if l.manager != 0 {
		_ = windows.CloseServiceHandle(l.manager)
		l.manager = 0
	}
	for i := len(l.held) - 1; i >= 0; i-- {
		_ = windows.CloseHandle(l.held[i])
	}
	l.held = nil
	if l.releaseDirectory != nil {
		l.releaseDirectory()
		l.releaseDirectory = nil
	}
}
