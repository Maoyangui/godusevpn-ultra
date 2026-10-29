//go:build windows

package metadatatool

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

// This is the exact, complete publisher ZIP file tree, including runtime,
// modules, docs and licenses. It is not loaded from a writable external manifest.
//
//go:embed bundle.sha256
var bundleManifest string

type nativeFiles struct{}
type nativeInput struct {
	file    *os.File
	release func()
}

func imagePath(path string) error {
	if len(path) < 4 || len(path) > 30000 || !filepath.IsAbs(path) || filepath.Clean(path) != path || len(filepath.VolumeName(path)) != 2 || path[1] != ':' || path[2] != '\\' {
		return ErrInput
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, `/:*?"<>|`) {
			return ErrInput
		}
		for _, r := range part {
			if r < 32 {
				return ErrInput
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return ErrInput
		}
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			tail := base[3:]
			if len([]rune(tail)) == 1 && strings.Contains("123456789¹²³", tail) {
				return ErrInput
			}
		}
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return ErrInput
	}
	return nil
}
func verifyFile(h windows.Handle, path string, links bool) error {
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || links && info.NumberOfLinks != 1 {
		return ErrInput
	}
	if typ, err := windows.GetFileType(h); err != nil || typ != windows.FILE_TYPE_DISK {
		return ErrInput
	}
	var final [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(h, &final[0], uint32(len(final)), 0)
	if err != nil || n >= uint32(len(final)) {
		return ErrInput
	}
	actual := strings.TrimPrefix(windows.UTF16ToString(final[:n]), `\\?\`)
	// Windows TEMP and a file chooser may use an ordinary 8.3 name. Expand that
	// spelling while the complete parent chain and leaf object remain pinned.
	n, err = windows.GetLongPathName(windows.StringToUTF16Ptr(path), &final[0], uint32(len(final)))
	if err != nil || n >= uint32(len(final)) || !strings.EqualFold(actual, windows.UTF16ToString(final[:n])) {
		return ErrInput
	}
	return nil
}
func (nativeFiles) openInput(path string) (inputLease, error) {
	if imagePath(path) != nil {
		return nil, ErrInput
	}
	release, err := localstate.AcquireDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, ErrInput
	}
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		release()
		return nil, ErrInput
	}
	if verifyFile(h, path, true) != nil {
		windows.CloseHandle(h)
		release()
		return nil, ErrInput
	}
	return &nativeInput{file: os.NewFile(uintptr(h), "metadata-input"), release: release}, nil
}
func (s *nativeInput) read() ([]byte, error) {
	stat, err := s.file.Stat()
	if err != nil || stat.Size() < 1 || stat.Size() > maxImage {
		return nil, ErrInput
	}
	if _, err = s.file.Seek(0, io.SeekStart); err != nil {
		return nil, ErrInput
	}
	data, err := io.ReadAll(io.LimitReader(s.file, maxImage+1))
	if err != nil || len(data) > maxImage {
		return nil, ErrInput
	}
	return data, nil
}
func (s *nativeInput) unchanged(expected string) error {
	data, err := s.read()
	if err != nil || digest(data) != expected {
		return ErrChanged
	}
	return nil
}
func (s *nativeInput) close() { _ = s.file.Close(); s.release() }
func (nativeFiles) saveNew(input, path string, data []byte) (result error) {
	if imagePath(path) != nil || strings.EqualFold(input, path) {
		return ErrDestination
	}
	info, err := inspectImage(data)
	if err != nil || info.format == "PNG" && !strings.EqualFold(filepath.Ext(path), ".png") || info.format == "JPEG" && strings.EqualFold(filepath.Ext(path), ".png") {
		return ErrDestination
	}
	release, err := localstate.AcquireDirectory(filepath.Dir(path), false)
	if err != nil {
		return ErrDestination
	}
	defer release()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ErrDestination
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return ErrDestination
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE, windows.FILE_SHARE_READ, sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return ErrDestination
	}
	file := os.NewFile(uintptr(h), "metadata-copy")
	committed := false
	defer func() {
		if !committed {
			flag := byte(1)
			_ = windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &flag, 1)
		}
		_ = file.Close()
	}()
	if verifyFile(h, path, true) != nil {
		return ErrDestination
	}
	if n, err := file.Write(data); err != nil || n != len(data) {
		return ErrDestination
	}
	if file.Sync() != nil {
		return ErrDestination
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return ErrDestination
	}
	written, err := io.ReadAll(io.LimitReader(file, maxImage+1))
	if err != nil || digest(written) != digest(data) {
		return ErrDestination
	}
	committed = true
	return nil
}
func ordinaryToken() error {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil || user.User.Sid == nil {
		return ErrUnavailable
	}
	sid := user.User.Sid.String()
	if sid == "S-1-5-18" || sid == "S-1-5-19" || sid == "S-1-5-20" {
		return ErrUnavailable
	}
	var elevated, used uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevated)), 4, &used) != nil || used != 4 || elevated != 0 {
		return ErrUnavailable
	}
	var buffer [512]byte
	if windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], uint32(len(buffer)), &used) != nil {
		return ErrUnavailable
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	if label.Label.Sid == nil {
		return ErrUnavailable
	}
	level := label.Label.Sid.String()
	if level != "S-1-16-8192" && level != "S-1-16-8448" {
		return ErrUnavailable
	}
	return nil
}
func codeDescriptor(sd *windows.SECURITY_DESCRIPTOR, ancestor bool) error {
	trusted := func(s string) bool {
		return s == "S-1-5-18" || s == "S-1-5-32-544" || ancestor && s == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	}
	if sd == nil {
		return ErrUnavailable
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil || !trusted(owner.String()) {
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
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrUnavailable
		}
		if uint32(ace.Mask)&mask != 0 && !trusted((*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()) {
			return ErrUnavailable
		}
	}
	return nil
}
func parseBundleManifest(text string) (map[string]string, map[string]bool, error) {
	files := map[string]string{}
	dirs := map[string]bool{".": true}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, nil, ErrUnavailable
		}
		hash, name := line[:64], line[66:]
		raw, err := hex.DecodeString(hash)
		if err != nil || len(raw) != 32 || name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
			return nil, nil, ErrUnavailable
		}
		for _, part := range strings.Split(name, "/") {
			if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
				return nil, nil, ErrUnavailable
			}
		}
		name = strings.ToLower(name)
		if _, ok := files[name]; ok {
			return nil, nil, ErrUnavailable
		}
		files[name] = strings.ToLower(hash)
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(name))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			dirs[parent] = true
		}
	}
	if scanner.Err() != nil || len(files) != 1501 {
		return nil, nil, ErrUnavailable
	}
	return files, dirs, nil
}
func acquireInstalled() (_ backendLease, result error) {
	if runtime.GOARCH != "amd64" || ordinaryToken() != nil {
		return backendLease{}, ErrUnavailable
	}
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return backendLease{}, ErrUnavailable
	}
	root := filepath.Join(base, "GodUseVPN Ultra")
	self, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Clean(self), filepath.Join(root, "godusevpn.exe")) {
		return backendLease{}, ErrUnavailable
	}
	files, dirs, err := parseBundleManifest(bundleManifest)
	if err != nil {
		return backendLease{}, err
	}
	var held []*os.File
	release := func() {
		for i := len(held) - 1; i >= 0; i-- {
			_ = held[i].Close()
		}
	}
	complete := false
	defer func() {
		if !complete {
			release()
		}
	}()
	pin := func(path string, directory, ancestor bool) (*os.File, error) {
		access, flags := uint32(windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES|windows.FILE_READ_DATA), uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if directory {
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		}
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), access, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return nil, ErrUnavailable
		}
		file := os.NewFile(uintptr(h), "metadata-backend")
		held = append(held, file)
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
			return nil, ErrUnavailable
		}
		sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil || codeDescriptor(sd, ancestor) != nil {
			return nil, ErrUnavailable
		}
		return file, nil
	}
	parents, err := localstate.AcquireDirectory(root, false)
	if err != nil {
		return backendLease{}, ErrUnavailable
	}
	defer func() {
		if !complete {
			parents()
		}
	}()
	// Pin the complete ancestor chain with owner/DACL checks too. Ancestors may
	// grant create-child, but never replacement/deletion of existing code paths.
	volume := filepath.VolumeName(root)
	current := volume + `\`
	if _, err = pin(current, true, true); err != nil {
		return backendLease{}, err
	}
	for _, part := range strings.Split(root[3:], `\`) {
		current = filepath.Join(current, part)
		if _, err = pin(current, true, current != root); err != nil {
			return backendLease{}, err
		}
	}
	backend := filepath.Join(root, "metadata-tool", "exiftool-"+Version)
	if _, err = pin(filepath.Dir(backend), true, false); err != nil {
		return backendLease{}, err
	}
	seen := 0
	total := int64(0)
	err = filepath.WalkDir(backend, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ErrUnavailable
		}
		relative, err := filepath.Rel(backend, path)
		if err != nil {
			return ErrUnavailable
		}
		relative = strings.ToLower(filepath.ToSlash(relative))
		if entry.IsDir() {
			if !dirs[relative] {
				return ErrUnavailable
			}
			_, err := pin(path, true, false)
			return err
		}
		expected, ok := files[relative]
		if !ok {
			return ErrUnavailable
		}
		file, err := pin(path, false, false)
		if err != nil {
			return err
		}
		stat, err := file.Stat()
		if err != nil || stat.Size() > 32<<20 {
			return ErrUnavailable
		}
		total += stat.Size()
		if total > 256<<20 {
			return ErrUnavailable
		}
		hash := sha256.New()
		if _, err = io.Copy(hash, file); err != nil || hex.EncodeToString(hash.Sum(nil)) != expected {
			return ErrUnavailable
		}
		seen++
		return nil
	})
	if err != nil || seen != len(files) {
		return backendLease{}, ErrUnavailable
	}
	complete = true
	return backendLease{executable: filepath.Join(backend, "ExifTool.exe"), directory: backend, release: func() { release(); parents() }}, nil
}
func backendEnvironment() []string {
	root, err := windows.KnownFolderPath(windows.FOLDERID_Windows, 0)
	if err != nil {
		return []string{}
	}
	return []string{"SystemRoot=" + root, "WINDIR=" + root}
}
func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
