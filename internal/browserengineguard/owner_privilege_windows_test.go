//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every method is in-memory. No test calls New, nativeOwnerAccess or a native
// token, process, privilege, thread, WFP or filesystem operation.
type ownerDebugFake struct {
	log                                                                          []string
	errs                                                                         map[string]error
	panicAt                                                                      string
	source, copy                                                                 ownerTokenCore
	before, copied                                                               map[windows.LUID]uint32
	enabled, attached, unlocked, retained                                        bool
	initialToken, initialError, finalToken, activeWrong, mutateSource, badEnable bool
	wait                                                                         uint32
	opened                                                                       windows.Handle
	openPID, openMask                                                            uint32
	closes                                                                       map[windows.Handle]int
}

var fakeDebug = windows.LUID{LowPart: 20}
var fakeOther = windows.LUID{LowPart: 23}

func fakeOwnerIdentity() ownerProcessIdentity {
	return ownerProcessIdentity{pid: 42, created: 100, image: `C:\Program Files\godusevpn ultra\godusevpn.exe`, alive: true,
		token: ownerTokenCore{sid: "S-1-5-21-42", id: windows.LUID{LowPart: 100}, authentication: windows.LUID{LowPart: 101}, modified: windows.LUID{LowPart: 102}, session: 1, integrity: 0x2000, kind: windows.TokenPrimary}}
}
func newOwnerDebugFake() *ownerDebugFake {
	source := ownerTokenCore{sid: "S-1-5-21-99", id: windows.LUID{LowPart: 1}, authentication: windows.LUID{LowPart: 2}, modified: windows.LUID{LowPart: 3}, session: 1, integrity: 0x3000, kind: windows.TokenPrimary, elevated: true}
	copy := source
	copy.id.LowPart = 4
	copy.modified.LowPart = 5
	copy.kind = windows.TokenImpersonation
	copy.impersonation = windows.SecurityImpersonation
	return &ownerDebugFake{errs: map[string]error{}, source: source, copy: copy, before: map[windows.LUID]uint32{fakeDebug: 0, fakeOther: windows.SE_PRIVILEGE_ENABLED}, wait: uint32(windows.WAIT_OBJECT_0), opened: 50, closes: map[windows.Handle]int{}}
}
func (f *ownerDebugFake) step(s string) error {
	f.log = append(f.log, s)
	if f.panicAt == s {
		panic("fake panic")
	}
	return f.errs[s]
}
func cloneFakePrivileges(p map[windows.LUID]uint32) map[windows.LUID]uint32 {
	r := map[windows.LUID]uint32{}
	for k, v := range p {
		r[k] = v
	}
	return r
}
func (f *ownerDebugFake) lockThread()                           { _ = f.step("lock") }
func (f *ownerDebugFake) unlockThread()                         { _ = f.step("unlock"); f.unlocked = true }
func (f *ownerDebugFake) threadHandle() (windows.Handle, error) { return 10, f.step("thread") }
func (f *ownerDebugFake) threadToken() (windows.Token, error) {
	if !slices.Contains(f.log, "initial") {
		err := f.step("initial")
		if err != nil {
			return 0, err
		}
		if f.initialToken {
			return 60, nil
		}
		if f.initialError {
			return 0, windows.ERROR_ACCESS_DENIED
		}
		return 0, windows.ERROR_NO_TOKEN
	}
	if f.attached {
		err := f.step("active")
		if err != nil {
			return 0, err
		}
		return 40, nil
	}
	err := f.step("final")
	if err != nil {
		return 0, err
	}
	if f.finalToken {
		return 61, nil
	}
	return 0, windows.ERROR_NO_TOKEN
}
func (f *ownerDebugFake) ownToken() (windows.Token, error) { return 20, f.step("own") }
func (f *ownerDebugFake) duplicateToken(windows.Token) (windows.Token, error) {
	err := f.step("duplicate")
	f.copied = cloneFakePrivileges(f.before)
	return 30, err
}
func (f *ownerDebugFake) tokenCore(t windows.Token) (ownerTokenCore, error) {
	if t == 20 {
		err := f.step("sourceCore")
		v := f.source
		if f.mutateSource && f.enabled {
			v.modified.LowPart++
		}
		return v, err
	}
	v := f.copy
	if f.enabled {
		v.modified.LowPart++
	}
	if t == 40 && f.activeWrong {
		v.authentication.LowPart++
	}
	return v, f.step(fmt.Sprintf("core%d", t))
}
func (f *ownerDebugFake) privileges(t windows.Token) (map[windows.LUID]uint32, error) {
	if t == 20 {
		return cloneFakePrivileges(f.before), f.step("sourcePrivileges")
	}
	return cloneFakePrivileges(f.copied), f.step(fmt.Sprintf("privileges%d", t))
}
func (f *ownerDebugFake) debugPrivilege() (windows.LUID, error) { return fakeDebug, f.step("debug") }
func (f *ownerDebugFake) enablePrivilege(t windows.Token, id windows.LUID) error {
	if t != 30 {
		panic("adjusted original token")
	}
	err := f.step("enable")
	if err != nil {
		return err
	}
	f.enabled = true
	f.copied[id] |= windows.SE_PRIVILEGE_ENABLED
	if f.badEnable {
		f.copied[fakeOther] = 0
	}
	return nil
}
func (f *ownerDebugFake) attach(t windows.Token) error {
	if t != 30 {
		panic("wrong attach")
	}
	f.attached = true
	return f.step("attach")
}
func (f *ownerDebugFake) revert() error {
	err := f.step("revert")
	if err == nil {
		f.attached = false
	}
	return err
}
func (f *ownerDebugFake) open(pid, mask uint32) (windows.Handle, error) {
	if !f.attached || !f.enabled {
		panic("opened before proof")
	}
	f.openPID, f.openMask = pid, mask
	return f.opened, f.step("open")
}
func (f *ownerDebugFake) close(h windows.Handle) error {
	f.closes[h]++
	return f.step(fmt.Sprintf("close%d", h))
}
func (f *ownerDebugFake) waitThread(windows.Handle) (uint32, error) { return f.wait, f.step("wait") }
func (f *ownerDebugFake) retainThread(windows.Handle)               { _ = f.step("retain"); f.retained = true }

