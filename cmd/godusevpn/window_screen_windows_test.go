//go:build windows

package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestWindowScreenABIAndInjectedPreCreateOptions(t *testing.T) {
	if unsafe.Sizeof(windowRECT{}) != 16 || unsafe.Sizeof(windowMonitorInfo{}) != 40 || unsafe.Offsetof(windowMonitorInfo{}.Work) != 20 {
		t.Fatal("Win32 RECT/MONITORINFO layout")
	}
	for _, known := range []bool{true, false} {
		o := &options.App{Width: 420, Height: 760, MinWidth: 380, MinHeight: 640, MaxWidth: 1040, MaxHeight: 1000, StartHidden: true, DisableResize: true}
		calls := 0
		fitWindowsWindowOptions(o, func() (windowWorkArea, bool) { calls++; return windowWorkArea{0, 0, 1041, 720, 96, 96}, known })
		if calls != 1 || o.MaxWidth != 0 || o.MaxHeight != 0 || o.DisableResize || !o.StartHidden {
			t.Fatal("pre-create options lost native resizing/tray intent")
		}
		if known && (o.Width != 420 || o.Height != 704 || o.MinWidth != 380 || o.MinHeight != 640) {
			t.Fatal("pre-create clamp")
		}
		if !known && (o.WindowStartState != options.Maximised || o.MinWidth != 1 || o.MinHeight != 1) {
			t.Fatal("unknown screen was guessed instead of OS fallback")
		}
	}
}

type windowLifecycleFixture struct {
	mu    sync.Mutex
	calls []string
	sizes []windowGeometry
}

func (f *windowLifecycleFixture) add(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}
func (f *windowLifecycleFixture) text() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}
func fakeWindowLifecycle(t *testing.T) *windowLifecycleFixture {
	t.Helper()
	f := &windowLifecycleFixture{}
	oldMin, oldSize, oldCenter, oldShow, oldWait := setInitialWindowMinSize, setInitialWindowSize, centerInitialWindow, showInitialWindow, initialWindowStartupWait
	t.Cleanup(func() {
		setInitialWindowMinSize, setInitialWindowSize, centerInitialWindow, showInitialWindow, initialWindowStartupWait = oldMin, oldSize, oldCenter, oldShow, oldWait
	})
	setInitialWindowMinSize = func(_ context.Context, w, h int) {
		f.add("min")
		f.mu.Lock()
		f.sizes = append(f.sizes, windowGeometry{MinWidth: w, MinHeight: h})
		f.mu.Unlock()
	}
	setInitialWindowSize = func(_ context.Context, w, h int) {
		f.add("size")
		f.mu.Lock()
		f.sizes = append(f.sizes, windowGeometry{Width: w, Height: h})
		f.mu.Unlock()
	}
	centerInitialWindow = func(context.Context) { f.add("center") }
	showInitialWindow = func(context.Context) { f.add("show") }
	initialWindowStartupWait = 50 * time.Millisecond
	return f
}

func TestWindowLifecycleGeometryPrecedesOriginalDOMAndFirstShow(t *testing.T) {
	f := fakeWindowLifecycle(t)
	o := &options.App{OnStartup: func(context.Context) { f.add("startup") }, OnDomReady: func(context.Context) { f.add("dom") }}
	guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) { f.add("read"); return windowWorkArea{-1041, 0, 0, 720, 144, 144}, true })
	if !o.StartHidden || o.MinWidth != 1 || o.MinHeight != 1 {
		t.Fatal("unguarded initial presentation")
	}
	o.OnStartup(context.Background())
	o.OnDomReady(context.Background())
	if got := f.text(); got != "startup,read,min,size,center,read,dom,show" || o.WindowStartState != options.Normal {
		t.Fatal("initial fit/DOM/show order", got)
	}
	o.OnDomReady(context.Background())
	if got := f.text(); got != "startup,read,min,size,center,read,dom,show,dom" {
		t.Fatal("reload reset user geometry or re-showed window", got)
	}
}

func TestWindowLifecycleEarlyDOMWaitsWithoutBlockingStartup(t *testing.T) {
	f := fakeWindowLifecycle(t)
	ctx := context.Background()
	o := &options.App{OnStartup: func(context.Context) { f.add("startup") }, OnDomReady: func(context.Context) { f.add("dom") }}
	guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) { f.add("read"); return windowWorkArea{0, 0, 1041, 720, 96, 96}, true })
	finished := make(chan struct{})
	go func() { o.OnDomReady(ctx); close(finished) }()
	if f.text() != "" {
		t.Fatal("early DOM invoked runtime before startup")
	}
	o.OnStartup(ctx)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("startup and DOM mutually blocked")
	}
	if got := f.text(); got != "startup,read,min,size,center,read,dom,show" {
		t.Fatal("early callback order", got)
	}
}

