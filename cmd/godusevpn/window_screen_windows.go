//go:build windows

package main

import (
	"context"
	"sync"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

type windowRECT struct{ Left, Top, Right, Bottom int32 }
type windowMonitorInfo struct {
	Size    uint32
	Monitor windowRECT
	Work    windowRECT
	Flags   uint32
}

// All lazy procedures below are read-only queries. No SetProcessDpi* call or
// screen configuration API is used. They are invoked only at real GUI startup,
// never from init or the synthetic geometry tests.
var windowUser32 = windows.NewLazySystemDLL("user32.dll")
var windowMonitorFromRect = windowUser32.NewProc("MonitorFromRect")
var windowMonitorFromWindow = windowUser32.NewProc("MonitorFromWindow")
var windowGetMonitorInfo = windowUser32.NewProc("GetMonitorInfoW")
var windowSystemParametersInfo = windowUser32.NewProc("SystemParametersInfoW")
var windowGetDPIForSystem = windowUser32.NewProc("GetDpiForSystem")
var windowGetDPIForWindow = windowUser32.NewProc("GetDpiForWindow")
var windowEnumWindows = windowUser32.NewProc("EnumWindows")
var windowGetThreadProcessID = windowUser32.NewProc("GetWindowThreadProcessId")
var windowGetClassName = windowUser32.NewProc("GetClassNameW")

var setInitialWindowMinSize = runtime.WindowSetMinSize
var setInitialWindowSize = runtime.WindowSetSize
var centerInitialWindow = runtime.WindowCenter
var showInitialWindow = runtime.WindowShow
var initialWindowStartupWait = 2 * time.Second

func readInitialWindowWorkArea() (windowWorkArea, bool) {
	// Pre-create safety estimate only. CW_USEDEFAULT can choose a different
	// monitor; presentation remains hidden until the actual HWND is refitted.
	// Primary system DPI is used here, not cursor/secondary-monitor DPI.
	// GetDpiForSystem observes the manifest's existing DPI-awareness context.
	// https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getdpiforsystem
	if windowGetDPIForSystem.Find() != nil {
		return windowWorkArea{}, false
	}
	dpi, _, _ := windowGetDPIForSystem.Call()
	if dpi < 48 || dpi > 768 {
		return windowWorkArea{}, false
	}
	var work windowRECT
	known := false
	if windowMonitorFromRect.Find() == nil && windowGetMonitorInfo.Find() == nil {
		// The primary monitor contains desktop origin. Pass RECT by pointer to
		// avoid architecture-specific POINT-by-value ABI conventions.
		origin := windowRECT{Right: 1, Bottom: 1}
		monitor, _, _ := windowMonitorFromRect.Call(uintptr(unsafe.Pointer(&origin)), 1)
		if monitor != 0 {
			info := windowMonitorInfo{Size: uint32(unsafe.Sizeof(windowMonitorInfo{}))}
			ok, _, _ := windowGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
			if ok != 0 && info.Work.Right > info.Work.Left && info.Work.Bottom > info.Work.Top {
				work, known = info.Work, true
			}
		}
	}
	if !known && windowSystemParametersInfo.Find() == nil {
		// SPI_GETWORKAREA=0x0030, read only; fWinIni=0. This fallback reports
		// the primary monitor work area. Never use SPI_SETWORKAREA.
		ok, _, _ := windowSystemParametersInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0)
		known = ok != 0
	}
	if !known {
		return windowWorkArea{}, false
	}
	return windowWorkArea{int(work.Left), int(work.Top), int(work.Right), int(work.Bottom), int(dpi), int(dpi)}, true
}

func readActualWindowWorkArea(className string) (windowWorkArea, bool) {
	for _, proc := range []*windows.LazyProc{windowEnumWindows, windowGetThreadProcessID, windowGetClassName, windowMonitorFromWindow, windowGetMonitorInfo, windowGetDPIForWindow} {
		if proc.Find() != nil {
			return windowWorkArea{}, false
		}
	}
	var handle uintptr
	count := 0
	self := windows.GetCurrentProcessId()
	callback := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		windowGetThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != self {
			return 1
		}
		var name [256]uint16
		n, _, _ := windowGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if n != 0 && n < uintptr(len(name)) && windows.UTF16ToString(name[:n]) == className {
			handle = hwnd
			count++
		}
		return 1
	})
	ok, _, _ := windowEnumWindows.Call(callback, 0)
	if ok == 0 || count != 1 {
		return windowWorkArea{}, false
	}
	monitor, _, _ := windowMonitorFromWindow.Call(handle, 2) // DEFAULTTONEAREST
	if monitor == 0 {
		return windowWorkArea{}, false
	}
	info := windowMonitorInfo{Size: uint32(unsafe.Sizeof(windowMonitorInfo{}))}
	ok, _, _ = windowGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return windowWorkArea{}, false
	}
	dpi, _, _ := windowGetDPIForWindow.Call(handle)
	return windowWorkArea{int(info.Work.Left), int(info.Work.Top), int(info.Work.Right), int(info.Work.Bottom), int(dpi), int(dpi)}, true
}

