//go:build windows && (amd64 || arm64)

package locationportalprepare

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only delegates and synthetic in-memory ABI objects are used here. No test
// loads DismAPI, queries the local image, creates an OS event, or services FoD.
type fakeDISM struct {
	t                                     *testing.T
	mu                                    sync.Mutex
	calls                                 []string
	states                                []uint32
	opens, closes, reads, deletes, writes int
	initializeCode, shutdownCode          uint32
	openErrorAt, closeErrorAt             int
	infoErrorAt, deleteErrorAt            int
	writeCode                             uint32
	missingInfo, wrongName                bool
	event                                 uintptr
	eventErr, closeEventErr, signalErr    error
	writeHook                             func()
	newEventHook, infoHook                func()
	openReturnedSessionOnError            bool
	functions                             *dismFunctions
}

func fakeCapabilityDISM(t *testing.T, states ...uint32) (*capabilityDISM, *fakeDISM) {
	t.Helper()
	f := &fakeDISM{t: t, states: states, event: 77}
	f.functions = &dismFunctions{
		initialize: func(level uint32, log, scratch *uint16) uint32 {
			f.record("initialize")
			if level != 0 || log != nil || scratch != nil {
				t.Error("nonfixed log/scratch/level")
			}
			return f.initializeCode
		},
		shutdown: func() uint32 { f.record("shutdown"); return f.shutdownCode },
		open: func(image, directory, drive *uint16, session *uint32) uint32 {
			f.opens++
			f.record("open")
			if windows.UTF16PtrToString(image) != dismOnlineImage || directory != nil || drive != nil || session == nil {
				t.Error("not fixed online image")
			}
			if f.openErrorAt == f.opens {
				if f.openReturnedSessionOnError {
					*session = uint32(f.opens + 100)
				}
				return 0x80070005
			}
			*session = uint32(f.opens + 100)
			return 0
		},
		close: func(session uint32) uint32 {
			f.closes++
			f.record("close")
			if session < 101 || session > uint32(100+f.opens) {
				t.Error("unknown session closed")
			}
			if f.closeErrorAt == f.closes {
				return 0x80070005
			}
			return 0
		},
		info: func(session uint32, name *uint16, out **dismCapabilityInfo) uint32 {
			f.reads++
			f.record("info")
			if session != uint32(100+f.opens) || !fixedCapabilityName(name) {
				t.Error("wrong capability/session query")
			}
			if !f.missingInfo {
				index := f.reads - 1
				if index >= len(f.states) {
					index = len(f.states) - 1
				}
				n := CapabilityName
				if f.wrongName {
					n = CapabilityName + "-unowned"
				}
				*out = &dismCapabilityInfo{name: windows.StringToUTF16Ptr(n), state: f.states[index]}
			}
			if f.infoHook != nil {
				f.infoHook()
			}
			if f.infoErrorAt == f.reads {
				return 0x80070005
			}
			return 0
		},
		delete: func(info *dismCapabilityInfo) uint32 {
			f.record("delete")
			f.deletes++
			if info == nil {
				t.Error("nil DismDelete")
			}
			if f.deleteErrorAt == f.deletes {
				return 0x80070005
			}
			return 0
		},
		add: func(session uint32, name *uint16, limit bool, sources **uint16, count uint32, event, progress, user uintptr) uint32 {
			f.record("add")
			f.writes++
			if session != uint32(100+f.opens) || !fixedCapabilityName(name) || limit || sources != nil || count != 0 || event != 77 || progress != 0 || user != 0 {
				t.Error("unbounded AddCapability parameters")
			}
			if f.writeHook != nil {
				f.writeHook()
			}
			return f.writeCode
		},
		remove: func(session uint32, name *uint16, event, progress, user uintptr) uint32 {
			f.record("remove")
			f.writes++
			if session != uint32(100+f.opens) || !fixedCapabilityName(name) || event != 77 || progress != 0 || user != 0 {
				t.Error("unbounded RemoveCapability parameters")
			}
			if f.writeHook != nil {
				f.writeHook()
			}
			return f.writeCode
		},
		newEvent: func() (uintptr, error) {
			f.record("new-event")
			if f.newEventHook != nil {
				f.newEventHook()
			}
			return f.event, f.eventErr
		},
		signal: func(event uintptr) error {
			f.record("signal")
			if event != 77 {
				t.Error("unknown cancellation event")
			}
			return f.signalErr
		},
		closeEvent: func(event uintptr) error {
			f.record("close-event")
			if event != 77 {
				t.Error("unknown event closed")
			}
			return f.closeEventErr
		},
	}
	return newCapabilityDISM(func() (*dismFunctions, error) { f.record("load"); return f.functions, nil }), f
}

