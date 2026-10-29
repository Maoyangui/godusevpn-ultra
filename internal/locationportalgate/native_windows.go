//go:build windows && (amd64 || arm64)

package locationportalgate

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Literal GUIDs avoid loading COM or any native API at package initialization.
var providerID = windows.GUID{Data1: 0x13c8a041, Data2: 0xd12d, Data3: 0x4ed6, Data4: [8]byte{0x8d, 0x64, 0x5a, 0xef, 0x24, 0x7b, 0xe9, 0x41}}
var sublayerID = windows.GUID{Data1: 0x13c8a042, Data2: 0xd12d, Data3: 0x4ed6, Data4: [8]byte{0x8d, 0x64, 0x5a, 0xef, 0x24, 0x7b, 0xe9, 0x41}}
var in4 = windows.GUID{Data1: 0xe1cd9fe7, Data2: 0xf4b5, Data3: 0x4273, Data4: [8]byte{0x96, 0xc0, 0x59, 0x2e, 0x48, 0x7b, 0x86, 0x50}}
var in6 = windows.GUID{Data1: 0xa3b42c97, Data2: 0x9f04, Data3: 0x4672, Data4: [8]byte{0xb8, 0x7e, 0xce, 0xe9, 0xc4, 0x83, 0x25, 0x7f}}
var protocolID = windows.GUID{Data1: 0x3971ef2b, Data2: 0x623e, Data3: 0x4f9a, Data4: [8]byte{0x8c, 0xb1, 0x6e, 0x79, 0xb8, 0x06, 0xb9, 0xa7}}
var localPortID = windows.GUID{Data1: 0x0c1ba1af, Data2: 0x5765, Data3: 0x453f, Data4: [8]byte{0xaf, 0x22, 0xa8, 0xf7, 0x91, 0xac, 0x77, 0x5b}}
var flagsID = windows.GUID{Data1: 0x632ce23b, Data2: 0x5167, Data3: 0x435c, Data4: [8]byte{0x86, 0xd7, 0xe9, 0x03, 0x68, 0x4a, 0xa8, 0x0c}}
var ownerData = []byte("godusevpn-ultra/location-portal-inbound/v1/58080/58443")

type blob struct {
	size uint32
	data *byte
}
type display struct{ name, description *uint16 }
type value struct {
	kind uint32
	data uintptr
}
type condition struct {
	key   windows.GUID
	match uint32
	value value
}
type action struct {
	kind uint32
	key  windows.GUID
}
type filter struct {
	key             windows.GUID
	display         display
	flags           uint32
	provider        *windows.GUID
	data            blob
	layer, sublayer windows.GUID
	weight          value
	count           uint32
	conditions      *condition
	action          action
	_               [4]byte
	context         windows.GUID
	reserved        *windows.GUID
	id              uint64
	effectiveWeight value
}
type provider struct {
	key     windows.GUID
	display display
	flags   uint32
	data    blob
	service *uint16
}
type sublayer struct {
	key      windows.GUID
	display  display
	flags    uint32
	provider *windows.GUID
	data     blob
	weight   uint16
}
type nativeEngine struct {
	handle  uintptr
	active  bool
	service *installedServiceLease
}
type specification struct {
	key, layer windows.GUID
	port       uint16
	data       []byte
}

