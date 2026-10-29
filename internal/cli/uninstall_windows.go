package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

type uninstallPlan struct {
	root, service string
	files         []*os.File
}

func (p *uninstallPlan) close() {
	for i := len(p.files) - 1; i >= 0; i-- {
		_ = p.files[i].Close()
	}
	p.files = nil
}
func trustedCodeSID(s string, ancestor bool) bool {
	return s == "S-1-5-18" || s == "S-1-5-32-544" || ancestor && s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
}

func verifyCodeDescriptor(sd *windows.SECURITY_DESCRIPTOR, ancestor, protected bool) error {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trustedCodeSID(owner.String(), ancestor) {
		return errors.New("安装代码所有者不受信，未接管目录")
	}
	control, _, err := sd.Control()
	if err != nil || protected && control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("安装根权限未受保护")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("安装代码缺少有效DACL")
	}
	mask := uint32(0x500d0040)
	if !ancestor {
		mask |= 0x116
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil || ace == nil {
			return errors.New("无效安装权限")
		}
		if ancestor && ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("未支持的安装权限")
		}
		if uint32(ace.Mask)&mask != 0 && !trustedCodeSID((*windows.SID)(unsafe.Pointer(&ace.SidStart)).String(), ancestor) {
			return errors.New("普通主体可修改安装代码或替换父目录")
		}
	}
	return nil
}

func openPinnedCode(path string, directory bool) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	access := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES)
	share := uint32(windows.FILE_SHARE_READ)
	flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		access |= windows.FILE_LIST_DIRECTORY
		share |= windows.FILE_SHARE_WRITE
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
	} else {
		access |= windows.FILE_READ_DATA
	}
	return windows.CreateFile(name, access, share, nil, windows.OPEN_EXISTING, flags, 0)
}
func (p *uninstallPlan) pin(path string, directory, ancestor, protected bool) error {
	h, err := openPinnedCode(path, directory)
	if err != nil {
		return errors.New("安装代码无法安全锁定")
	}
	f := os.NewFile(uintptr(h), "ultra-installed-code")
	if f == nil {
		windows.CloseHandle(h)
		return errors.New("无效安装句柄")
	}
	p.files = append(p.files, f)
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return errors.New("安装路径含链接或类型异常")
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return verifyCodeDescriptor(sd, ancestor, protected)
}

