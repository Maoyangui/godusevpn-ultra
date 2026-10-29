//go:build windows

package clipboardprivacy

import (
	"crypto/sha256"
	"crypto/subtle"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clipboardUser32       = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel32     = windows.NewLazySystemDLL("kernel32.dll")
	clipCreateWindow      = clipboardUser32.NewProc("CreateWindowExW")
	clipDestroyWindow     = clipboardUser32.NewProc("DestroyWindow")
	clipGetMessage        = clipboardUser32.NewProc("GetMessageW")
	clipTranslateMessage  = clipboardUser32.NewProc("TranslateMessage")
	clipDispatchMessage   = clipboardUser32.NewProc("DispatchMessageW")
	clipPostMessage       = clipboardUser32.NewProc("PostMessageW")
	clipPostThreadMessage = clipboardUser32.NewProc("PostThreadMessageW")
	clipOpen              = clipboardUser32.NewProc("OpenClipboard")
	clipClose             = clipboardUser32.NewProc("CloseClipboard")
	clipEmpty             = clipboardUser32.NewProc("EmptyClipboard")
	clipSet               = clipboardUser32.NewProc("SetClipboardData")
	clipGet               = clipboardUser32.NewProc("GetClipboardData")
	clipOwner             = clipboardUser32.NewProc("GetClipboardOwner")
	clipSequence          = clipboardUser32.NewProc("GetClipboardSequenceNumber")
	clipGlobalAlloc       = clipboardKernel32.NewProc("GlobalAlloc")
	clipGlobalFree        = clipboardKernel32.NewProc("GlobalFree")
	clipGlobalLock        = clipboardKernel32.NewProc("GlobalLock")
	clipGlobalUnlock      = clipboardKernel32.NewProc("GlobalUnlock")
	clipGlobalSize        = clipboardKernel32.NewProc("GlobalSize")
	clipMoveMemory        = clipboardKernel32.NewProc("RtlMoveMemory")
)

const clipboardWorkMessage = 0x8000 + 0x1a2

type clipboardMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type nativeClipboardRequest struct {
	kind    string
	text    string
	receipt Receipt
	reply   chan nativeClipboardResult
}
type nativeClipboardResult struct {
	receipt Receipt
	cleared bool
	err     error
}
type windowsClipboard struct {
	mu              sync.Mutex
	started, closed bool
	window          uintptr
	thread          uint32
	requests        chan nativeClipboardRequest
	done            chan struct{}
}

