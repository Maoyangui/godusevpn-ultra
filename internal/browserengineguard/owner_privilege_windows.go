//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// This interface is private and its production implementation has no callbacks
// or state supplied by an IPC client. It exists to test every failure without
// opening a process, attaching a token, or changing a privilege in tests.
type ownerDebugBackend interface {
	lockThread()
	unlockThread()
	threadHandle() (windows.Handle, error)
	threadToken() (windows.Token, error)
	ownToken() (windows.Token, error)
	duplicateToken(windows.Token) (windows.Token, error)
	tokenCore(windows.Token) (ownerTokenCore, error)
	privileges(windows.Token) (map[windows.LUID]uint32, error)
	debugPrivilege() (windows.LUID, error)
	enablePrivilege(windows.Token, windows.LUID) error
	attach(windows.Token) error
	revert() error
	open(uint32, uint32) (windows.Handle, error)
	close(windows.Handle) error
	waitThread(windows.Handle) (uint32, error)
	retainThread(windows.Handle)
}

type ownerDebugResult struct {
	process, thread windows.Handle
	dirty           bool
	err             error
}

// A dirty worker exits while still locked. Go terminates that OS thread instead
// of scheduling another goroutine on it. Before returning, the caller must also
// observe its actual termination through the independently held thread HANDLE.
func runOwnerDebugOpen(parent ownerProcessIdentity, b ownerDebugBackend) (windows.Handle, error) {
	results := make(chan ownerDebugResult, 1)
	go ownerDebugWorker(parent, b, results)
	r := <-results
	if r.dirty {
		if r.thread == 0 || r.thread == windows.InvalidHandle {
			r.err = refusal("owner_debug_retire", nil)
		} else {
			wait, err := b.waitThread(r.thread)
			if err != nil || wait != uint32(windows.WAIT_OBJECT_0) {
				if err == nil {
					err = syscall.Errno(wait)
				}
				r.err = refusal("owner_debug_retire", err)
				b.retainThread(r.thread)
				r.thread = 0
			}
		}
	}
	if r.thread != 0 {
		if err := b.close(r.thread); err != nil {
			r.err = refusal("owner_debug_retire", err)
			b.retainThread(r.thread)
		}
	}
	if r.err != nil || r.dirty {
		if r.process != 0 {
			_ = b.close(r.process)
		}
		if r.err == nil {
			r.err = refusal("owner_debug_revert", nil)
		}
		return 0, r.err
	}
	if r.process == 0 || r.process == windows.InvalidHandle {
		return 0, refusal("owner_debug_open", nil)
	}
	return r.process, nil
}

