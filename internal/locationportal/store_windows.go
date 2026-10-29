//go:build windows

package locationportal

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"golang.org/x/sys/windows"
)

type nativeStore struct {
	directory, parent, lock *os.File
	release                 func()
	owner                   string
	mode                    storeMode
	overlap                 windows.Overlapped
	once                    sync.Once
	locked, known           bool
	expected                []byte
}

func portalFileError(err error) error {
	if errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND) || errors.Is(err, windows.STATUS_OBJECT_PATH_NOT_FOUND) || errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	if errors.Is(err, windows.STATUS_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.STATUS_OBJECT_NAME_COLLISION) {
		return ErrBusy
	}
	return ErrStore
}

func portalSD(owner string, directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	flags := ""
	if directory {
		flags = "OICI"
	}
	return windows.SecurityDescriptorFromString("D:P(A;" + flags + ";FA;;;SY)(A;" + flags + ";FA;;;BA)(A;" + flags + ";FA;;;" + owner + ")")
}

type portalOpenPolicy struct{ access, share, disposition, options, attributes uint32 }

func portalPolicy(directory, write bool, disposition uint32) portalOpenPolicy {
	p := portalOpenPolicy{access: windows.FILE_GENERIC_READ | windows.READ_CONTROL | windows.SYNCHRONIZE, share: windows.FILE_SHARE_READ, disposition: disposition, options: windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT, attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	if directory {
		p.options |= windows.FILE_DIRECTORY_FILE
		// Relative rename opens the target directory internally for data-write.
		// Permit that access; no-delete ancestor pins and private ACLs remain.
		p.share |= windows.FILE_SHARE_WRITE
	} else {
		p.options |= windows.FILE_NON_DIRECTORY_FILE
	}
	if write {
		p.access |= windows.FILE_GENERIC_WRITE
		if !directory {
			p.access |= windows.DELETE
			p.options |= windows.FILE_WRITE_THROUGH
		}
	}
	return p
}

func portalOpen(parent *os.File, name, owner string, directory, write bool, disposition uint32) (*os.File, error) {
	return portalOpenWithPolicy(parent, name, owner, directory, portalPolicy(directory, write, disposition))
}

func portalLockPolicy(disposition uint32) portalOpenPolicy {
	p := portalPolicy(false, false, disposition)
	// A permanent, empty byte-range lock, never a rename/delete candidate.
	p.access |= windows.FILE_GENERIC_WRITE
	p.share |= windows.FILE_SHARE_WRITE
	return p
}

func portalOpenWithPolicy(parent *os.File, name, owner string, directory bool, p portalOpenPolicy) (*os.File, error) {
	if parent == nil || name == "" || len(name) > 96 || strings.ContainsAny(name, "/\\:\x00\r\n") || name == "." || name == ".." || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return nil, ErrStore
	}
	text, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, ErrStore
	}
	oa := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(parent.Fd()), ObjectName: text, Attributes: p.attributes}
	if p.disposition == windows.FILE_CREATE || p.disposition == windows.FILE_OPEN_IF {
		oa.SecurityDescriptor, err = portalSD(owner, directory)
		if err != nil {
			return nil, ErrStore
		}
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, p.access, &oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, p.share, p.disposition, p.options, 0, 0)
	validInformation := status.Information == 1 && p.disposition != windows.FILE_CREATE || status.Information == 2 && p.disposition != windows.FILE_OPEN
	if err != nil || handle == 0 || handle == windows.InvalidHandle || status.Status != 0 || !validInformation {
		if handle != 0 && handle != windows.InvalidHandle {
			windows.CloseHandle(handle)
		}
		return nil, portalFileError(err)
	}
	file := os.NewFile(uintptr(handle), "ultra-portal-private-object")
	if verifyPortalHandle(handle, directory, owner) != nil {
		file.Close()
		return nil, ErrStore
	}
	return file, nil
}

type privateACE struct {
	sid         string
	flags, kind uint8
	mask        uint32
}

const portalFileAllAccess uint32 = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff

func portalPrivateACL(owner, user string, protected, directory bool, entries []privateACE) bool {
	trusted := func(value string) bool { return value == user || value == "S-1-5-18" || value == "S-1-5-32-544" }
	if !trusted(owner) || !protected || len(entries) != 3 {
		return false
	}
	flags := uint8(0)
	if directory {
		flags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !trusted(entry.sid) || seen[entry.sid] || entry.flags != flags || entry.kind != windows.ACCESS_ALLOWED_ACE_TYPE || entry.mask != portalFileAllAccess {
			return false
		}
		seen[entry.sid] = true
	}
	return seen[user] && seen["S-1-5-18"] && seen["S-1-5-32-544"]
}

func verifyPortalHandle(handle windows.Handle, directory bool, user string) error {
	var info windows.ByHandleFileInformation
	kind, err := windows.GetFileType(handle)
	if err != nil || kind != windows.FILE_TYPE_DISK || windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		return ErrStore
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return ErrStore
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return ErrStore
	}
	control, _, err := sd.Control()
	if err != nil {
		return ErrStore
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil || acl.AceCount != 3 {
		return ErrStore
	}
	entries := make([]privateACE, 0, 3)
	for index := uint32(0); index < 3; index++ {
		var entry *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, index, &entry) != nil || entry == nil || entry.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return ErrStore
		}
		entries = append(entries, privateACE{sid: (*windows.SID)(unsafe.Pointer(&entry.SidStart)).String(), flags: entry.Header.AceFlags, kind: entry.Header.AceType, mask: uint32(entry.Mask)})
	}
	if !portalPrivateACL(owner.String(), user, control&windows.SE_DACL_PROTECTED != 0, directory, entries) {
		return ErrStore
	}
	return nil
}