// Construction and Supported are side-effect free. A private message-only
// owner window/thread is created only by an explicit secret-copy operation.
func NewNativeBackend() Backend           { return &windowsClipboard{} }
func (*windowsClipboard) Supported() bool { return true }
func (w *windowsClipboard) startLocked() error {
	if w.started {
		return nil
	}
	ready := make(chan error, 1)
	w.requests = make(chan nativeClipboardRequest, 1)
	w.done = make(chan struct{})
	w.started = true
	go w.run(ready)
	if err := <-ready; err != nil {
		return err
	}
	return nil
}
func (w *windowsClipboard) run(ready chan error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(w.done)
	class, _ := windows.UTF16PtrFromString("STATIC")
	window, _, _ := clipCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, 0, 0)
	if window == 0 {
		ready <- ErrUnavailable
		return
	}
	w.window, w.thread = window, windows.GetCurrentThreadId()
	defer clipDestroyWindow.Call(window)
	ready <- nil
	for {
		var message clipboardMessage
		ok, _, _ := clipGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if ok == 0 || int32(ok) == -1 {
			return
		}
		if message.Window == window && message.Message == clipboardWorkMessage {
			select {
			case request := <-w.requests:
				var result nativeClipboardResult
				switch request.kind {
				case "write":
					result.receipt, result.err = w.write(request.text)
				case "clear":
					result.cleared, result.err = w.clear(request.receipt)
				case "close":
					request.reply <- result
					return
				default:
					result.err = ErrInvalid
				}
				request.text = ""
				request.reply <- result
			default:
			}
			continue
		}
		clipTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		clipDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
	}
}
func (w *windowsClipboard) operation(request nativeClipboardRequest) nativeClipboardResult {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nativeClipboardResult{err: ErrClosed}
	}
	if err := w.startLocked(); err != nil {
		return nativeClipboardResult{err: err}
	}
	request.reply = make(chan nativeClipboardResult, 1)
	select {
	case <-w.done:
		return nativeClipboardResult{err: ErrUnavailable}
	default:
	}
	w.requests <- request
	if ok, _, _ := clipPostMessage.Call(w.window, clipboardWorkMessage, 0, 0); ok == 0 {
		select {
		case <-w.requests:
		default:
		}
		return nativeClipboardResult{err: ErrUnavailable}
	}
	select {
	case result := <-request.reply:
		return result
	case <-w.done:
		return nativeClipboardResult{err: ErrUnavailable}
	}
}
func (w *windowsClipboard) WriteSecret(text string) (Receipt, error) {
	r := w.operation(nativeClipboardRequest{kind: "write", text: text})
	return r.receipt, r.err
}
func (w *windowsClipboard) ClearIfUnchanged(receipt Receipt) (bool, error) {
	r := w.operation(nativeClipboardRequest{kind: "clear", receipt: receipt})
	return r.cleared, r.err
}
func (w *windowsClipboard) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if !w.started {
		return nil
	}
	// No clipboard mutation: the manager has already attempted its conditional
	// cleanup. A thread quit also releases a window destroyed externally.
	clipPostThreadMessage.Call(uintptr(w.thread), 0x12, 0, 0)
	<-w.done
	return nil
}
func (w *windowsClipboard) write(text string) (Receipt, error) {
	if !validSecret(text) {
		return Receipt{}, ErrInvalid
	}
	utf16, err := windows.UTF16FromString(text)
	if err != nil {
		return Receipt{}, ErrInvalid
	}
	memory, _, _ := clipGlobalAlloc.Call(2, uintptr(len(utf16)*2))
	if memory == 0 {
		return Receipt{}, ErrUnavailable
	}
	transferred := false
	defer func() {
		if !transferred {
			clipGlobalFree.Call(memory)
		}
	}()
	address, _, _ := clipGlobalLock.Call(memory)
	if address == 0 {
		return Receipt{}, ErrUnavailable
	}
	clipMoveMemory.Call(address, uintptr(unsafe.Pointer(&utf16[0])), uintptr(len(utf16)*2))
	runtime.KeepAlive(utf16)
	clipGlobalUnlock.Call(memory)
	if ok, _, _ := clipOpen.Call(w.window); ok == 0 {
		return Receipt{}, ErrBusy
	}
	defer clipClose.Call()
	if ok, _, _ := clipEmpty.Call(); ok == 0 {
		return Receipt{}, ErrUnavailable
	}
	if handle, _, _ := clipSet.Call(13, memory); handle == 0 {
		return Receipt{}, ErrUnavailable
	}
	transferred = true
	sequence, _, _ := clipSequence.Call()
	return Receipt{Sequence: uint32(sequence), Digest: sha256.Sum256([]byte(text))}, nil
}
func (w *windowsClipboard) clear(receipt Receipt) (bool, error) {
	if receipt.Sequence == 0 {
		return false, ErrUnverifiable
	}
	if ok, _, _ := clipOpen.Call(w.window); ok == 0 {
		return false, ErrBusy
	}
	defer clipClose.Call()
	sequence, _, _ := clipSequence.Call()
	owner, _, _ := clipOwner.Call()
	if sequence == 0 {
		return false, ErrUnverifiable
	}
	if uint32(sequence) != receipt.Sequence || owner != w.window {
		return false, nil
	}
	memory, _, _ := clipGet.Call(13)
	if memory == 0 {
		return false, ErrUnverifiable
	}
	size, _, _ := clipGlobalSize.Call(memory)
	if size < 2 || size > uintptr((MaxSecretBytes+1)*2) || size%2 != 0 {
		return false, ErrUnverifiable
	}
	address, _, _ := clipGlobalLock.Call(memory)
	if address == 0 {
		return false, ErrUnverifiable
	}
	data := make([]uint16, int(size/2))
	clipMoveMemory.Call(uintptr(unsafe.Pointer(&data[0])), address, size)
	runtime.KeepAlive(data)
	end := -1
	for i, ch := range data {
		if ch == 0 {
			end = i
			break
		}
	}
	if end < 0 {
		clipGlobalUnlock.Call(memory)
		return false, ErrUnverifiable
	}
	digest := sha256.Sum256([]byte(windows.UTF16ToString(data[:end])))
	clipGlobalUnlock.Call(memory)
	if subtle.ConstantTimeCompare(digest[:], receipt.Digest[:]) != 1 {
		return false, nil
	}
	// The clipboard remains open throughout comparison and mutation.
	current, _, _ := clipSequence.Call()
	if uint32(current) != receipt.Sequence {
		return false, nil
	}
	if ok, _, _ := clipEmpty.Call(); ok == 0 {
		return false, ErrUnavailable
	}
	return true, nil
}
