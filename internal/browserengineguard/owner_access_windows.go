//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ownerAccessMask = windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_DUP_HANDLE | windows.SYNCHRONIZE

type ownerTokenCore struct {
	sid                                     string
	id, authentication, modified            windows.LUID
	session, integrity, kind, impersonation uint32
	elevated                                bool
}
type ownerProcessIdentity struct {
	pid     uint32
	created uint64
	image   string
	token   ownerTokenCore
	alive   bool
}
type ownerAccessBackend interface {
	snapshot(windows.Handle) (ownerProcessIdentity, error)
	open(uint32, uint32) (windows.Handle, error)
	privilegedOpen(windows.Handle, ownerProcessIdentity) (windows.Handle, error)
	close(windows.Handle) error
}

func validOwnerProcess(v ownerProcessIdentity) bool {
	return v.pid != 0 && v.created != 0 && v.alive && v.token.sid != "" && !serviceOwnerSID(v.token.sid) &&
		v.token.session != 0 && !v.token.elevated && v.token.integrity == 0x2000 && v.token.kind == windows.TokenPrimary &&
		strings.EqualFold(filepath.Base(v.image), "godusevpn.exe") && verifyLocalDirectory(v.image, func(string) error { return nil }) == nil
}
func sameOwnerProcess(a, b ownerProcessIdentity) bool {
	return a.pid == b.pid && a.created == b.created && a.token == b.token && a.alive && b.alive && strings.EqualFold(a.image, b.image)
}
func serviceOwnerSID(s string) bool { return s == "S-1-5-18" || s == "S-1-5-19" || s == "S-1-5-20" }

// The only public entry is New(heldAuthenticatedParent). No PID, image, SID,
// access mask, token or callback can be supplied by an IPC/frontend request.
func openAuthenticatedOwner(parent windows.Handle, backend ownerAccessBackend) (windows.Handle, error) {
	before, err := backend.snapshot(parent)
	if err != nil || !validOwnerProcess(before) {
		return 0, refusal("owner_process_identity", err)
	}
	opened, err := backend.open(before.pid, ownerAccessMask)
	if err != nil {
		if opened != 0 {
			_ = backend.close(opened)
		}
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return 0, refusal("owner_process_open", err)
		}
		opened, err = backend.privilegedOpen(parent, before)
		if err != nil {
			if opened != 0 {
				_ = backend.close(opened)
			}
			return 0, err
		}
	}
	if opened == 0 || opened == windows.InvalidHandle {
		return 0, refusal("owner_process_open", nil)
	}
	afterHeld, e1 := backend.snapshot(parent)
	afterNew, e2 := backend.snapshot(opened)
	if e1 != nil || e2 != nil || !sameOwnerProcess(before, afterHeld) || !sameOwnerProcess(before, afterNew) {
		_ = backend.close(opened)
		if e1 == nil {
			e1 = e2
		}
		return 0, refusal("owner_process_identity", e1)
	}
	return opened, nil
}

type nativeOwnerAccess struct{}

func (nativeOwnerAccess) open(pid, access uint32) (windows.Handle, error) {
	return windows.OpenProcess(access, false, pid)
}
func (nativeOwnerAccess) close(h windows.Handle) error { return windows.CloseHandle(h) }
func (nativeOwnerAccess) privilegedOpen(parent windows.Handle, before ownerProcessIdentity) (windows.Handle, error) {
	current, err := (nativeOwnerAccess{}).snapshot(parent)
	if err != nil || !sameOwnerProcess(before, current) {
		return 0, refusal("owner_debug_identity", err)
	}
	return runOwnerDebugOpen(before, nativeOwnerDebug{})
}
func (nativeOwnerAccess) snapshot(h windows.Handle) (ownerProcessIdentity, error) {
	var result ownerProcessIdentity
	var err error
	result.pid, err = windows.GetProcessId(h)
	if err != nil || result.pid == windows.GetCurrentProcessId() {
		return result, refusal("owner_process_id", err)
	}
	var creation, exit, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return result, refusal("owner_process_times", err)
	}
	result.created = uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime)
	wait, err := windows.WaitForSingleObject(h, 0)
	if err != nil || wait != uint32(windows.WAIT_TIMEOUT) {
		return result, refusal("owner_process_identity", err)
	}
	result.alive = true
	result.image, err = ownerImage(h)
	if err != nil {
		return result, refusal("owner_process_identity", err)
	}
	self, err := ownerImage(windows.CurrentProcess())
	if err != nil || !strings.EqualFold(result.image, self) {
		return result, refusal("owner_process_identity", err)
	}
	var token windows.Token
	if err = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return result, refusal("owner_token_open", err)
	}
	defer token.Close()
	result.token, err = readOwnerTokenCore(token)
	if err != nil {
		return result, refusal("owner_token_identity", err)
	}
	var session uint32
	if err = windows.ProcessIdToSessionId(result.pid, &session); err != nil || session != result.token.session {
		return result, refusal("owner_process_identity", err)
	}
	return result, nil
}
func ownerImage(h windows.Handle) (string, error) {
	data := make([]uint16, 32768)
	n := uint32(len(data))
	if err := windows.QueryFullProcessImageName(h, 0, &data[0], &n); err != nil {
		return "", err
	}
	if n == 0 || n >= uint32(len(data)) {
		return "", ErrRefused
	}
	return windows.UTF16ToString(data[:n]), nil
}