func (f *fakeDISM) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}
func (f *fakeDISM) trace() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}
func (f *fakeDISM) count(call string) int {
	count := 0
	for _, item := range f.trace() {
		if item == call {
			count++
		}
	}
	return count
}

func TestDISMReadPreservesAllFormalStatesAndNeverServices(t *testing.T) {
	for state := uint32(0); state <= 7; state++ {
		t.Run(fmt.Sprint(state), func(t *testing.T) {
			d, f := fakeCapabilityDISM(t, state)
			actual, reboot, err := d.execute(context.Background(), 0)
			if actual != state || reboot || err != nil || f.writes != 0 {
				t.Fatal(actual, reboot, err)
			}
			if !reflect.DeepEqual(f.trace(), []string{"load", "initialize", "open", "info", "delete", "close", "shutdown"}) {
				t.Fatal(f.trace())
			}
		})
	}
}

func TestDISMAddRemoveUseOneMutationAndFreshReopenedReadback(t *testing.T) {
	for _, test := range []struct {
		action        int
		before, after uint32
	}{{1, 0, 4}, {1, 3, 4}, {-1, 4, 0}, {-1, 4, 3}} {
		for _, code := range []uint32{0, dismReloadSession} {
			t.Run(fmt.Sprint(test, code), func(t *testing.T) {
				d, f := fakeCapabilityDISM(t, test.before, test.after)
				f.writeCode = code
				state, reboot, err := d.execute(context.Background(), test.action)
				if err != nil || reboot || state != test.after || f.writes != 1 || f.opens != 2 || f.closes != 2 || f.deletes != 2 || f.count("initialize") != 1 || f.count("shutdown") != 1 {
					t.Fatal(state, reboot, err, f.trace())
				}
				operation := "add"
				if test.action < 0 {
					operation = "remove"
				}
				if !reflect.DeepEqual(f.trace(), []string{"load", "initialize", "open", "info", "delete", "new-event", operation, "close-event", "close", "open", "info", "delete", "close", "shutdown"}) {
					t.Fatal(f.trace())
				}
			})
		}
	}
}

func TestDISMRebootAndPendingNeverBecomeInstalledProof(t *testing.T) {
	for _, code := range []uint32{dismRebootRequired, dismRebootRequiredHRESULT} {
		for _, after := range []uint32{0, 1, 3, 4, 5} {
			d, f := fakeCapabilityDISM(t, 0, after)
			f.writeCode = code
			state, reboot, err := d.execute(context.Background(), 1)
			if err != nil || !reboot || state != after || f.writes != 1 || f.opens != 2 {
				t.Fatal(code, after, state, reboot, err, f.trace())
			}
		}
	}
	for _, action := range []int{-1, 1} {
		for _, pending := range []uint32{1, 5} {
			d, f := fakeCapabilityDISM(t, pending)
			state, reboot, err := d.execute(context.Background(), action)
			if err != nil || !reboot || state != pending || f.writes != 0 || f.count("new-event") != 0 {
				t.Fatal(state, reboot, err, f.trace())
			}
		}
	}
	for _, pending := range []uint32{1, 5} {
		d, f := fakeCapabilityDISM(t, 0, pending)
		state, reboot, err := d.execute(context.Background(), 1)
		if err != nil || !reboot || state != pending || f.writes != 1 {
			t.Fatal(state, reboot, err)
		}
	}
}

func TestDISMIdempotentAndUnsupportedStatesNeverMutate(t *testing.T) {
	for _, test := range []struct {
		action int
		before uint32
	}{{1, 4}, {-1, 0}, {-1, 3}} {
		d, f := fakeCapabilityDISM(t, test.before)
		state, reboot, err := d.execute(context.Background(), test.action)
		if err != nil || reboot || state != test.before || f.writes != 0 || f.count("new-event") != 0 {
			t.Fatal(state, reboot, err, f.trace())
		}
	}
	for _, action := range []int{-1, 1} {
		for _, before := range []uint32{2, 6, 7} {
			d, f := fakeCapabilityDISM(t, before)
			_, _, err := d.execute(context.Background(), action)
			if !errors.Is(err, ErrConflict) || f.writes != 0 || f.count("new-event") != 0 {
				t.Fatal(err, f.trace())
			}
		}
	}
}