func TestOwnerDebugSuccessOrderAndNoGlobalTokenMutation(t *testing.T) {
	f := newOwnerDebugFake()
	source := f.source
	privileges := cloneFakePrivileges(f.before)
	h, err := runOwnerDebugOpen(fakeOwnerIdentity(), f)
	if err != nil || h != 50 {
		t.Fatalf("result %v %v log %v", h, err, f.log)
	}
	want := []string{"lock", "thread", "initial", "own", "sourceCore", "sourcePrivileges", "duplicate", "core30", "privileges30", "debug", "enable", "privileges30", "core30", "attach", "active", "core40", "privileges40", "close40", "open", "revert", "final", "sourceCore", "sourcePrivileges", "close30", "close20", "unlock", "close10"}
	if !reflect.DeepEqual(f.log, want) {
		t.Fatalf("unsafe order:\n%v\nwant %v", f.log, want)
	}
	if f.openPID != 42 || f.openMask != ownerAccessMask || !f.unlocked || f.retained || f.attached || f.source != source || !reflect.DeepEqual(f.before, privileges) {
		t.Fatal("scope changed")
	}
	for _, h := range []windows.Handle{10, 20, 30, 40} {
		if f.closes[h] != 1 {
			t.Fatalf("handle %d closes %d", h, f.closes[h])
		}
	}
	if f.closes[50] != 0 {
		t.Fatal("successful parent closed")
	}
}