func specifications() []specification {
	ports := []uint16{HTTPPort, HTTPSPort, HTTPPort, HTTPSPort}
	layers := []windows.GUID{in4, in4, in6, in6}
	result := make([]specification, 4)
	for i := range result {
		key := providerID
		key.Data1 += uint32(i + 2)
		result[i] = specification{key: key, layer: layers[i], port: ports[i], data: append(append([]byte(nil), ownerData...), byte('/'), byte('0'+i))}
	}
	return result
}
func conditionsFor(port uint16) []condition {
	return []condition{{key: protocolID, value: value{kind: 1, data: 6}}, {key: localPortID, value: value{kind: 2, data: uintptr(port)}}, {key: flagsID, match: 8, value: value{kind: 3, data: 1}}}
}
func blobMatches(actual blob, expected []byte) bool {
	return actual.size == uint32(len(expected)) && actual.data != nil && bytes.Equal(unsafe.Slice(actual.data, actual.size), expected)
}
func verifiedProvider(p *provider) bool {
	return p != nil && p.key == providerID && p.flags == 1 && fixedProviderService(p.service) && blobMatches(p.data, ownerData)
}
func verifiedSublayer(s *sublayer) bool {
	return s != nil && s.key == sublayerID && s.flags == 1 && s.provider != nil && *s.provider == providerID && s.weight == 0xffff && blobMatches(s.data, ownerData)
}
func verifiedFilter(f *filter, s specification) bool {
	if f == nil || f.key != s.key || f.provider == nil || *f.provider != providerID || f.layer != s.layer || f.sublayer != sublayerID || f.flags&^uint32(0x40) != 1 || !blobMatches(f.data, s.data) || f.weight.kind != 1 || f.weight.data != 15 || f.count != 3 || f.conditions == nil || f.action.kind != 0x1001 || f.action.key != (windows.GUID{}) || f.context != (windows.GUID{}) || f.reserved != nil {
		return false
	}
	want := conditionsFor(s.port)
	seen := [3]bool{}
	for _, actual := range unsafe.Slice(f.conditions, f.count) {
		found := false
		for i, expected := range want {
			if actual == expected && !seen[i] {
				seen[i] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return seen[0] && seen[1] && seen[2]
}

var dll = windows.NewLazySystemDLL("fwpuclnt.dll")
var fwOpen = dll.NewProc("FwpmEngineOpen0")
var fwClose = dll.NewProc("FwpmEngineClose0")
var fwBegin = dll.NewProc("FwpmTransactionBegin0")
var fwCommit = dll.NewProc("FwpmTransactionCommit0")
var fwAbort = dll.NewProc("FwpmTransactionAbort0")
var fwProviderGet = dll.NewProc("FwpmProviderGetByKey0")
var fwProviderAdd = dll.NewProc("FwpmProviderAdd0")
var fwProviderDelete = dll.NewProc("FwpmProviderDeleteByKey0")
var fwSublayerGet = dll.NewProc("FwpmSubLayerGetByKey0")
var fwSublayerAdd = dll.NewProc("FwpmSubLayerAdd0")
var fwSublayerDelete = dll.NewProc("FwpmSubLayerDeleteByKey0")
var fwFilterGet = dll.NewProc("FwpmFilterGetByKey0")
var fwFilterAdd = dll.NewProc("FwpmFilterAdd0")
var fwFilterDelete = dll.NewProc("FwpmFilterDeleteByKey0")
var fwEnumCreate = dll.NewProc("FwpmFilterCreateEnumHandle0")
var fwEnum = dll.NewProc("FwpmFilterEnum0")
var fwEnumDestroy = dll.NewProc("FwpmFilterDestroyEnumHandle0")
var fwFree = dll.NewProc("FwpmFreeMemory0")

const missingProvider syscall.Errno = 0x80320005
const missingSublayer syscall.Errno = 0x80320007
const missingFilter syscall.Errno = 0x80320003

//go:uintptrescapes
func call(proc *windows.LazyProc, args ...uintptr) error {
	result, _, _ := proc.Call(args...)
	if result != 0 {
		return syscall.Errno(result)
	}
	return nil
}
func free(pointer unsafe.Pointer)   { fwFree.Call(uintptr(pointer)); runtime.KeepAlive(pointer) }
func newNativeGate() (*Gate, error) { return &Gate{open: openNative}, nil }
func openNative(ctx context.Context) (engine, error) {
	if ctx == nil || ctx.Err() != nil || !windows.GetCurrentProcessToken().IsElevated() {
		return nil, ErrUnavailable
	}
	for _, proc := range []*windows.LazyProc{fwOpen, fwClose, fwBegin, fwCommit, fwAbort, fwProviderGet, fwProviderAdd, fwProviderDelete, fwSublayerGet, fwSublayerAdd, fwSublayerDelete, fwFilterGet, fwFilterAdd, fwFilterDelete, fwEnumCreate, fwEnum, fwEnumDestroy, fwFree} {
		if proc.Find() != nil {
			return nil, ErrUnavailable
		}
	}
	service, err := acquireInstalledService()
	if err != nil {
		return nil, ErrUnavailable
	}
	var handle uintptr
	if err := call(fwOpen, 0, 10, 0, 0, uintptr(unsafe.Pointer(&handle))); err != nil || handle == 0 {
		service.close()
		return nil, ErrUnavailable
	}
	return &nativeEngine{handle: handle, service: service}, nil
}
func (e *nativeEngine) close() {
	if e.handle != 0 {
		_ = call(fwClose, e.handle)
		e.handle = 0
	}
	if e.service != nil {
		e.service.close()
		e.service = nil
	}
}
func (e *nativeEngine) withTransaction(flags uintptr, operation func() error) error {
	if e.active || e.handle == 0 {
		return ErrUnknown
	}
	if err := call(fwBegin, e.handle, flags); err != nil {
		return ErrUnknown
	}
	e.active = true
	defer func() { e.active = false }()
	committed := false
	defer func() {
		if !committed {
			_ = call(fwAbort, e.handle)
		}
	}()
	if err := operation(); err != nil {
		return err
	}
	if err := call(fwCommit, e.handle); err != nil {
		return ErrUnknown
	}
	committed = true
	return nil
}
func (e *nativeEngine) transaction(operation func() error) error {
	return e.withTransaction(0, operation)
}
func (e *nativeEngine) snapshot() (s snapshot, err error) {
	if !e.active {
		err = e.withTransaction(1, func() error { var readErr error; s, readErr = e.readSnapshot(); return readErr })
		return s, err
	}
	return e.readSnapshot()
}
func (e *nativeEngine) readSnapshot() (snapshot, error) {
	s := snapshot{valid: true}
	if e.service == nil || e.service.verify() != nil {
		return s, ErrUnknown
	}
	var p *provider
	err := call(fwProviderGet, e.handle, uintptr(unsafe.Pointer(&providerID)), uintptr(unsafe.Pointer(&p)))
	providerPresent := err == nil
	if err != nil && !errors.Is(err, missingProvider) {
		return s, ErrUnknown
	}
	if err == nil {
		valid := verifiedProvider(p)
		free(unsafe.Pointer(&p))
		if !valid {
			return s, ErrUnknown
		}
	}
	var sub *sublayer
	err = call(fwSublayerGet, e.handle, uintptr(unsafe.Pointer(&sublayerID)), uintptr(unsafe.Pointer(&sub)))
	sublayerPresent := err == nil
	if err != nil && !errors.Is(err, missingSublayer) {
		return s, ErrUnknown
	}
	if err == nil {
		valid := verifiedSublayer(sub)
		free(unsafe.Pointer(&sub))
		if !valid {
			return s, ErrUnknown
		}
	}
	if providerPresent != sublayerPresent {
		return s, ErrUnknown
	}
	s.base = providerPresent
	expected := specifications()
	present := map[windows.GUID]bool{}
	for _, spec := range expected {
		var f *filter
		err = call(fwFilterGet, e.handle, uintptr(unsafe.Pointer(&spec.key)), uintptr(unsafe.Pointer(&f)))
		if errors.Is(err, missingFilter) {
			continue
		}
		if err != nil {
			return s, ErrUnknown
		}
		valid := verifiedFilter(f, spec)
		free(unsafe.Pointer(&f))
		if !valid {
			return s, ErrUnknown
		}
		present[spec.key] = true
		s.count++
	}
	if s.count != 0 && (!s.base || s.count != 4) {
		return s, ErrUnknown
	}
	// A nil template is documented to enumerate all layers. Inspect only this
	// provider/sublayer, but detect alien extra objects even on another layer.
	var enumeration uintptr
	if call(fwEnumCreate, e.handle, 0, uintptr(unsafe.Pointer(&enumeration))) != nil || enumeration == 0 {
		return s, ErrUnknown
	}
	defer call(fwEnumDestroy, e.handle, enumeration)
	seen := map[windows.GUID]bool{}
	total := 0
	for {
		var list **filter
		var count uint32
		if call(fwEnum, e.handle, enumeration, 128, uintptr(unsafe.Pointer(&list)), uintptr(unsafe.Pointer(&count))) != nil {
			return s, ErrUnknown
		}
		if count > 128 || count > 0 && list == nil {
			if list != nil {
				free(unsafe.Pointer(&list))
			}
			return s, ErrUnknown
		}
		invalid := false
		if count > 0 {
			for _, f := range unsafe.Slice(list, count) {
				if f == nil {
					invalid = true
					break
				}
				if f.sublayer != sublayerID && (f.provider == nil || *f.provider != providerID) {
					continue
				}
				if !present[f.key] || seen[f.key] {
					invalid = true
					break
				}
				seen[f.key] = true
				valid := false
				for _, spec := range expected {
					if f.key == spec.key && verifiedFilter(f, spec) {
						valid = true
						break
					}
				}
				if !valid {
					invalid = true
					break
				}
			}
		}
		if list != nil {
			free(unsafe.Pointer(&list))
		}
		total += int(count)
		if invalid || total > 65536 {
			return s, ErrUnknown
		}
		if count < 128 {
			break
		}
	}
	if len(seen) != s.count {
		return s, ErrUnknown
	}
	return s, nil
}
func (e *nativeEngine) install() error {
	if !e.active {
		return ErrUnknown
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return ErrUnknown
	}
	defer runtime.KeepAlive(sd)
	name := windows.StringToUTF16Ptr("GodUseVPN Ultra Device Portal inbound gate")
	s, err := e.readSnapshot()
	if err != nil || !s.absent() {
		return ErrUnknown
	}
	if !s.base {
		p := provider{key: providerID, display: display{name: name}, flags: 1, data: blob{uint32(len(ownerData)), &ownerData[0]}, service: windows.StringToUTF16Ptr(providerServiceName)}
		if call(fwProviderAdd, e.handle, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(sd))) != nil {
			return ErrUnknown
		}
		runtime.KeepAlive(p)
		sub := sublayer{key: sublayerID, display: display{name: name}, flags: 1, provider: &providerID, data: blob{uint32(len(ownerData)), &ownerData[0]}, weight: 0xffff}
		if call(fwSublayerAdd, e.handle, uintptr(unsafe.Pointer(&sub)), uintptr(unsafe.Pointer(sd))) != nil {
			return ErrUnknown
		}
		runtime.KeepAlive(sub)
	}
	for _, spec := range specifications() {
		conditions := conditionsFor(spec.port)
		f := filter{key: spec.key, display: display{name: name}, flags: 1, provider: &providerID, data: blob{uint32(len(spec.data)), &spec.data[0]}, layer: spec.layer, sublayer: sublayerID, weight: value{kind: 1, data: 15}, count: 3, conditions: &conditions[0], action: action{kind: 0x1001}}
		var id uint64
		if call(fwFilterAdd, e.handle, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(&id))) != nil {
			return ErrUnknown
		}
		runtime.KeepAlive(f)
		runtime.KeepAlive(conditions)
		runtime.KeepAlive(spec)
	}
	return nil
}
func (e *nativeEngine) remove() error {
	if !e.active {
		return ErrUnknown
	}
	s, err := e.readSnapshot()
	if err != nil || !s.absent() && !s.complete() {
		return ErrUnknown
	}
	if s.count > 0 {
		for _, spec := range specifications() {
			if call(fwFilterDelete, e.handle, uintptr(unsafe.Pointer(&spec.key))) != nil {
				return ErrUnknown
			}
		}
	}
	if s.base {
		if call(fwSublayerDelete, e.handle, uintptr(unsafe.Pointer(&sublayerID))) != nil {
			return ErrUnknown
		}
		if call(fwProviderDelete, e.handle, uintptr(unsafe.Pointer(&providerID))) != nil {
			return ErrUnknown
		}
	}
	return nil
}
func (e *nativeEngine) serviceSafe() error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return ErrServiceActive
	}
	defer windows.CloseServiceHandle(manager)
	name := windows.StringToUTF16Ptr("WebManagement")
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil
	}
	if err != nil {
		return ErrServiceActive
	}
	defer windows.CloseServiceHandle(service)
	var status windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed) != nil || needed != uint32(unsafe.Sizeof(status)) || status.CurrentState != windows.SERVICE_STOPPED || status.ProcessId != 0 {
		return ErrServiceActive
	}
	var size uint32
	err = windows.QueryServiceConfig(service, nil, 0, &size)
	if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || size < uint32(unsafe.Sizeof(windows.QUERY_SERVICE_CONFIG{})) || size > 65536 {
		return ErrServiceActive
	}
	data := make([]byte, size)
	if windows.QueryServiceConfig(service, (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&data[0])), size, &size) != nil {
		return ErrServiceActive
	}
	config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&data[0]))
	if !serviceStateSafe(status.CurrentState, status.ProcessId, config.StartType) {
		return ErrServiceActive
	}
	runtime.KeepAlive(data)
	return nil
}

func serviceStateSafe(state, pid, start uint32) bool {
	return state == windows.SERVICE_STOPPED && pid == 0 && start == windows.SERVICE_DISABLED
}