func ownerDebugWorker(parent ownerProcessIdentity, b ownerDebugBackend, results chan<- ownerDebugResult) {
	r := ownerDebugResult{dirty: true}
	var source, copy windows.Token
	var sourceCore ownerTokenCore
	var sourcePrivileges map[windows.LUID]uint32
	initiallyAbsent, attachAttempted, locked := false, false, false
	// A second, outer defer covers an unexpected panic in cleanup itself. It
	// must not unlock a thread whose token could no longer be established.
	defer func() {
		if recover() != nil {
			r.dirty = true
			r.err = refusal("owner_debug_thread", nil)
		}
		results <- r
		close(results)
	}()
	// Install before LockOSThread and before any token operation. Even a panic
	// or a partial SetThreadToken failure must reach cleanup and notify caller.
	defer func() {
		if recover() != nil {
			r.err = refusal("owner_debug_thread", nil)
		}
		var unsafeRevert error
		if attachAttempted {
			if err := ownerDebugCleanupCall(b.revert); err != nil {
				unsafeRevert = refusal("owner_debug_revert", err)
			} else {
				r.dirty = false
			}
		} else if initiallyAbsent {
			r.dirty = false
		}
		if initiallyAbsent {
			var active windows.Token
			err := ownerDebugCleanupCall(func() (err error) { active, err = b.threadToken(); return err })
			if active != 0 {
				_ = ownerDebugCleanupCall(func() error { return b.close(windows.Handle(active)) })
			}
			if !errors.Is(err, windows.ERROR_NO_TOKEN) || active != 0 {
				r.dirty = true
				if unsafeRevert == nil {
					unsafeRevert = refusal("owner_debug_revert", err)
				}
			}
		}
		// The helper's primary token is read, never adjusted. Check its complete
		// stable identity and privilege set again after detaching the copy.
		if source != 0 && sourcePrivileges != nil {
			err := ownerDebugCleanupCall(func() error {
				core, e1 := b.tokenCore(source)
				privileges, e2 := b.privileges(source)
				if e1 != nil {
					return e1
				}
				if e2 != nil {
					return e2
				}
				if core != sourceCore || !sameOwnerPrivileges(sourcePrivileges, privileges) {
					return ErrRefused
				}
				return nil
			})
			if err != nil {
				r.err = refusal("owner_debug_identity", err)
			}
		}
		for _, token := range []windows.Token{copy, source} {
			if token != 0 {
				if err := ownerDebugCleanupCall(func() error { return b.close(windows.Handle(token)) }); err != nil {
					r.err = refusal("owner_debug_token", err)
				}
			}
		}
		// Token-close or later identity failures must not erase the original
		// native revert code when the thread cannot safely return to the pool.
		if unsafeRevert != nil {
			r.err = unsafeRevert
		}
		if r.err != nil && r.process != 0 {
			_ = ownerDebugCleanupCall(func() error { return b.close(r.process) })
			r.process = 0
		}
		if locked && !r.dirty {
			b.unlockThread()
		}
	}()
	b.lockThread()
	locked = true
	var err error
	r.thread, err = b.threadHandle()
	if err != nil || r.thread == 0 || r.thread == windows.InvalidHandle {
		r.err = refusal("owner_debug_thread", err)
		return
	}
	active, err := b.threadToken()
	if active != 0 {
		_ = b.close(windows.Handle(active))
	}
	if !errors.Is(err, windows.ERROR_NO_TOKEN) || active != 0 {
		r.err = refusal("owner_debug_thread", err)
		return
	}
	initiallyAbsent = true
	source, err = b.ownToken()
	if err != nil || source == 0 {
		r.err = refusal("owner_debug_token", err)
		return
	}
	sourceCore, err = b.tokenCore(source)
	if err != nil || !validDebugSource(sourceCore, parent) {
		r.err = refusal("owner_debug_identity", err)
		return
	}
	sourcePrivileges, err = b.privileges(source)
	if err != nil {
		sourcePrivileges = nil // an errored read cannot become a cleanup baseline
		r.err = refusal("owner_debug_token", err)
		return
	}
	copy, err = b.duplicateToken(source)
	if err != nil || copy == 0 || copy == source {
		if copy == source {
			copy = 0
		} // do not close the same owned HANDLE twice
		r.err = refusal("owner_debug_duplicate", err)
		return
	}
	copyCore, err := b.tokenCore(copy)
	if err != nil || !validDebugCopy(sourceCore, copyCore) {
		r.err = refusal("owner_debug_identity", err)
		return
	}
	copyPrivileges, err := b.privileges(copy)
	if err != nil || !sameOwnerPrivileges(sourcePrivileges, copyPrivileges) {
		r.err = refusal("owner_debug_identity", err)
		return
	}
	debug, err := b.debugPrivilege()
	if err != nil {
		r.err = refusal("owner_debug_enable", err)
		return
	}
	if _, present := copyPrivileges[debug]; !present {
		r.err = refusal("owner_debug_enable", windows.ERROR_NOT_ALL_ASSIGNED)
		return
	}
	if err = b.enablePrivilege(copy, debug); err != nil {
		r.err = refusal("owner_debug_enable", err)
		return
	}
	after, err := b.privileges(copy)
	if err != nil || !onlyDebugEnabled(copyPrivileges, after, debug) {
		r.err = refusal("owner_debug_enable", err)
		return
	}
	afterCore, err := b.tokenCore(copy)
	if err != nil || !sameAdjustedDebugCopy(copyCore, afterCore) {
		r.err = refusal("owner_debug_identity", err)
		return
	}
	attachAttempted = true
	if err = b.attach(copy); err != nil {
		r.err = refusal("owner_debug_attach", err)
		return
	}
	active, err = b.threadToken()
	if err != nil || active == 0 {
		if active != 0 {
			_ = b.close(windows.Handle(active))
		}
		r.err = refusal("owner_debug_attach", err)
		return
	}
	activeCore, e1 := b.tokenCore(active)
	activePrivileges, e2 := b.privileges(active)
	closeErr := b.close(windows.Handle(active))
	if e1 != nil || e2 != nil || closeErr != nil || activeCore != afterCore || !sameOwnerPrivileges(after, activePrivileges) {
		if e1 == nil {
			e1 = e2
		}
		if e1 == nil {
			e1 = closeErr
		}
		r.err = refusal("owner_debug_attach", e1)
		return
	}
	// The single privileged target operation. Post-open target identity checks
	// happen only after cleanup has verified that this thread has no token.
	r.process, err = b.open(parent.pid, ownerAccessMask)
	if err != nil || r.process == 0 || r.process == windows.InvalidHandle {
		r.err = refusal("owner_debug_open", err)
	}
}

// Cleanup must keep running after a failed native step. Unexpected panics are
// fixed local refusals, and can never skip notification or unlock a dirty thread.
func ownerDebugCleanupCall(action func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrRefused
		}
	}()
	return action()
}

func validDebugSource(c ownerTokenCore, parent ownerProcessIdentity) bool {
	return c.sid != "" && !serviceOwnerSID(c.sid) && c.kind == windows.TokenPrimary && c.elevated && c.integrity == 0x3000 && c.session == parent.token.session
}
func validDebugCopy(source, copy ownerTokenCore) bool {
	return copy.id != source.id && copy.sid == source.sid && copy.authentication == source.authentication && copy.session == source.session &&
		copy.integrity == source.integrity && copy.elevated == source.elevated && copy.kind == windows.TokenImpersonation && copy.impersonation == windows.SecurityImpersonation
}
func sameAdjustedDebugCopy(before, after ownerTokenCore) bool {
	before.modified = after.modified
	return before == after
}
func sameOwnerPrivileges(a, b map[windows.LUID]uint32) bool {
	if a == nil || b == nil || len(a) != len(b) {
		return false
	}
	for id, value := range a {
		if other, ok := b[id]; !ok || other != value {
			return false
		}
	}
	return true
}
func onlyDebugEnabled(before, after map[windows.LUID]uint32, debug windows.LUID) bool {
	if before == nil || after == nil || len(before) != len(after) {
		return false
	}
	for id, value := range before {
		other, ok := after[id]
		if !ok {
			return false
		}
		if id == debug {
			value |= windows.SE_PRIVILEGE_ENABLED
		}
		if other != value {
			return false
		}
	}
	_, exists := before[debug]
	return exists
}