func TestOwnerDebugRefusesFailureMatrix(t *testing.T) {
	cases := []struct {
		name, stage string
		setup       func(*ownerDebugFake)
		noUnlock    bool
	}{
		{"existing_thread_token", "owner_debug_thread", func(f *ownerDebugFake) { f.initialToken = true }, true},
		{"unknown_initial_token", "owner_debug_thread", func(f *ownerDebugFake) { f.initialError = true }, true},
		{"source_open", "owner_debug_token", func(f *ownerDebugFake) { f.errs["own"] = windows.ERROR_ACCESS_DENIED }, false},
		{"source_read", "owner_debug_identity", func(f *ownerDebugFake) { f.errs["sourceCore"] = windows.ERROR_ACCESS_DENIED }, false},
		{"source_not_high", "owner_debug_identity", func(f *ownerDebugFake) { f.source.integrity = 0x2000 }, false},
		{"source_other_session", "owner_debug_identity", func(f *ownerDebugFake) { f.source.session = 2 }, false},
		{"source_system", "owner_debug_identity", func(f *ownerDebugFake) { f.source.sid = "S-1-5-18" }, false},
		{"source_privileges", "owner_debug_token", func(f *ownerDebugFake) { f.errs["sourcePrivileges"] = windows.ERROR_ACCESS_DENIED }, false},
		{"duplicate", "owner_debug_duplicate", func(f *ownerDebugFake) { f.errs["duplicate"] = windows.ERROR_ACCESS_DENIED }, false},
		{"copy_authentication", "owner_debug_identity", func(f *ownerDebugFake) { f.copy.authentication.LowPart++ }, false},
		{"copy_sid", "owner_debug_identity", func(f *ownerDebugFake) { f.copy.sid = "S-1-5-21-404" }, false},
		{"copy_not_impersonation", "owner_debug_identity", func(f *ownerDebugFake) { f.copy.kind = windows.TokenPrimary }, false},
		{"copy_level", "owner_debug_identity", func(f *ownerDebugFake) { f.copy.impersonation = windows.SecurityIdentification }, false},
		{"lookup", "owner_debug_enable", func(f *ownerDebugFake) { f.errs["debug"] = windows.ERROR_ACCESS_DENIED }, false},
		{"absent_debug", "owner_debug_enable", func(f *ownerDebugFake) { delete(f.before, fakeDebug) }, false},
		{"not_all_assigned", "owner_debug_enable", func(f *ownerDebugFake) { f.errs["enable"] = windows.ERROR_NOT_ALL_ASSIGNED }, false},
		{"changed_other_privilege", "owner_debug_enable", func(f *ownerDebugFake) { f.badEnable = true }, false},
		{"partial_attach", "owner_debug_attach", func(f *ownerDebugFake) { f.errs["attach"] = windows.ERROR_ACCESS_DENIED }, false},
		{"active_read", "owner_debug_attach", func(f *ownerDebugFake) { f.errs["active"] = windows.ERROR_ACCESS_DENIED }, false},
		{"active_wrong_identity", "owner_debug_attach", func(f *ownerDebugFake) { f.activeWrong = true }, false},
		{"open_denied", "owner_debug_open", func(f *ownerDebugFake) { f.errs["open"] = windows.ERROR_ACCESS_DENIED }, false},
		{"revert_failed", "owner_debug_revert", func(f *ownerDebugFake) { f.errs["revert"] = windows.ERROR_ACCESS_DENIED }, true},
		{"final_token_present", "owner_debug_revert", func(f *ownerDebugFake) { f.finalToken = true }, true},
		{"final_token_query_failed", "owner_debug_revert", func(f *ownerDebugFake) { f.errs["final"] = windows.ERROR_ACCESS_DENIED }, true},
		{"global_identity_changed", "owner_debug_identity", func(f *ownerDebugFake) { f.mutateSource = true }, false},
		{"copy_close_failed", "owner_debug_token", func(f *ownerDebugFake) { f.errs["close30"] = windows.ERROR_INVALID_HANDLE }, false},
		{"body_panic_before_attach", "owner_debug_thread", func(f *ownerDebugFake) { f.panicAt = "enable" }, false},
		{"body_panic_after_attach", "owner_debug_thread", func(f *ownerDebugFake) { f.panicAt = "open" }, false},
		{"cleanup_panic", "owner_debug_revert", func(f *ownerDebugFake) { f.panicAt = "revert" }, true},
		{"copy_close_panic", "owner_debug_token", func(f *ownerDebugFake) { f.panicAt = "close30" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerDebugFake()
			tc.setup(f)
			done := make(chan struct{})
			var h windows.Handle
			var err error
			go func() { h, err = runOwnerDebugOpen(fakeOwnerIdentity(), f); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("worker failed to notify")
			}
			d := DiagnosticFromError(err)
			if h != 0 || d == nil || d.Stage != tc.stage {
				t.Fatalf("handle %d diagnostic %#v log %v", h, d, f.log)
			}
			if f.unlocked == tc.noUnlock {
				t.Fatalf("unsafe unlock=%v log %v", f.unlocked, f.log)
			}
			if tc.noUnlock && !slices.Contains(f.log, "wait") {
				t.Fatal("dirty thread termination not observed")
			}
			if f.closes[50] > 1 {
				t.Fatal("candidate closed repeatedly")
			}
			if slices.Contains(f.log, "attach") && !slices.Contains(f.log, "revert") {
				t.Fatal("no revert after attempted attach")
			}
			if slices.Contains(f.log, "duplicate") && f.closes[20] != 1 {
				t.Fatal("cleanup did not close source token")
			}
		})
	}
}