func TestDISMUnknownReadbackOrNativeFailuresNeverClaimSuccess(t *testing.T) {
	for _, kind := range []string{"initialize", "open", "open-returned-session", "info", "nil-info", "wrong-name", "unknown-state", "delete", "event-zero", "event-invalid", "event-error-owned", "write", "unknown-success-code", "reopen", "final-info", "unchanged", "event-close"} {
		t.Run(kind, func(t *testing.T) {
			d, f := fakeCapabilityDISM(t, 0, 4)
			switch kind {
			case "initialize":
				f.initializeCode = 0xc0040001
			case "open":
				f.openErrorAt = 1
			case "open-returned-session":
				f.openErrorAt, f.openReturnedSessionOnError = 1, true
			case "info":
				f.infoErrorAt = 1
			case "nil-info":
				f.missingInfo = true
			case "wrong-name":
				f.wrongName = true
			case "unknown-state":
				f.states = []uint32{8}
			case "delete":
				f.deleteErrorAt = 1
			case "event-zero":
				f.event = 0
			case "event-invalid":
				f.event = ^uintptr(0)
			case "event-error-owned":
				f.eventErr = ErrReadback
			case "write":
				f.writeCode = 0x80070005
			case "unknown-success-code":
				f.writeCode = 2
			case "reopen":
				f.openErrorAt = 2
			case "final-info":
				f.infoErrorAt = 2
			case "unchanged":
				f.states = []uint32{0, 0}
			case "event-close":
				f.closeEventErr = ErrReadback
			}
			_, _, err := d.execute(context.Background(), 1)
			if err == nil {
				t.Fatal("unknown reported success", f.trace())
			}
			if f.count("initialize") == 1 && f.initializeCode == 0 && f.count("shutdown") != 1 {
				t.Fatal("initialization leaked", f.trace())
			}
			if kind == "initialize" && (f.count("shutdown") != 0 || f.opens != 0) {
				t.Fatal("took over foreign initialization", f.trace())
			}
			if kind == "event-error-owned" && f.count("close-event") != 1 {
				t.Fatal("owned event leaked", f.trace())
			}
			if f.reads > 0 && !f.missingInfo && f.deletes != f.reads {
				t.Fatal("returned allocation not released", f.trace())
			}
		})
	}
}

func TestDISMCloseOrShutdownUncertaintyPoisonsFurtherOperations(t *testing.T) {
	for _, kind := range []string{"reload-close", "final-close", "shutdown"} {
		t.Run(kind, func(t *testing.T) {
			d, f := fakeCapabilityDISM(t, 0, 4)
			switch kind {
			case "reload-close":
				f.closeErrorAt, f.writeCode = 1, dismReloadSession
			case "final-close":
				f.closeErrorAt = 2
			case "shutdown":
				f.shutdownCode = 0x80070005
			}
			if _, _, err := d.execute(context.Background(), 1); err == nil || !d.poisoned {
				t.Fatal(err, f.trace())
			}
			if kind != "shutdown" && f.count("shutdown") != 0 {
				t.Fatal("shutdown on unconfirmed open session", f.trace())
			}
			before := f.trace()
			if _, _, err := d.execute(context.Background(), 0); err == nil || !reflect.DeepEqual(before, f.trace()) {
				t.Fatal("poisoned runtime reused", err, f.trace())
			}
		})
	}
}

func TestDISMCancellationOnlySignalsUntilNativeCallActuallyReturns(t *testing.T) {
	d, f := fakeCapabilityDISM(t, 0, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, signalled, release, complete := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan error, 1)
	f.writeHook = func() { close(entered); <-release }
	f.functions.signal = func(event uintptr) error {
		f.record("signal")
		if event != 77 {
			t.Error("wrong event")
		}
		close(signalled)
		return nil
	}
	go func() { _, _, err := d.execute(ctx, 1); complete <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake mutation did not start")
	}
	cancel()
	select {
	case <-signalled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel not signalled")
	}
	select {
	case <-complete:
		t.Fatal("returned while native operation still uses session/event")
	default:
	}
	if f.count("close") != 0 || f.count("close-event") != 0 || f.count("shutdown") != 0 {
		t.Fatal("cleanup raced native operation", f.trace())
	}
	close(release)
	select {
	case err := <-complete:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fake mutation did not clean up")
	}
	if f.opens != 1 || f.writes != 1 || f.count("close-event") != 1 || f.closes != 1 || f.count("shutdown") != 1 {
		t.Fatal(f.trace())
	}
}

