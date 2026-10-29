//go:build windows && (amd64 || arm64)

package locationportalprepare

import (
	"context"
	"errors"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Formal signatures/state/layout:
// https://learn.microsoft.com/windows-hardware/manufacture/desktop/dism/dismaddcapability
// https://learn.microsoft.com/windows-hardware/manufacture/desktop/dism/dismremovecapability
// https://learn.microsoft.com/windows-hardware/manufacture/desktop/dism/dismgetcapabilityinfo
// https://learn.microsoft.com/windows-hardware/manufacture/desktop/dism/dismcapabilityinfo
// https://learn.microsoft.com/windows-hardware/manufacture/desktop/dism/dismpackagefeaturestate-enumeration
// https://learn.microsoft.com/windows-hardware/manufacture/desktop/dism/dismsession
// DISM_ONLINE_IMAGE and reload=1 are also corroborated in Google's maintained
// upstream wrapper (constants only; no implementation/dependency is imported):
// https://github.com/google/glazier/blob/master/go/dism/dism.go
const dismOnlineImage = "DISM_{53BFAE52-B167-4E2F-A258-0A37B57FF845}"
const dismReloadSession uint32 = 1
const dismRebootRequired uint32 = 3010
const dismRebootRequiredHRESULT uint32 = 0x80070bc2

type dismCapabilityInfo struct {
	name          *uint16
	state         uint32
	displayName   *uint16
	description   *uint16
	downloadSize  uint32
	installedSize uint32
}

// The delegates preserve each native parameter for tests, while all production
// callers below supply only fixed capability/online image, no source paths,
// callbacks, custom log/scratch paths, or frontend-selected servicing flags.
type dismFunctions struct {
	initialize func(uint32, *uint16, *uint16) uint32
	shutdown   func() uint32
	open       func(*uint16, *uint16, *uint16, *uint32) uint32
	close      func(uint32) uint32
	info       func(uint32, *uint16, **dismCapabilityInfo) uint32
	delete     func(*dismCapabilityInfo) uint32
	add        func(uint32, *uint16, bool, **uint16, uint32, uintptr, uintptr, uintptr) uint32
	remove     func(uint32, *uint16, uintptr, uintptr, uintptr) uint32
	newEvent   func() (uintptr, error)
	signal     func(uintptr) error
	closeEvent func(uintptr) error
}

func (f *dismFunctions) valid() bool {
	return f != nil && f.initialize != nil && f.shutdown != nil && f.open != nil && f.close != nil && f.info != nil && f.delete != nil && f.add != nil && f.remove != nil && f.newEvent != nil && f.signal != nil && f.closeEvent != nil
}

type dismFailure uint32

func (d dismFailure) Error() string { return "LOCATION_PORTAL_DISM_OPERATION_FAILED" }
func (d dismFailure) Unwrap() error { return syscall.Errno(uint32(d)) }
func dismHRESULT(code uint32) error {
	if code == 0 {
		return nil
	}
	return dismFailure(code)
}

// Only this coordinator owns DISM initialization in this process. A close or
// shutdown failure poisons it until the helper process exits; it never guesses
// that a foreign/already-initialized session belongs to this module.
type capabilityDISM struct {
	serial   chan struct{}
	load     func() (*dismFunctions, error)
	poisoned bool
}

func newCapabilityDISM(load func() (*dismFunctions, error)) *capabilityDISM {
	return &capabilityDISM{serial: make(chan struct{}, 1), load: load}
}

var fixedCapabilityDISM = newCapabilityDISM(loadNativeDISM)

func (d *capabilityDISM) execute(ctx context.Context, action int) (state uint32, reboot bool, err error) {
	if d == nil || d.load == nil || d.serial == nil || ctx == nil {
		return 0, false, ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	select {
	case d.serial <- struct{}{}:
		defer func() { <-d.serial }()
	case <-ctx.Done():
		return 0, false, ctx.Err()
	}
	if ctx.Err() != nil {
		return 0, false, ctx.Err()
	}
	if d.poisoned || action < -1 || action > 1 {
		return 0, false, ErrReadback
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	f, err := d.load()
	if err != nil || !f.valid() {
		return 0, false, ErrUnsupported
	}
	if ctx.Err() != nil {
		return 0, false, ctx.Err()
	}
	// A matching initialization/shutdown pair surrounds the whole serialized
	// operation, including post-mutation reopening. No callbacks run servicing.
	if code := f.initialize(0, nil, nil); code != 0 {
		return 0, false, dismHRESULT(code)
	}
	var session uint32
	canShutdown := true
	defer func() {
		if !canShutdown {
			return
		}
		if session != 0 {
			if code := f.close(session); code != 0 {
				d.poisoned = true
				err = errors.Join(err, dismHRESULT(code), ErrReadback)
				return // Never shut down a session not confirmed closed.
			}
		}
		if code := f.shutdown(); code != 0 {
			d.poisoned = true
			err = errors.Join(err, dismHRESULT(code), ErrReadback)
		}
	}()
	open := func() error {
		if code := f.open(windows.StringToUTF16Ptr(dismOnlineImage), nil, nil, &session); code != 0 {
			// A nonzero session accompanying an error is still conservatively
			// retained for the common cleanup path, never used for mutation.
			return dismHRESULT(code)
		}
		if session == 0 {
			return ErrReadback
		}
		return nil
	}
	if err = open(); err != nil {
		return 0, false, err
	}
	if ctx.Err() != nil {
		return 0, false, ctx.Err()
	}
	state, err = readFixedCapability(f, session)
	if err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		return state, false, err
	}
	if action == 0 {
		return state, false, nil
	}
	// Pending is a restart-needed result, never proof of Installed/NotPresent.
	// No further servicing is attempted against an already-pending image.
	if state == 1 || state == 5 {
		return state, true, nil
	}
	if action == 1 && state == 4 || action == -1 && (state == 0 || state == 3) {
		return state, false, nil
	}
	if action == 1 && state != 0 && state != 3 || action == -1 && state != 4 {
		return state, false, ErrConflict
	}
	if ctx.Err() != nil {
		return state, false, ctx.Err()
	}
	event, eventErr := f.newEvent()
	if eventErr != nil || event == 0 || event == ^uintptr(0) {
		if event != 0 && event != ^uintptr(0) && f.closeEvent(event) != nil {
			d.poisoned = true
		}
		return state, false, ErrReadback
	}
	// The cancelling goroutine only signals this event. It never closes the
	// image/session or reports completion while the servicing call is running.
	stop, stopped := make(chan struct{}), make(chan error, 1)
	go func() {
		select {
		case <-ctx.Done():
			stopped <- f.signal(event)
		case <-stop:
			stopped <- nil
		}
	}()
	var code uint32
	if ctx.Err() == nil {
		if action == 1 {
			// WU/WSUS is allowed for this explicit fixed FoD action only.
			// No custom source, proxy, path or package name crosses this API.
			code = f.add(session, windows.StringToUTF16Ptr(CapabilityName), false, nil, 0, event, 0, 0)
		} else {
			code = f.remove(session, windows.StringToUTF16Ptr(CapabilityName), event, 0, 0)
		}
	}
	close(stop)
	signalErr := <-stopped
	closeEventErr := f.closeEvent(event)
	reboot = code == dismRebootRequired || code == dismRebootRequiredHRESULT
	if closeEventErr != nil {
		d.poisoned = true
	}
	if ctx.Err() != nil || signalErr != nil || closeEventErr != nil {
		return state, reboot, errors.Join(ctx.Err(), signalErr, closeEventErr, ErrReadback)
	}
	if code != 0 && code != dismReloadSession && !reboot {
		return state, false, dismHRESULT(code)
	}
	// Reload=1 is success requiring a new session, not permission to retry the
	// mutation. Also reopen after ordinary success/reboot for a fresh query.
	if closeCode := f.close(session); closeCode != 0 {
		d.poisoned = true
		canShutdown = false
		// Do not double-close an unknown handle or initialize again. The helper
		// must exit; the still-existing transaction backup remains authoritative.
		session = 0
		return state, reboot, errors.Join(dismHRESULT(closeCode), ErrReadback)
	}
	session = 0
	if ctx.Err() != nil {
		return state, reboot, ctx.Err()
	}
	if err = open(); err != nil {
		return state, reboot, err
	}
	if ctx.Err() != nil {
		return state, reboot, ctx.Err()
	}
	state, err = readFixedCapability(f, session)
	if err != nil || ctx.Err() != nil {
		if err == nil {
			err = ctx.Err()
		}
		return state, reboot, err
	}
	if state == 1 || state == 5 {
		return state, true, nil
	}
	if reboot {
		return state, true, nil
	}
	if action == 1 && state == 4 || action == -1 && (state == 0 || state == 3) {
		return state, false, nil
	}
	return state, false, ErrReadback
}

func readFixedCapability(f *dismFunctions, session uint32) (state uint32, err error) {
	var info *dismCapabilityInfo
	code := f.info(session, windows.StringToUTF16Ptr(CapabilityName), &info)
	if info != nil {
		defer func() {
			if deleteCode := f.delete(info); deleteCode != 0 {
				err = errors.Join(err, dismHRESULT(deleteCode), ErrReadback)
			}
		}()
	}
	if code != 0 {
		return 0, dismHRESULT(code)
	}
	if info == nil || !fixedCapabilityName(info.name) || info.state > 7 {
		return 0, ErrReadback
	}
	return info.state, nil
}

func fixedCapabilityName(value *uint16) bool {
	if value == nil {
		return false
	}
	for i, want := range []byte(CapabilityName) {
		if *(*uint16)(unsafe.Add(unsafe.Pointer(value), i*2)) != uint16(want) {
			return false
		}
	}
	return *(*uint16)(unsafe.Add(unsafe.Pointer(value), len(CapabilityName)*2)) == 0
}

func nativeCapabilityState(ctx context.Context) (uint32, error) {
	state, _, err := fixedCapabilityDISM.execute(ctx, 0)
	return state, err
}

func nativeAddCapability(ctx context.Context) (bool, error) {
	_, reboot, err := fixedCapabilityDISM.execute(ctx, 1)
	return reboot, err
}

func nativeRemoveCapability(ctx context.Context) (bool, error) {
	_, reboot, err := fixedCapabilityDISM.execute(ctx, -1)
	return reboot, err
}

var dismDLL = windows.NewLazySystemDLL("DismAPI.dll")

//go:uintptrescapes
func callDISM(proc *windows.LazyProc, args ...uintptr) uint32 {
	result, _, _ := proc.Call(args...)
	return uint32(result) // HRESULT is 32 bits even on x64/arm64.
}

func loadNativeDISM() (*dismFunctions, error) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return nil, ErrUnsupported
	}
	names := []string{"DismInitialize", "DismShutdown", "DismOpenSession", "DismCloseSession", "DismGetCapabilityInfo", "DismDelete", "DismAddCapability", "DismRemoveCapability"}
	procs := make([]*windows.LazyProc, len(names))
	for i, name := range names {
		procs[i] = dismDLL.NewProc(name)
		if procs[i].Find() != nil {
			return nil, ErrUnsupported
		}
	}
	return &dismFunctions{
		initialize: func(level uint32, log, scratch *uint16) uint32 {
			return callDISM(procs[0], uintptr(level), uintptr(unsafe.Pointer(log)), uintptr(unsafe.Pointer(scratch)))
		},
		shutdown: func() uint32 { return callDISM(procs[1]) },
		open: func(image, directory, drive *uint16, session *uint32) uint32 {
			return callDISM(procs[2], uintptr(unsafe.Pointer(image)), uintptr(unsafe.Pointer(directory)), uintptr(unsafe.Pointer(drive)), uintptr(unsafe.Pointer(session)))
		},
		close: func(session uint32) uint32 { return callDISM(procs[3], uintptr(session)) },
		info: func(session uint32, name *uint16, result **dismCapabilityInfo) uint32 {
			return callDISM(procs[4], uintptr(session), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(result)))
		},
		delete: func(info *dismCapabilityInfo) uint32 { return callDISM(procs[5], uintptr(unsafe.Pointer(info))) },
		add: func(session uint32, name *uint16, limit bool, sources **uint16, count uint32, event, progress, user uintptr) uint32 {
			var flag uintptr
			if limit {
				flag = 1
			}
			return callDISM(procs[6], uintptr(session), uintptr(unsafe.Pointer(name)), flag, uintptr(unsafe.Pointer(sources)), uintptr(count), event, progress, user)
		},
		remove: func(session uint32, name *uint16, event, progress, user uintptr) uint32 {
			return callDISM(procs[7], uintptr(session), uintptr(unsafe.Pointer(name)), event, progress, user)
		},
		newEvent: func() (uintptr, error) {
			handle, err := windows.CreateEvent(nil, 1, 0, nil)
			return uintptr(handle), err
		},
		signal:     func(handle uintptr) error { return windows.SetEvent(windows.Handle(handle)) },
		closeEvent: func(handle uintptr) error { return windows.CloseHandle(windows.Handle(handle)) },
	}, nil
}