func TestOwnerDebugRetirementUnknownKeepsThreadAndRefusesCapability(t *testing.T) {
	for _, tc := range []struct {
		name string
		wait uint32
		err  error
		code uint32
	}{
		{"timeout", uint32(windows.WAIT_TIMEOUT), nil, 258},
		{"failed", uint32(windows.WAIT_FAILED), windows.ERROR_ACCESS_DENIED, 5},
		{"unexpected", 17, nil, 17},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerDebugFake()
			f.errs["revert"] = windows.ERROR_ACCESS_DENIED
			f.wait = tc.wait
			f.errs["wait"] = tc.err
			h, err := runOwnerDebugOpen(fakeOwnerIdentity(), f)
			d := DiagnosticFromError(err)
			if h != 0 || d == nil || d.Stage != "owner_debug_retire" || d.NativeCode != tc.code || !f.retained || f.unlocked || f.closes[10] != 0 || f.closes[50] != 1 {
				t.Fatalf("unsafe result h%d d%#v log%v", h, d, f.log)
			}
		})
	}
}

func TestOwnerDebugRevertFailurePreservesOriginalNumericEvidence(t *testing.T) {
	f := newOwnerDebugFake()
	f.errs["revert"] = windows.ERROR_ACCESS_DENIED
	f.errs["close30"] = windows.ERROR_INVALID_HANDLE
	h, err := runOwnerDebugOpen(fakeOwnerIdentity(), f)
	d := DiagnosticFromError(err)
	if h != 0 || d == nil || d.Stage != "owner_debug_revert" || d.NativeCode != 5 || f.unlocked || f.closes[50] != 1 {
		t.Fatalf("original revert code lost: %#v log %v", d, f.log)
	}
}

func TestOwnerPrivilegeAdjustmentRequiresBOOLAndSameCallSuccessCode(t *testing.T) {
	if ownerAccessMask != windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_DUP_HANDLE|windows.SYNCHRONIZE {
		t.Fatal("parent access mask widened")
	}
	for _, tc := range []struct {
		r   uintptr
		err error
		ok  bool
	}{{1, syscall.Errno(0), true}, {1, nil, true}, {0, syscall.Errno(0), false}, {0, windows.ERROR_ACCESS_DENIED, false}, {1, windows.ERROR_NOT_ALL_ASSIGNED, false}, {1, windows.ERROR_ACCESS_DENIED, false}} {
		if got := ownerPrivilegeAdjustmentResult(tc.r, tc.err); (got == nil) != tc.ok {
			t.Fatalf("BOOL %d code %v => %v", tc.r, tc.err, got)
		}
	}
	if unsafe.Sizeof(ownerTokenStatistics{}) != 56 {
		t.Fatal("TokenStatistics ABI changed")
	}
}

type ownerAccessFake struct {
	value                   ownerProcessIdentity
	err                     error
	retryErr                error
	retry, opens            int
	partial                 windows.Handle
	changedHeld, changedNew bool
	closed                  []windows.Handle
	masks                   []uint32
}

func (f *ownerAccessFake) snapshot(h windows.Handle) (ownerProcessIdentity, error) {
	v := f.value
	if f.opens > 0 && ((h == 1 && f.changedHeld) || (h != 1 && f.changedNew)) {
		v.created++
	}
	return v, nil
}
func (f *ownerAccessFake) open(pid, mask uint32) (windows.Handle, error) {
	f.opens++
	f.masks = append(f.masks, mask)
	if pid != f.value.pid {
		panic("wrong PID")
	}
	if f.err != nil {
		return f.partial, f.err
	}
	return 2, nil
}
func (f *ownerAccessFake) privilegedOpen(parent windows.Handle, v ownerProcessIdentity) (windows.Handle, error) {
	f.retry++
	if parent != 1 || v != f.value {
		panic("unbound retry")
	}
	return 3, f.retryErr
}
func (f *ownerAccessFake) close(h windows.Handle) error { f.closed = append(f.closed, h); return nil }

