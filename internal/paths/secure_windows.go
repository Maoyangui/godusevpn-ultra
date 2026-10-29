package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	sddlPrivate         = "O:BAG:BAD:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)"
	trustedInstallerSID = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	maxStoreEntries     = 20000
	fileDeleteChild     = 0x40
)

type storeACE struct {
	typ, flags byte
	mask       uint32
	sid        string
}

type storeNode struct {
	directory, reparse bool
	links              uint32
	owner              string
	aces               []storeACE
	close              func()
}

// The small backend lets tests exercise trust decisions without touching ACLs.
type storeBackend interface {
	fixedDrive(string) error
	open(string) (*storeNode, error)
	create(string, bool) error
	entries(string) ([]string, error)
	harden(string, bool) error
}

// PrepareServiceStore verifies service inputs before any configuration, ruleset
// or log is consumed. Optional privacy stores are not service inputs. Keep
// release until the daemon is closed: open
// directory handles deny rename/delete of each checked path component.
// It never takes ownership of an existing directory or repairs an untrusted ACL.
func PrepareServiceStore() (func(), error) {
	return prepareStore(DataDir(), nativeStore{})
}

func cleanStorePath(root string) (string, error) {
	if len(root) < 4 || len(root) > 30000 || !filepath.IsAbs(root) || len(filepath.VolumeName(root)) != 2 || root[1] != ':' || root[2] != '\\' {
		return "", errors.New("服务数据目录必须是本地磁盘的绝对路径")
	}
	root = strings.TrimRight(root, `\`)
	if len(root) < 4 {
		return "", errors.New("服务数据目录不能是磁盘根目录")
	}
	for _, component := range strings.Split(root[3:], `\`) {
		if !safeComponent(component) {
			return "", errors.New("服务数据目录包含不安全的路径组件")
		}
	}
	return root, nil
}

func safeComponent(name string) bool {
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") || strings.ContainsAny(name, "\\/:*?\"<>|\x00") {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
		return false
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		tail := base[3:]
		if len(tail) == 1 && tail[0] >= '1' && tail[0] <= '9' {
			return false
		}
		if tail == "¹" || tail == "²" || tail == "³" {
			return false
		}
	}
	return true
}

var serviceDirectories = []string{"logs", "diag", "profiles", "rulesets"}

// Keep in sync with paths.go, ipc controller registration and netmode backups.
// Unknown root entries (including privacy-machine/data-tool state) are ignored,
// never opened, traversed or interpreted by service startup.
func serviceRootFiles() []string {
	var files []string
	for _, base := range []string{"settings.json", "profile.json", "config.json", "config.last-good.json", "state.json", "cache.db", "ui.json", "controller.sid", "nic-ipv6-backup.txt", "nic-ipv6-lost.txt"} {
		files = append(files, base, base+".tmp", base+".bad")
	}
	return append(files, "cache.db-wal", "cache.db-shm", "cache.db-journal")
}

func prepareStore(root string, fs storeBackend) (_ func(), err error) {
	root, err = cleanStorePath(root)
	if err != nil {
		return nil, err
	}
	if err = fs.fixedDrive(root[:3]); err != nil {
		return nil, err
	}
	var held []func()
	var once sync.Once
	release := func() {
		once.Do(func() {
			for i := len(held) - 1; i >= 0; i-- {
				held[i]()
			}
		})
	}
	defer func() {
		if err != nil {
			release()
		}
	}()
	openDirectory := func(path string, ancestor bool, readable bool) error {
		n, e := fs.open(path)
		if errors.Is(e, os.ErrNotExist) {
			if ancestor && path == root[:3] {
				return errors.New("服务数据磁盘不存在")
			}
			if e = fs.create(path, readable); e != nil {
				return e
			}
			n, e = fs.open(path)
		}
		if e != nil {
			return e
		}
		held = append(held, n.close)
		if !n.directory {
			return errors.New("服务数据路径包含非目录")
		}
		return checkStoreNode(n, ancestor)
	}
	// Visit from the volume down. A successful check remains pinned while later
	// children are checked/created; an untrusted existing ancestor is never fixed.
	chain := []string{root[:3]}
	p := root[:3]
	for _, component := range strings.Split(root[3:], `\`) {
		p = filepath.Join(p, component)
		chain = append(chain, p)
	}
	for i, path := range chain {
		if err = openDirectory(path, i != len(chain)-1, false); err != nil {
			return nil, fmt.Errorf("服务数据目录不可信（所有者、权限或重解析检查失败）；请管理员离线核对，未接管目录: %w", err)
		}
	}
	// Probe the EXACT fixed paths later used by the service. Enumeration plus
	// Go case folding must not decide whether Windows resolved an input alias.
	// Unknown root entries are never opened, including optional privacy stores.
	for _, name := range serviceRootFiles() {
		var n *storeNode
		n, err = fs.open(filepath.Join(root, name))
		if errors.Is(err, os.ErrNotExist) {
			err = nil
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("无法核对服务核心文件: %w", err)
		}
		err = checkStoreNode(n, false)
		if err == nil && n.directory {
			err = errors.New("服务核心文件实际为目录")
		}
		n.close()
		if err != nil {
			return nil, fmt.Errorf("服务核心文件不可信，未读取配置或订阅: %w", err)
		}
	}
	var pending []string
	var missing []string
	// Legacy logs/archives may have protected public-read DACLs. Tighten every
	// validated descendant explicitly; changing only its parent is insufficient.
	var privateDiagnosticPaths []string
	for _, sub := range serviceDirectories {
		path := filepath.Join(root, sub)
		var n *storeNode
		n, err = fs.open(path)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
			missing = append(missing, sub)
			continue
		}
		if err != nil {
			return nil, err
		}
		held = append(held, n.close)
		if !n.directory {
			return nil, errors.New("服务核心子目录实际为文件")
		}
		if err = checkStoreNode(n, false); err != nil {
			return nil, fmt.Errorf("服务核心子目录不可信: %w", err)
		}
		pending = append(pending, path)
		if sub == "logs" || sub == "diag" {
			privateDiagnosticPaths = append(privateDiagnosticPaths, path)
		}
	}
	// Only these core subtrees are enumerated. Stop at every reparse point.
	count := 0
	for len(pending) > 0 {
		dir := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		var names []string
		names, err = fs.entries(dir)
		if err != nil {
			return nil, fmt.Errorf("无法核对服务数据子项: %w", err)
		}
		for _, name := range names {
			count++
			if count > maxStoreEntries || !safeComponent(name) {
				return nil, errors.New("服务数据子项数量或名称超出安全检查范围")
			}
			path := filepath.Join(dir, name)
			var n *storeNode
			n, err = fs.open(path)
			if err != nil {
				return nil, fmt.Errorf("无法核对服务数据子项: %w", err)
			}
			err = checkStoreNode(n, false)
			if n.directory && err == nil {
				held = append(held, n.close)
				pending = append(pending, path)
			} else {
				n.close()
			}
			if err != nil {
				return nil, fmt.Errorf("服务数据已有子项不可信，未读取配置或订阅: %w", err)
			}
			if withinDiagnosticTree(root, path) {
				privateDiagnosticPaths = append(privateDiagnosticPaths, path)
			}
		}
	}
	// New children get a protected descriptor at CreateDirectory, not afterwards.
	for _, sub := range missing {
		if err = openDirectory(filepath.Join(root, sub), false, false); err != nil {
			return nil, err
		}
	}
	// Tightening may only happen after every consumed entry passed validation.
	privateDiagnosticPaths = append(privateDiagnosticPaths, root, filepath.Join(root, "logs"), filepath.Join(root, "diag"))
	for _, path := range privateDiagnosticPaths {
		if err = fs.harden(path, false); err != nil {
			return nil, fmt.Errorf("服务数据权限收紧失败，已停止初始化: %w", err)
		}
	}
	return release, nil
}

func withinDiagnosticTree(root, path string) bool {
	for _, sub := range []string{"logs", "diag"} {
		parent := filepath.Join(root, sub)
		if strings.HasPrefix(path, parent+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func trustedStoreSID(s string) bool { return s == "S-1-5-18" || s == "S-1-5-32-544" }

func checkStoreNode(n *storeNode, ancestor bool) error {
	if n.reparse {
		return errors.New("发现重解析点")
	}
	if !trustedStoreSID(n.owner) && !(ancestor && n.owner == trustedInstallerSID) {
		return errors.New("所有者不是 SYSTEM 或 Administrators")
	}
	if !n.directory && n.links != 1 {
		return errors.New("文件硬链接数不可信")
	}
	// Ancestors may allow creating NEW siblings (ProgramData normally does).
	// They must not allow replacing this existing child or resetting its ACL.
	mask := uint32(windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | fileDeleteChild)
	if !ancestor {
		mask |= windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES
	}
	for _, ace := range n.aces {
		if ace.flags&windows.INHERIT_ONLY_ACE != 0 {
			// An ancestor's template cannot grant access to this pinned existing
			// child, and the product root is explicitly protected before writes.
			// In a core directory, OI/CI templates WILL affect newly written
			// cache/tmp/ruleset files, so inspect their future write grants too.
			if ancestor || !n.directory || ace.flags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) == 0 {
				continue
			}
		}
		switch ace.typ {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue // Conservative: a deny never makes a dangerous allow trusted.
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if ace.mask&mask != 0 && !trustedStoreSID(ace.sid) && !(ancestor && ace.sid == trustedInstallerSID) {
				return errors.New("非受信主体可以写入或替换数据")
			}
		default:
			return errors.New("权限表含未支持的条件或对象 ACE")
		}
	}
	return nil
}

type nativeStore struct{}

func (nativeStore) fixedDrive(root string) error {
	p, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return err
	}
	if windows.GetDriveType(p) != windows.DRIVE_FIXED {
		return errors.New("服务数据目录必须位于本地固定磁盘")
	}
	return nil
}

func (nativeStore) open(path string) (*storeNode, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// FILE_READ_DATA is FILE_LIST_DIRECTORY for directories. A metadata-only
	// handle does not make the no-delete sharing lease effective on Windows.
	h, err := windows.CreateFile(p, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_READ_DATA,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			windows.CloseHandle(h)
		}
	}()
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		return nil, err
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	n, err := descriptorNode(sd)
	if err != nil {
		return nil, err
	}
	n.directory = info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	n.reparse = info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	n.links = info.NumberOfLinks
	n.close = func() { windows.CloseHandle(h) }
	ok = true
	return n, nil
}

func descriptorNode(sd *windows.SECURITY_DESCRIPTOR) (*storeNode, error) {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return nil, errors.New("缺少所有者")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return nil, errors.New("缺少有效 DACL")
	}
	n := &storeNode{owner: owner.String()}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err = windows.GetAce(acl, i, &ace); err != nil {
			return nil, err
		}
		if ace == nil {
			return nil, errors.New("无效 ACE")
		}
		a := storeACE{typ: ace.Header.AceType, flags: ace.Header.AceFlags, mask: uint32(ace.Mask)}
		if a.typ == windows.ACCESS_ALLOWED_ACE_TYPE || a.typ == windows.ACCESS_DENIED_ACE_TYPE {
			a.sid = (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
		}
		n.aces = append(n.aces, a)
	}
	runtime.KeepAlive(sd)
	return n, nil
}

func (nativeStore) create(path string, _ bool) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(sddlPrivate)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(p, &sa)
	runtime.KeepAlive(sd)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil
	} // caller reopens and validates
	return err
}

func (nativeStore) entries(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}

func (nativeStore) harden(path string, _ bool) error {
	sd, err := windows.SecurityDescriptorFromString(sddlPrivate)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// Never set OWNER_SECURITY_INFORMATION: existing ownership must already pass.
	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil)
	runtime.KeepAlive(sd)
	return err
}