type ownerTokenStatistics struct {
	id, authentication                                          windows.LUID
	expires                                                     int64
	kind, impersonation, charged, available, groups, privileges uint32
	modified                                                    windows.LUID
}

func ownerTokenUint(t windows.Token, class uint32) (uint32, error) {
	var value, used uint32
	err := windows.GetTokenInformation(t, class, (*byte)(unsafe.Pointer(&value)), 4, &used)
	if err != nil {
		return 0, err
	}
	if used != 4 {
		return 0, ErrRefused
	}
	return value, nil
}
func ownerTokenBytes(t windows.Token, class uint32) ([]byte, error) {
	size := uint32(4096)
	for attempt := 0; attempt < 2; attempt++ {
		data := make([]byte, size)
		used := size
		err := windows.GetTokenInformation(t, class, &data[0], size, &used)
		if err == nil {
			if used == 0 || used > size {
				return nil, ErrRefused
			}
			return data[:used], nil
		}
		if attempt != 0 || (!errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) && !errors.Is(err, windows.ERROR_BAD_LENGTH)) || used <= size || used > 1<<20 {
			return nil, err
		}
		size = used
	}
	return nil, ErrRefused
}
func readOwnerTokenCore(t windows.Token) (ownerTokenCore, error) {
	var c ownerTokenCore
	user, err := t.GetTokenUser()
	if err != nil {
		return c, err
	}
	if user.User.Sid == nil || !user.User.Sid.IsValid() {
		return c, ErrRefused
	}
	c.sid = user.User.Sid.String()
	var stats ownerTokenStatistics
	var used uint32
	err = windows.GetTokenInformation(t, windows.TokenStatistics, (*byte)(unsafe.Pointer(&stats)), uint32(unsafe.Sizeof(stats)), &used)
	if err != nil {
		return c, err
	}
	if used != 56 || unsafe.Sizeof(stats) != 56 {
		return c, ErrRefused
	}
	c.id, c.authentication, c.modified, c.kind, c.impersonation = stats.id, stats.authentication, stats.modified, stats.kind, stats.impersonation
	c.session, err = ownerTokenUint(t, windows.TokenSessionId)
	if err != nil {
		return c, err
	}
	elevated, err := ownerTokenUint(t, windows.TokenElevation)
	if err != nil {
		return c, err
	}
	c.elevated = elevated != 0
	data, err := ownerTokenBytes(t, windows.TokenIntegrityLevel)
	if err != nil {
		return c, err
	}
	if len(data) < int(unsafe.Sizeof(windows.Tokenmandatorylabel{})) {
		return c, ErrRefused
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&data[0]))
	sid := label.Label.Sid
	start := uintptr(unsafe.Pointer(&data[0]))
	p := uintptr(unsafe.Pointer(sid))
	if p < start || p-start > uintptr(len(data))-8 {
		return c, ErrRefused
	}
	count := sid.SubAuthorityCount()
	length := uintptr(8 + 4*uint32(count))
	if count == 0 || count > 15 || length > uintptr(len(data))-(p-start) || !sid.IsValid() {
		return c, ErrRefused
	}
	c.integrity = sid.SubAuthority(uint32(count) - 1)
	runtime.KeepAlive(data)
	return c, nil
}
func readOwnerPrivileges(t windows.Token) (map[windows.LUID]uint32, error) {
	data, err := ownerTokenBytes(t, windows.TokenPrivileges)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, ErrRefused
	}
	count := binary.LittleEndian.Uint32(data[:4])
	if count > 256 || uint64(count)*12+4 != uint64(len(data)) {
		return nil, ErrRefused
	}
	result := make(map[windows.LUID]uint32, count)
	for i := uint32(0); i < count; i++ {
		p := data[4+i*12:]
		id := windows.LUID{LowPart: binary.LittleEndian.Uint32(p), HighPart: int32(binary.LittleEndian.Uint32(p[4:]))}
		if _, exists := result[id]; exists {
			return nil, ErrRefused
		}
		result[id] = binary.LittleEndian.Uint32(p[8:])
	}
	return result, nil
}