func TestOwnerAccessOnlyAccessDeniedAllowsOneSameMaskRetry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		retryErr  error
		held, new bool
		retry     int
		ok        bool
	}{
		{"normal", nil, nil, false, false, 0, true}, {"denied", windows.ERROR_ACCESS_DENIED, nil, false, false, 1, true},
		{"other_error", windows.ERROR_INVALID_PARAMETER, nil, false, false, 0, false},
		{"retry_error", windows.ERROR_ACCESS_DENIED, refusal("owner_debug_open", windows.ERROR_ACCESS_DENIED), false, false, 1, false},
		{"held_reused", windows.ERROR_ACCESS_DENIED, nil, true, false, 1, false}, {"candidate_changed", nil, nil, false, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &ownerAccessFake{value: fakeOwnerIdentity(), err: tc.err, retryErr: tc.retryErr, changedHeld: tc.held, changedNew: tc.new}
			h, err := openAuthenticatedOwner(1, f)
			if (err == nil) != tc.ok || f.retry != tc.retry || f.opens != 1 || !reflect.DeepEqual(f.masks, []uint32{ownerAccessMask}) {
				t.Fatalf("h%d err%v fake%#v", h, err, f)
			}
			if slices.Contains(f.closed, windows.Handle(1)) {
				t.Fatal("caller-held authenticated handle closed")
			}
			if !tc.ok && h != 0 {
				t.Fatal("failure exported capability")
			}
		})
	}
	f := &ownerAccessFake{value: fakeOwnerIdentity()}
	f.value.token.elevated = true
	if h, err := openAuthenticatedOwner(1, f); h != 0 || err == nil || f.opens != 0 || f.retry != 0 {
		t.Fatal("invalid parent attempted open")
	}
}

func TestOwnerAccessClosesPartialResultsWithoutClosingAuthenticatedParent(t *testing.T) {
	for _, code := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_INVALID_PARAMETER} {
		f := &ownerAccessFake{value: fakeOwnerIdentity(), err: code, partial: 9}
		h, err := openAuthenticatedOwner(1, f)
		if !reflect.DeepEqual(f.closed, []windows.Handle{9}) {
			t.Fatalf("partial result not closed once: %v", f.closed)
		}
		if errors.Is(code, windows.ERROR_ACCESS_DENIED) {
			if h != 3 || err != nil || f.retry != 1 {
				t.Fatal("single retry not isolated")
			}
		} else if h != 0 || err == nil || f.retry != 0 {
			t.Fatal("non-access-denied retried")
		}
	}
	f := &ownerAccessFake{value: fakeOwnerIdentity(), err: windows.ERROR_ACCESS_DENIED, retryErr: refusal("owner_debug_open", windows.ERROR_ACCESS_DENIED)}
	if h, err := openAuthenticatedOwner(1, f); h != 0 || err == nil || !reflect.DeepEqual(f.closed, []windows.Handle{3}) {
		t.Fatal("partial retry leaked a process handle")
	}
}

func TestOwnerDebugConcurrentAttemptsHaveIndependentTokens(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 48; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := newOwnerDebugFake()
			if i%3 == 0 {
				f.errs["attach"] = windows.ERROR_ACCESS_DENIED
			}
			h, err := runOwnerDebugOpen(fakeOwnerIdentity(), f)
			if i%3 == 0 {
				if h != 0 || err == nil {
					t.Error("failed attempt exported handle")
				}
			} else if h != 50 || err != nil {
				t.Error("independent attempt failed")
			}
			if !f.unlocked || f.attached {
				t.Error("token leaked")
			}
		}(i)
	}
	wg.Wait()
}

func TestOwnerProcessIdentityRejectsChanges(t *testing.T) {
	before := fakeOwnerIdentity()
	for _, change := range []func(*ownerProcessIdentity){func(v *ownerProcessIdentity) { v.pid++ }, func(v *ownerProcessIdentity) { v.created++ }, func(v *ownerProcessIdentity) { v.image = `C:\Other\godusevpn.exe` }, func(v *ownerProcessIdentity) { v.token.sid += "x" }, func(v *ownerProcessIdentity) { v.token.id.LowPart++ }, func(v *ownerProcessIdentity) { v.token.authentication.LowPart++ }, func(v *ownerProcessIdentity) { v.token.modified.LowPart++ }, func(v *ownerProcessIdentity) { v.token.session++ }, func(v *ownerProcessIdentity) { v.token.integrity++ }, func(v *ownerProcessIdentity) { v.token.elevated = true }, func(v *ownerProcessIdentity) { v.token.kind++ }, func(v *ownerProcessIdentity) { v.alive = false }} {
		after := before
		change(&after)
		if sameOwnerProcess(before, after) {
			t.Fatal("changed identity accepted")
		}
	}
	if !errors.Is(ownerPrivilegeAdjustmentResult(1, windows.ERROR_NOT_ALL_ASSIGNED), windows.ERROR_NOT_ALL_ASSIGNED) {
		t.Fatal("native numeric evidence lost")
	}
}