func openNativeStore(identity userIdentity, mode storeMode) (_ store, result error) {
	if mode > storeCreate || filepath.Base(identity.directory) != "LocationPortal" || identity.sid == "" {
		return nil, ErrStore
	}
	// localstate validates fixed local namespace/mapping before any path I/O.
	parentPath := filepath.Dir(identity.directory)
	release, err := localstate.AcquireDirectory(parentPath, mode == storeCreate)
	if err != nil {
		return nil, portalFileError(err)
	}
	s := &nativeStore{release: release, owner: identity.sid, mode: mode}
	defer func() {
		if result != nil {
			s.close()
		}
	}()
	name, err := windows.UTF16PtrFromString(parentPath)
	if err != nil {
		return nil, ErrStore
	}
	parent, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.SYNCHRONIZE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, portalFileError(err)
	}
	s.parent = os.NewFile(uintptr(parent), "ultra-portal-fixed-parent")
	var info windows.ByHandleFileInformation
	var final [32768]uint16
	length, pathErr := windows.GetFinalPathNameByHandle(parent, &final[0], uint32(len(final)), 0)
	if windows.GetFileInformationByHandle(parent, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || pathErr != nil || length == 0 || length >= uint32(len(final)) || !strings.EqualFold(strings.TrimPrefix(windows.UTF16ToString(final[:length]), `\\?\`), parentPath) {
		return nil, ErrStore
	}
	s.directory, err = portalOpen(s.parent, "LocationPortal", s.owner, true, mode != storeReadOnly, windows.FILE_OPEN)
	if errors.Is(err, os.ErrNotExist) && mode == storeCreate {
		s.directory, err = portalOpen(s.parent, "LocationPortal", s.owner, true, true, windows.FILE_CREATE)
	}
	if err != nil {
		return nil, err
	}
	disposition := uint32(windows.FILE_OPEN)
	if mode != storeReadOnly {
		disposition = windows.FILE_OPEN_IF
	}
	// Lock contents are never changed/deleted. READ|WRITE sharing permits all
	// cooperating instances to reach the same kernel byte-range lock object.
	s.lock, err = portalOpenWithPolicy(s.directory, "portal.lock", s.owner, false, portalLockPolicy(disposition))
	if errors.Is(err, os.ErrNotExist) && mode == storeReadOnly {
		file, readErr := portalOpen(s.directory, journalName, s.owner, false, false, windows.FILE_OPEN)
		if file != nil {
			file.Close()
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return nil, ErrStore
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	stat, err := s.lock.Stat()
	if err != nil || stat.Size() != 0 {
		return nil, ErrStore
	}
	if windows.LockFileEx(windows.Handle(s.lock.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &s.overlap) != nil {
		return nil, ErrBusy
	}
	s.locked = true
	return s, nil
}

func (s *nativeStore) bytes() ([]byte, error) {
	file, err := portalOpen(s.directory, journalName, s.owner, false, false, windows.FILE_OPEN)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxJournal+1))
	if err != nil || len(data) > maxJournal {
		return nil, ErrStore
	}
	return data, nil
}
func (s *nativeStore) read() ([]byte, error) {
	data, err := s.bytes()
	if err == nil || errors.Is(err, os.ErrNotExist) {
		s.known, s.expected = true, append([]byte(nil), data...)
	}
	return data, err
}

type journalWriter interface {
	Write([]byte) (int, error)
	Sync() error
}

func writeJournalContent(file journalWriter, data []byte) error {
	count, err := file.Write(data)
	if err != nil || count != len(data) || file.Sync() != nil {
		return ErrStore
	}
	return nil
}

type portalRename struct {
	Flags  uint32
	Root   windows.Handle
	Length uint32
	Name   [1]uint16
}

func portalRenameBuffer(root windows.Handle, replace bool) ([]byte, error) {
	if root == 0 || root == windows.InvalidHandle {
		return nil, ErrStore
	}
	name, err := windows.UTF16FromString(journalName)
	if err != nil {
		return nil, ErrStore
	}
	var header portalRename
	offset := int(unsafe.Offsetof(header.Name))
	buffer := make([]byte, offset+len(name)*2)
	value := (*portalRename)(unsafe.Pointer(&buffer[0]))
	value.Root, value.Length = root, uint32((len(name)-1)*2)
	if replace {
		value.Flags = 1
	}
	for index, unit := range name {
		binary.LittleEndian.PutUint16(buffer[offset+index*2:], unit)
	}
	return buffer, nil
}
func (s *nativeStore) save(data []byte) error {
	if s.mode == storeReadOnly || !s.locked || !s.known || len(data) == 0 || len(data) > maxJournal {
		return ErrStore
	}
	actual, err := s.bytes()
	if err != nil && !errors.Is(err, os.ErrNotExist) || !bytes.Equal(actual, s.expected) {
		return ErrConflict
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ErrStore
	}
	file, err := portalOpen(s.directory, ".portal-"+hex.EncodeToString(nonce[:])+".pending", s.owner, false, true, windows.FILE_CREATE)
	if err != nil {
		return err
	}
	renameAttempted, closed := false, false
	defer func() {
		if !renameAttempted && !closed {
			flag := byte(1)
			_ = windows.SetFileInformationByHandle(windows.Handle(file.Fd()), windows.FileDispositionInfo, &flag, 1)
		}
		if !closed {
			file.Close()
		}
	}()
	if writeJournalContent(file, data) != nil {
		return ErrStore
	}
	rename, err := portalRenameBuffer(windows.Handle(s.directory.Fd()), len(s.expected) != 0)
	if err != nil {
		return err
	}
	renameAttempted = true
	if windows.SetFileInformationByHandle(windows.Handle(file.Fd()), windows.FileRenameInfo, &rename[0], uint32(len(rename))) != nil || file.Sync() != nil {
		return ErrStore
	}
	closeErr := file.Close()
	closed = true
	if closeErr != nil {
		return ErrStore
	}
	actual, err = s.bytes()
	if err != nil || !bytes.Equal(actual, data) {
		return ErrStore
	}
	s.expected = append([]byte(nil), data...)
	return nil
}
func (s *nativeStore) close() {
	s.once.Do(func() {
		if s.lock != nil {
			if s.locked {
				_ = windows.UnlockFileEx(windows.Handle(s.lock.Fd()), 0, 1, 0, &s.overlap)
			}
			s.lock.Close()
		}
		if s.directory != nil {
			s.directory.Close()
		}
		if s.parent != nil {
			s.parent.Close()
		}
		if s.release != nil {
			s.release()
		}
	})
}