func configureInitialWindowsWindow(o *options.App) {
	preferred := windowGeometry{o.Width, o.Height, o.MinWidth, o.MinHeight}
	initialState := o.WindowStartState
	fitWindowsWindowOptions(o, readInitialWindowWorkArea)
	className := "wailsWindow"
	if o.Windows != nil && o.Windows.WindowClassName != "" {
		className = o.Windows.WindowClassName
	}
	guardInitialWindowPresentation(o, preferred, initialState, func() (windowWorkArea, bool) { return readActualWindowWorkArea(className) })
}

// Wails starts both callbacks in independent goroutines. OnStartup performs
// no geometry/runtime calls, avoiding an Invoke cycle with a waiting DOM
// callback. Only the first DOM callback waits, with a finite timeout; a
// missing/stuck startup or failed native read falls back to OS maximization.
func guardInitialWindowPresentation(o *options.App, preferred windowGeometry, initialState options.WindowStartState, read func() (windowWorkArea, bool)) {
	hidden := o.StartHidden
	startup, domReady := o.OnStartup, o.OnDomReady
	ready := make(chan struct{})
	var presented sync.Once
	var deferredDOM sync.Once
	o.StartHidden = true
	o.MinWidth, o.MinHeight = 1, 1
	o.WindowStartState = options.Maximised
	o.OnStartup = func(ctx context.Context) {
		defer close(ready)
		if startup != nil {
			startup(ctx)
		}
	}
	o.OnDomReady = func(ctx context.Context) {
		if ctx.Err() != nil {
			return
		}
		firstPresentation := false
		presented.Do(func() {
			firstPresentation = true
			timer := time.NewTimer(initialWindowStartupWait)
			defer timer.Stop()
			startupComplete := false
			select {
			case <-ctx.Done():
				return
			case <-ready:
				startupComplete = true
			case <-timer.C:
			}
			if ctx.Err() != nil {
				return
			}
			if startupComplete && applyInitialWindowFit(ctx, preferred, read) {
				o.WindowStartState = initialState
			}
		})
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ready:
			if domReady != nil {
				domReady(ctx)
			}
		default:
			// A timed-out lifecycle may show a safe default window, but must
			// not pretend App startup completed or release pending deep links.
			// Coalesce pre-startup reloads; this waiter never blocks the UI.
			deferredDOM.Do(func() {
				go func() {
					select {
					case <-ctx.Done():
						return
					case <-ready:
						if ctx.Err() == nil && domReady != nil {
							domReady(ctx)
						}
					}
				}()
			})
		}
		if firstPresentation && !hidden && ctx.Err() == nil {
			showInitialWindow(ctx)
		}
	}
}

func applyInitialWindowFit(ctx context.Context, preferred windowGeometry, read func() (windowWorkArea, bool)) bool {
	area, known := read()
	for pass := 0; pass < 3 && known && ctx.Err() == nil; pass++ {
		fitted, ok := fitWindowGeometry(preferred, area)
		if !ok {
			break
		}
		setInitialWindowMinSize(ctx, fitted.MinWidth, fitted.MinHeight)
		setInitialWindowSize(ctx, fitted.Width, fitted.Height)
		centerInitialWindow(ctx)
		// Wails calls these operations synchronously. Verify the monitor/DPI
		// once more after centering before allowing a pending-link Show.
		next, exists := read()
		if exists && next == area {
			return true
		}
		area, known = next, exists
	}
	setInitialWindowMinSize(ctx, 1, 1)
	return false
}

func fitWindowsWindowOptions(o *options.App, read func() (windowWorkArea, bool)) {
	// A fixed max size also caps Wails' frameless maximized client area, leaving
	// most of a large display unused. Let Windows maximize to rcWork instead.
	o.MaxWidth, o.MaxHeight = 0, 0
	o.DisableResize = false
	preferred := windowGeometry{o.Width, o.Height, o.MinWidth, o.MinHeight}
	area, known := read()
	if fitted, ok := fitWindowGeometry(preferred, area); known && ok {
		o.Width, o.Height, o.MinWidth, o.MinHeight = fitted.Width, fitted.Height, fitted.MinWidth, fitted.MinHeight
		return
	}
	// No reliable native measurement: do not invent a screen or 96-DPI result.
	// The OS can still maximize within its own work area. Avoid min-track sizes
	// that could force that maximized window beyond an unusually small desktop.
	o.Width, o.Height = 420, 600
	o.MinWidth, o.MinHeight = 1, 1
	o.WindowStartState = options.Maximised
}
