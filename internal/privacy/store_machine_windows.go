//go:build windows

package privacy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const machineStoreSDDL = "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

func machineRole(sid *windows.SID) bool {
	return sid != nil && (sid.IsWellKnown(windows.WinLocalSystemSid) || sid.IsWellKnown(windows.WinBuiltinAdministratorsSid))
}

// Machine files never gain an account-specific ACE. A user store intentionally
// has different ownership and cannot be substituted for this role-only store.
func trustedMachineDescriptor(sd *windows.SECURITY_DESCRIPTOR, private, protected bool) error {
	owner, _, err := sd.Owner()
	if err != nil || !machineRole(owner) {
		return errors.New("untrusted machine-store owner")
	}
	control, _, err := sd.Control()
	if err != nil || (protected && control&windows.SE_DACL_PROTECTED == 0) {
		return errors.New("machine directory inheritance is not protected")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("missing machine-store access control")
	}
	const writeAccess = 0x10000000 | 0x40000000 | 0x00000002 | 0x00000004 | 0x00000010 | 0x00000040 | 0x00000100 | 0x00010000 | 0x00040000 | 0x00080000
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil || ace == nil {
			return errors.New("invalid machine-store access entry")
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
			if (private || ace.Mask&writeAccess != 0) && !machineRole((*windows.SID)(unsafe.Pointer(&ace.SidStart))) {
				return errors.New("machine-store access is not limited to administrator roles")
			}
		default:
			return errors.New("unsupported machine-store access entry")
		}
	}
	return nil
}

// Check from the drive root downward before touching a descendant. Neither
// read-only reports nor writes may follow redirected state onto a remote drive.
func machineDirectoryPath(dir string, allowMissing bool) (bool, error) {
	clean := filepath.Clean(dir)
	volume := filepath.VolumeName(clean)
	if clean != dir || len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(clean) || strings.Contains(clean[2:], ":") {
		return false, errors.New("local absolute machine directory required")
	}
	root := volume + `\`
	p, err := windows.UTF16PtrFromString(root)
	if err != nil || windows.GetDriveType(p) != windows.DRIVE_FIXED {
		return false, errors.New("machine directory must be on a fixed local drive")
	}
	paths := []string{root}
	current := root
	for _, part := range strings.Split(strings.TrimPrefix(clean, root), `\`) {
		if part == "" {
			continue
		}
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false, errors.New("ambiguous machine directory")
		}
		current = filepath.Join(current, part)
		paths = append(paths, current)
	}
	for _, path := range paths {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return false, err
		}
		attrs, err := windows.GetFileAttributes(name)
		if err != nil {
			if allowMissing && path == clean && (errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)) {
				return false, nil
			}
			return false, err
		}
		if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return false, errors.New("machine directory contains a reparse point or non-directory")
		}
	}
	return true, nil
}

func checkMachineDirectory(dir string, private bool) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return trustedMachineDescriptor(sd, private, private)
}

func validateMachineDirectoryRead(dir string) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("machine-store access requires an elevated token")
	}
	if _, err := machineDirectoryPath(dir, false); err != nil {
		return err
	}
	if err := checkMachineDirectory(filepath.Dir(dir), false); err != nil {
		return err
	}
	return checkMachineDirectory(dir, true)
}

func validateMachineStoreRead(dir string) error {
	if err := validateMachineDirectoryRead(dir); err != nil {
		return err
	}
	return validateMachineChildren(dir)
}

func validateMachineChildren(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	count := 0
	for {
		entries, err := directory.ReadDir(128)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries {
			count++
			if count > 4096 {
				return errors.New("machine-store inventory requires administrator review")
			}
			file, err := openMachineFile(filepath.Join(dir, entry.Name()), windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.OPEN_EXISTING)
			if err != nil {
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
	}
}

func secureMachineDirectory(dir string) error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("machine-store access requires an elevated token")
	}
	exists, err := machineDirectoryPath(dir, true)
	if err != nil {
		return err
	}
	// privacyhost provides a fixed, trusted ProgramData product parent. Never
	// create arbitrary ancestors or take over a non-administrator-owned parent.
	if err := checkMachineDirectory(filepath.Dir(dir), false); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString(machineStoreSDDL)
	if err != nil {
		return err
	}
	if !exists {
		name, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
		sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		if err := windows.CreateDirectory(name, &sa); err != nil {
			return err
		}
	} else {
		// Even a role-owned legacy directory may have granted a named account
		// write access. Do not repair its ACL and silently trust historical data.
		if err := checkMachineDirectory(dir, true); err != nil {
			return err
		}
	}
	runtime.KeepAlive(sd)
	return validateMachineStoreRead(dir)
}

func openMachineFile(path string, access, share, disposition uint32) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var sa *windows.SecurityAttributes
	var sd *windows.SECURITY_DESCRIPTOR
	if disposition == windows.CREATE_NEW {
		sd, err = windows.SecurityDescriptorFromString(machineStoreSDDL)
		if err != nil {
			return nil, err
		}
		sa = &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	}
	h, err := windows.CreateFile(name, access|windows.READ_CONTROL, share, sa, disposition, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	closeOnError := func(err error) (*os.File, error) { windows.CloseHandle(h); return nil, err }
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return closeOnError(err)
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		return closeOnError(errors.New("machine-store file is not a single ordinary file"))
	}
	actual, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return closeOnError(err)
	}
	if err := trustedMachineDescriptor(actual, true, false); err != nil {
		return closeOnError(err)
	}
	return os.NewFile(uintptr(h), path), nil
}

func openMachineJournal(path string) (*os.File, error) {
	if filepath.Base(path) != journalName {
		return nil, errors.New("unexpected machine journal name")
	}
	return openMachineFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, windows.OPEN_EXISTING)
}

func lockMachineStore(dir string) (func(), error) {
	if err := secureMachineDirectory(dir); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "privacy.lock")
	f, err := openMachineFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.CREATE_NEW)
	if errors.Is(err, windows.ERROR_FILE_EXISTS) || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		f, err = openMachineFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.OPEN_EXISTING)
	}
	if err != nil {
		return nil, err
	}
	h := windows.Handle(f.Fd())
	ov := &windows.Overlapped{}
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ov); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = windows.UnlockFileEx(h, 0, 1, 0, ov); _ = f.Close() }, nil
}

func durableMachineWrite(path string, data []byte) error {
	if filepath.Base(path) != journalName {
		return errors.New("unexpected machine journal name")
	}
	dir := filepath.Dir(path)
	if err := secureMachineDirectory(dir); err != nil {
		return err
	}
	if old, err := openMachineJournal(path); err == nil {
		old.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	temp := filepath.Join(dir, ".privacy-journal-"+hex.EncodeToString(random[:]))
	f, err := openMachineFile(temp, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, windows.CREATE_NEW)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := atomicReplace(temp, path); err != nil {
		return err
	}
	f, err = openMachineJournal(path)
	if err == nil {
		err = f.Close()
	}
	return err
}
