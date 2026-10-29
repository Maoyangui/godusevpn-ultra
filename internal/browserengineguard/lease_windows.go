//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

type nativeLease struct {
	job      windows.Handle
	programs []string
	files    []*os.File
	seal     []sealedObject
	release  func()
	once     sync.Once
}

func (l *nativeLease) Programs() []string { return append([]string(nil), l.programs...) }

type accounting struct {
	user, kernel, periodUser, periodKernel int64
	faults, total, active, terminated      uint32
}

func (l *nativeLease) Active() (uint32, error) {
	var info accounting
	var length uint32
	if err := windows.QueryInformationJobObject(l.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &length); err != nil {
		return 0, refusal("job_query", err)
	}
	if length != uint32(unsafe.Sizeof(info)) {
		return 0, ErrUnknown
	}
	return info.active, nil
}
func (l *nativeLease) Terminate() error {
	if windows.TerminateJobObject(l.job, 1) != nil {
		return ErrUnknown
	}
	return nil
}
func (l *nativeLease) Close() {
	l.once.Do(func() {
		for i := len(l.files) - 1; i >= 0; i-- {
			l.files[i].Close()
		}
		if l.release != nil {
			l.release()
		}
		windows.CloseHandle(l.job)
	})
}

func openNativeLease(owner *nativeOwner, r InstallRequest) (_ sessionLease, result error) {
	lease := &nativeLease{}
	complete := false
	stage := "lease_job_duplicate"
	defer func() {
		if !complete {
			lease.Close()
			if result != nil && DiagnosticFromError(result) == nil {
				result = refusal(stage, result)
			}
		}
	}()
	if !owner.alive() {
		return nil, ErrRefused
	}
	if err := windows.DuplicateHandle(owner.parent, windows.Handle(r.JobHandle), windows.CurrentProcess(), &lease.job, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, refusal(stage, err)
	}
	stage = "lease_job_limits"
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	var length uint32
	if err := windows.QueryInformationJobObject(lease.job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), &length); err != nil {
		return nil, err
	}
	if length != uint32(unsafe.Sizeof(limits)) || limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 || limits.BasicLimitInformation.LimitFlags&(windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK|windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK) != 0 {
		return nil, ErrRefused
	}
	// A guard must be installed while the sole initial process is suspended,
	// before it can create helpers or issue any network requests.
	stage = "lease_job_processes"
	var list struct {
		assigned, count uint32
		ids             [2]uintptr
	}
	if err := windows.QueryInformationJobObject(lease.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), &length); err != nil {
		return nil, err
	}
	if list.assigned != 1 || list.count != 1 || list.ids[0] != uintptr(r.MainPID) {
		return nil, ErrRefused
	}
	stage = "lease_process_open"
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, r.MainPID)
	if err != nil {
		return nil, refusal(stage, err)
	}
	defer windows.CloseHandle(process)
	var created, exit, kernel, user windows.Filetime
	stage = "lease_process_identity"
	if err := windows.GetProcessTimes(process, &created, &exit, &kernel, &user); err != nil {
		return nil, err
	}
	if uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) != r.MainCreated {
		return nil, ErrRefused
	}
	directory := filepath.Join(owner.root, r.SessionID, r.EngineID, r.Version)
	stage = "lease_directory"
	release, err := localstate.AcquireDirectory(directory, false)
	if err != nil {
		return nil, refusal(stage, err)
	}
	lease.release = release
	session := filepath.Join(owner.root, r.SessionID)
	if err := pinSealDirectory(lease, session); err != nil {
		return nil, err
	}
	if err := pinSealDirectory(lease, filepath.Join(session, r.EngineID)); err != nil {
		return nil, err
	}
	main := "chrome.exe"
	if r.EngineID == "firefox" {
		main = "firefox.exe"
	}
	var image [32768]uint16
	stage = "lease_image"
	n := uint32(len(image))
	if err := windows.QueryFullProcessImageName(process, 0, &image[0], &n); err != nil {
		return nil, err
	}
	if n == 0 || n > uint32(len(image)) || !strings.EqualFold(windows.UTF16ToString(image[:n]), filepath.Join(directory, main)) {
		return nil, ErrRefused
	}
	stage = "lease_receipt"
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, refusal(stage, err)
	}
	defer root.Close()
	receiptFile, err := root.Open(".ultra-engine-receipt.json")
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(receiptFile, (8<<20)+1))
	receiptFile.Close()
	if err != nil || len(raw) > 8<<20 {
		return nil, ErrRefused
	}
	var receipt browserengine.Receipt
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF || receipt.SchemaVersion != 1 || receipt.Release.EngineID != r.EngineID || receipt.Release.Version != r.Version || receipt.Executable != main || len(receipt.Files) == 0 || len(receipt.Files) > 20000 {
		return nil, ErrRefused
	}
	expected := map[string]browserengine.TreeFile{}
	dirs := map[string]bool{".": true}
	for _, entry := range receipt.Files {
		if !safeRelative(entry.Name) || expected[entry.Name].Name != "" || entry.Size < 0 || entry.Size > 512<<20 {
			return nil, ErrRefused
		}
		digest, err := hex.DecodeString(entry.SHA256)
		if err != nil || len(digest) != 32 {
			return nil, ErrRefused
		}
		expected[entry.Name] = entry
	}
	for _, name := range receipt.Directories {
		if !safeRelative(name) || dirs[name] {
			return nil, ErrRefused
		}
		dirs[name] = true
	}
	files, dirCount := 0, 0
	var total int64
	stage = "lease_code_pin"
	err = filepath.WalkDir(directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil {
			return ErrRefused
		}
		relative = filepath.ToSlash(relative)
		isDir := entry.IsDir()
		if isDir {
			if !dirs[relative] {
				return ErrRefused
			}
			dirCount++
		} else if relative != ".ultra-engine-receipt.json" && expected[relative].Name == "" {
			return ErrRefused
		}
		access, flags := uint32(windows.GENERIC_READ|windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER), uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if isDir {
			access = windows.FILE_LIST_DIRECTORY | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.WRITE_DAC | windows.WRITE_OWNER
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		}
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(name), access, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(h), "browser-guard-code")
		lease.files = append(lease.files, file)
		lease.seal = append(lease.seal, sealedObject{file, name, isDir})
		var info windows.ByHandleFileInformation
		if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != isDir || !isDir && info.NumberOfLinks != 1 {
			return ErrRefused
		}
		var final, long [32768]uint16
		n, err := windows.GetFinalPathNameByHandle(h, &final[0], uint32(len(final)), 0)
		if err != nil || n >= uint32(len(final)) {
			return ErrRefused
		}
		count, err := windows.GetLongPathName(windows.StringToUTF16Ptr(name), &long[0], uint32(len(long)))
		if err != nil || count >= uint32(len(long)) || !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(final[:n]), `\\?\`), windows.UTF16ToString(long[:count])) {
			return ErrRefused
		}
		if isDir || relative == ".ultra-engine-receipt.json" {
			return nil
		}
		want := expected[relative]
		stat, err := file.Stat()
		if err != nil || stat.Size() != want.Size {
			return ErrRefused
		}
		total += want.Size
		if total > 2<<30 {
			return ErrRefused
		}
		hash := sha256.New()
		if _, err = io.Copy(hash, io.LimitReader(file, want.Size+1)); err != nil || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(want.SHA256) {
			return ErrRefused
		}
		files++
		if strings.EqualFold(filepath.Ext(name), ".exe") {
			lease.programs = append(lease.programs, name)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if files != len(expected) || dirCount != len(dirs) || len(lease.programs) == 0 {
		return nil, ErrRefused
	}
	names := map[string]bool{".ultra-engine-receipt.json": true}
	for name := range dirs {
		names[name] = true
	}
	for name := range expected {
		names[name] = true
	}
	stage = "lease_code_seal"
	if err := sealCode(lease, owner.sid, session, r.EngineID, r.Version, names); err != nil {
		return nil, refusal(stage, err)
	}
	complete = true
	return lease, nil
}
func safeRelative(name string) bool {
	if name == "" || len(name) > 1024 || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.ContainsAny(name, `\:*?"<>|`) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		for _, r := range part {
			if r < 32 || r > 126 {
				return false
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return false
		}
		if (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && len(base) == 4 && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}