func prepareUninstall() (_ *uninstallPlan, err error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return nil, errors.New("ProgramFiles KnownFolder不可用")
	}
	root := filepath.Join(base, "GodUseVPN Ultra")
	self, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Clean(self), filepath.Join(root, "godusevpn-cli.exe")) {
		return nil, errors.New("uninstall-machine仅允许固定ProgramFiles安装版CLI，不接受便携路径")
	}
	volume := filepath.VolumeName(root)
	if len(volume) != 2 || !filepath.IsAbs(root) || windows.GetDriveType(windows.StringToUTF16Ptr(volume+`\`)) != windows.DRIVE_FIXED {
		return nil, errors.New("安装位置不是本地固定磁盘")
	}
	p := &uninstallPlan{root: root, service: filepath.Join(root, "godusevpn-svc.exe")}
	ok := false
	defer func() {
		if !ok {
			p.close()
		}
	}()
	path := volume + `\`
	if err = p.pin(path, true, true, false); err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(root, volume+`\`), `\`) {
		path = filepath.Join(path, part)
		if err = p.pin(path, true, path != root, path == root); err != nil {
			return nil, err
		}
	}
	pending := []string{root}
	count := 0
	for len(pending) > 0 {
		dir := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		entries, e := os.ReadDir(dir)
		if e != nil {
			return nil, e
		}
		for _, entry := range entries {
			count++
			if count > 10000 {
				return nil, errors.New("安装文件超出检查上限")
			}
			path := filepath.Join(dir, entry.Name())
			directory := entry.IsDir()
			if err = p.pin(path, directory, false, false); err != nil {
				return nil, err
			}
			if directory {
				pending = append(pending, path)
			}
		}
	}
	ok = true
	return p, nil
}

func (p *uninstallPlan) verify() error {
	hashes := map[string]string{}
	// Files deny write/delete sharing; directory list handles deny delete sharing.
	// Trusted ACLs additionally restrict file creation and directory modification.
	err := filepath.WalkDir(p.root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(p.root, path)
		if e != nil {
			return e
		}
		name := filepath.ToSlash(rel)
		if !safeManifestPath(name) {
			return errors.New("不安全的安装文件名")
		}
		st, e := d.Info()
		if e != nil {
			return e
		}
		if st.Size() > 1<<30 {
			return errors.New("安装文件大小超出验证上限")
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		h := sha256.New()
		_, e = io.Copy(h, io.LimitReader(f, (1<<30)+1))
		f.Close()
		if e != nil {
			return e
		}
		hashes[name] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	if err != nil {
		return err
	}
	readBounded := func(name string, max int64) ([]byte, error) {
		f, e := os.Open(filepath.Join(p.root, name))
		if e != nil {
			return nil, e
		}
		defer f.Close()
		b, e := io.ReadAll(io.LimitReader(f, max+1))
		if int64(len(b)) > max {
			return nil, errors.New("安装清单大小超限")
		}
		return b, e
	}
	manifest, err := readBounded("SHA256SUMS", 4<<20)
	if err != nil {
		return err
	}
	descriptor, err := readBounded("ULTRA_PACKAGE.json", 64<<10)
	if err != nil {
		return err
	}
	if err = verifyUninstallManifest(manifest, descriptor, hashes); err != nil {
		return err
	}
	if err = p.verifyService(); err != nil {
		return err
	}
	return p.noInstalledGUI()
}

func (p *uninstallPlan) noInstalledGUI() error {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return errors.New("无法核对已安装GUI，请关闭后重试")
	}
	defer windows.CloseHandle(h)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(h, &entry); err == nil; err = windows.Process32Next(h, &entry) {
		relative := installedUIRelative(windows.UTF16ToString(entry.ExeFile[:]))
		if relative == "" {
			continue
		}
		proc, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			return errors.New("产品GUI进程来源无法核对，请关闭后重试")
		}
		buf := make([]uint16, 32768)
		n := uint32(len(buf))
		e = windows.QueryFullProcessImageName(proc, 0, &buf[0], &n)
		windows.CloseHandle(proc)
		if e != nil {
			return errors.New("产品GUI映像无法核对，请关闭后重试")
		}
		if strings.EqualFold(filepath.Clean(windows.UTF16ToString(buf[:n])), filepath.Join(p.root, relative)) {
			return errors.New("请先关闭已安装Ultra界面、浏览器及其助手，未终止任何进程")
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return errors.New("产品GUI枚举未完整完成")
	}
	return nil
}

func installedUIRelative(name string) string {
	if strings.EqualFold(name, "godusevpn.exe") {
		return "godusevpn.exe"
	}
	if strings.EqualFold(name, "GodUseVPN.Ultra.BrowserHost.exe") {
		return filepath.Join("browser-host", "GodUseVPN.Ultra.BrowserHost.exe")
	}
	return ""
}

func (p *uninstallPlan) verifyService() error {
	m, err := mgr.Connect()
	if err != nil {
		return errors.New("SCM无法核对，未卸载")
	}
	defer m.Disconnect()
	s, err := m.OpenService("godusevpn-ultra")
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return errors.New("Ultra服务身份不可查询")
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return errors.New("Ultra服务配置不可查询")
	}
	if !matchesInstalledServiceConfig(cfg, p.service) {
		return errors.New("服务映像或账号不属于固定安装，未执行卸载")
	}
	return nil
}

// Keep the SCM read separate from the fixed installation identity predicate so
// malformed configurations can be checked without opening or changing services.
func matchesInstalledServiceConfig(cfg mgr.Config, servicePath string) bool {
	args, err := windows.DecomposeCommandLine(cfg.BinaryPathName)
	return err == nil && len(args) == 2 && args[1] == "service" &&
		strings.EqualFold(filepath.Clean(args[0]), servicePath) &&
		cfg.ServiceType == windows.SERVICE_WIN32_OWN_PROCESS &&
		strings.EqualFold(cfg.ServiceStartName, "LocalSystem")
}

func (p *uninstallPlan) uninstall() error {
	cmd := exec.Command(p.service, "uninstall")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errors.New("固定Ultra服务卸载失败；恢复工具与备份已保留")
	}
	m, e := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if e != nil {
		return errors.New("卸载后SCM无法复核")
	}
	defer windows.CloseServiceHandle(m)
	s, e := windows.OpenService(m, windows.StringToUTF16Ptr("godusevpn-ultra"), windows.SERVICE_QUERY_STATUS)
	if e == nil {
		windows.CloseServiceHandle(s)
		return errors.New("服务仍存在或待删除，维护未完成")
	}
	if !errors.Is(e, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return errors.New("无法确认服务已删除")
	}
	return nil
}