func TestDISMAlreadyCancelledWaitingAndPreMutationCancellationDoNoLateWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d, f := fakeCapabilityDISM(t, 0)
	if _, _, err := d.execute(ctx, 1); !errors.Is(err, context.Canceled) || len(f.trace()) != 0 {
		t.Fatal(err, f.trace())
	}
	d.serial <- struct{}{}
	waitCtx, waitCancel := context.WithCancel(context.Background())
	complete := make(chan error, 1)
	go func() { _, _, err := d.execute(waitCtx, 1); complete <- err }()
	waitCancel()
	if err := <-complete; !errors.Is(err, context.Canceled) || len(f.trace()) != 0 {
		t.Fatal(err, f.trace())
	}
	<-d.serial
	for _, where := range []string{"read", "event"} {
		d, f := fakeCapabilityDISM(t, 0)
		ctx, cancel := context.WithCancel(context.Background())
		if where == "read" {
			f.infoHook = cancel
		} else {
			f.newEventHook = cancel
		}
		_, _, err := d.execute(ctx, 1)
		cancel()
		if !errors.Is(err, context.Canceled) || f.writes != 0 || f.closes != 1 || f.count("shutdown") != 1 {
			t.Fatal(where, err, f.trace())
		}
	}
}

func TestDISMUnchangedOrCancelledFinalReadDoesNotBecomeSuccess(t *testing.T) {
	d, f := fakeCapabilityDISM(t, 0, 4)
	ctx, cancel := context.WithCancel(context.Background())
	f.infoHook = func() {
		if f.reads == 2 {
			cancel()
		}
	}
	_, _, err := d.execute(ctx, 1)
	cancel()
	if !errors.Is(err, context.Canceled) || f.opens != 2 || f.closes != 2 || f.deletes != 2 {
		t.Fatal(err, f.trace())
	}
}

func TestDISMNilOrIncompleteRuntimeNeverLoadsNativeFunctions(t *testing.T) {
	for _, d := range []*capabilityDISM{nil, newCapabilityDISM(nil), newCapabilityDISM(func() (*dismFunctions, error) { return nil, nil }), newCapabilityDISM(func() (*dismFunctions, error) { return &dismFunctions{}, nil })} {
		if _, _, err := d.execute(context.Background(), 0); err == nil {
			t.Fatal("invalid runtime accepted")
		}
	}
	d, f := fakeCapabilityDISM(t, 0)
	if _, _, err := d.execute(nil, 1); err == nil || len(f.trace()) != 0 {
		t.Fatal(err, f.trace())
	}
	if _, _, err := d.execute(context.Background(), 2); err == nil || len(f.trace()) != 0 {
		t.Fatal(err, f.trace())
	}
}

func TestDISM64BitABINameBoundsAndNativeCodes(t *testing.T) {
	var info dismCapabilityInfo
	var session uint32
	if unsafe.Sizeof(session) != 4 || unsafe.Sizeof(info) != 40 || unsafe.Offsetof(info.state) != 8 || unsafe.Offsetof(info.displayName) != 16 || unsafe.Offsetof(info.description) != 24 || unsafe.Offsetof(info.downloadSize) != 32 || unsafe.Offsetof(info.installedSize) != 36 {
		t.Fatal("native capability/session ABI differs")
	}
	if fixedCapabilityName(nil) || fixedCapabilityName(windows.StringToUTF16Ptr("")) || fixedCapabilityName(windows.StringToUTF16Ptr(CapabilityName+"x")) || fixedCapabilityName(windows.StringToUTF16Ptr("Tools.Other~~~~0.0.1.0")) || !fixedCapabilityName(windows.StringToUTF16Ptr(CapabilityName)) {
		t.Fatal("unowned capability name accepted")
	}
	var errno syscall.Errno
	err := dismHRESULT(0x80070005)
	if err.Error() != "LOCATION_PORTAL_DISM_OPERATION_FAILED" || !errors.As(err, &errno) || uint32(errno) != 0x80070005 || dismHRESULT(0) != nil {
		t.Fatal("native code lost or message leaked")
	}
}