func TestWindowLifecycleMissingOrStuckStartupHasFiniteMaximizedFallback(t *testing.T) {
	for _, stuck := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "stuck"}[stuck], func(t *testing.T) {
			f := fakeWindowLifecycle(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release, startDone, domDone := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			o := &options.App{OnStartup: func(context.Context) { close(entered); <-release; f.add("startup-finished") }, OnDomReady: func(context.Context) { f.add("dom"); close(domDone) }}
			guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) {
				t.Fatal("timed-out startup reached native measurement")
				return windowWorkArea{}, false
			})
			if stuck {
				go func() { o.OnStartup(ctx); close(startDone) }()
				<-entered
			}
			returned := make(chan struct{})
			go func() { o.OnDomReady(ctx); close(returned) }()
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("permanently hidden")
			}
			if f.text() != "show" || o.WindowStartState != options.Maximised || o.MinWidth != 1 || o.MinHeight != 1 {
				t.Fatal("fallback pretended App DOM/startup was ready", f.text())
			}
			if stuck {
				close(release)
				<-startDone
				select {
				case <-domDone:
				case <-time.After(time.Second):
					t.Fatal("original DOM not released after actual startup")
				}
			} else {
				cancel()
			}
		})
	}
}

func TestWindowLifecycleQueryFailureAndTrayOnlyRemainSafe(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "tray-only"}[hidden], func(t *testing.T) {
			f := fakeWindowLifecycle(t)
			o := &options.App{StartHidden: hidden, OnDomReady: func(context.Context) { f.add("dom") }}
			guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) { f.add("read-failed"); return windowWorkArea{}, false })
			o.OnStartup(context.Background())
			o.OnDomReady(context.Background())
			want := "read-failed,min,dom"
			if !hidden {
				want += ",show"
			}
			if f.text() != want || o.WindowStartState != options.Maximised {
				t.Fatal("native read failure lost fallback/tray intent", f.text())
			}
		})
	}
}

func TestWindowLifecycleCancelledDOMNeverDisplaysOrPretendsReady(t *testing.T) {
	f := fakeWindowLifecycle(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := &options.App{OnDomReady: func(context.Context) { t.Fatal("cancelled original DOM invoked") }}
	guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) { t.Fatal("cancelled query"); return windowWorkArea{}, false })
	o.OnDomReady(ctx)
	if f.text() != "" {
		t.Fatal("cancelled context displayed window")
	}
}

func TestWindowLifecycleTrayOnlySuccessAndTimeoutNeverAutoShow(t *testing.T) {
	for _, startupComplete := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "fitted"}[startupComplete], func(t *testing.T) {
			f := fakeWindowLifecycle(t)
			initialWindowStartupWait = 5 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			o := &options.App{StartHidden: true, OnDomReady: func(context.Context) { f.add("dom") }}
			guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) {
				f.add("read")
				return windowWorkArea{0, 0, 1041, 720, 96, 96}, true
			})
			if startupComplete {
				o.OnStartup(ctx)
			}
			o.OnDomReady(ctx)
			if strings.Contains(f.text(), "show") || !startupComplete && f.text() != "" {
				t.Fatal("tray startup displayed or prematurely released original DOM", f.text())
			}
		})
	}
}

func TestWindowLifecycleCancellationDuringStartupWaitCannotStayBlocked(t *testing.T) {
	f := fakeWindowLifecycle(t)
	ctx, cancel := context.WithCancel(context.Background())
	o := &options.App{OnDomReady: func(context.Context) { t.Fatal("cancelled DOM released") }}
	guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) {
		t.Fatal("cancelled monitor queried")
		return windowWorkArea{}, false
	})
	done := make(chan struct{})
	go func() { o.OnDomReady(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled DOM wait blocked")
	}
	if f.text() != "" {
		t.Fatal("cancelled wait displayed or resized a window")
	}
}

func TestWindowLifecycleUnstableMonitorFallsBackAfterBoundedRefits(t *testing.T) {
	f := fakeWindowLifecycle(t)
	o := &options.App{}
	reads := 0
	guardInitialWindowPresentation(o, windowGeometry{420, 760, 380, 640}, options.Normal, func() (windowWorkArea, bool) {
		reads++
		return windowWorkArea{reads, 0, 1041 + reads, 720, 96, 96}, true
	})
	o.OnStartup(context.Background())
	o.OnDomReady(context.Background())
	if reads != 4 || o.WindowStartState != options.Maximised || !strings.HasSuffix(f.text(), "min,show") {
		t.Fatal("unstable monitor refit unbounded or wrongly accepted", reads, f.text())
	}
}
