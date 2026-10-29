//go:build windows && (amd64 || arm64)

package browserengineguard

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This namespace is intentionally distinct from both the VPN guard and the
// general optional appfirewall. No operation enumerates/deletes another owner.
var providerID = guid("6e6f48e6-ae4c-4f26-bb9f-e90fda8d13b1")
var sublayerID = guid("6e6f48e7-ae4c-4f26-bb9f-e90fda8d13b1")
var layerIDs = map[string]windows.GUID{"out4": guid("c38d57d1-05a7-4c33-904f-7fbceee60e82"), "out6": guid("4a72393b-319f-44bc-84c3-ba54dcb3b6b4"), "in4": guid("e1cd9fe7-f4b5-4273-96c0-592e487b8650"), "in6": guid("a3b42c97-9f04-4672-b87e-cee9c483257f")}
var appIDCondition = guid("d78e1e87-8644-4ea5-9437-d809ecefc971")
var remoteAddressCondition = guid("b235ae9a-1d64-49b8-a44c-5ff3d9095045")
var localAddressCondition = guid("d9ee00de-c1ef-4617-bfe3-ffd8f5a08957")
var remotePortCondition = guid("c35a604d-d22b-4e1a-91b4-68f674ee674b")
var localPortCondition = guid("0c1ba1af-5765-453f-af22-a8f791ac775b")
var protocolCondition = guid("3971ef2b-623e-4f9a-8cb1-6e79b806b9a7")

type wfpBlob struct {
	size uint32
	data *byte
}
type wfpDisplay struct{ name, description *uint16 }
type wfpValue struct {
	kind  uint32
	value uintptr
}
type wfpCondition struct {
	key   windows.GUID
	match uint32
	value wfpValue
}
type wfpAction struct {
	kind uint32
	key  windows.GUID
}
type wfpFilter struct {
	key             windows.GUID
	display         wfpDisplay
	flags           uint32
	provider        *windows.GUID
	data            wfpBlob
	layer, sublayer windows.GUID
	weight          wfpValue
	count           uint32
	conditions      *wfpCondition
	action          wfpAction
	_               [4]byte
	context         windows.GUID
	reserved        *windows.GUID
	id              uint64
	effectiveWeight wfpValue
}
type wfpProvider struct {
	key     windows.GUID
	display wfpDisplay
	flags   uint32
	data    wfpBlob
	service *uint16
}
type wfpSublayer struct {
	key      windows.GUID
	display  wfpDisplay
	flags    uint32
	provider *windows.GUID
	data     wfpBlob
	weight   uint16
}

var fwpuclnt = windows.NewLazySystemDLL("fwpuclnt.dll")
var fwOpen = fwpuclnt.NewProc("FwpmEngineOpen0")
var fwClose = fwpuclnt.NewProc("FwpmEngineClose0")
var fwBegin = fwpuclnt.NewProc("FwpmTransactionBegin0")
var fwCommit = fwpuclnt.NewProc("FwpmTransactionCommit0")
var fwAbort = fwpuclnt.NewProc("FwpmTransactionAbort0")
var fwProviderAdd = fwpuclnt.NewProc("FwpmProviderAdd0")
var fwProviderGet = fwpuclnt.NewProc("FwpmProviderGetByKey0")
var fwSublayerAdd = fwpuclnt.NewProc("FwpmSubLayerAdd0")
var fwSublayerGet = fwpuclnt.NewProc("FwpmSubLayerGetByKey0")
var fwFilterAdd = fwpuclnt.NewProc("FwpmFilterAdd0")
var fwFilterGet = fwpuclnt.NewProc("FwpmFilterGetByKey0")
var fwFilterDelete = fwpuclnt.NewProc("FwpmFilterDeleteByKey0")
var fwAppID = fwpuclnt.NewProc("FwpmGetAppIdFromFileName0")
var fwFree = fwpuclnt.NewProc("FwpmFreeMemory0")

const filterMissing syscall.Errno = 0x80320003
const objectMissing syscall.Errno = 0x80320005
const sublayerMissing syscall.Errno = 0x80320007

func guid(raw string) windows.GUID {
	value, err := windows.GUIDFromString("{" + raw + "}")
	if err != nil {
		panic(err)
	}
	return value
}

//go:uintptrescapes
func nativeCall(proc *windows.LazyProc, args ...uintptr) error {
	result, _, _ := proc.Call(args...)
	if result != 0 {
		return syscall.Errno(result)
	}
	return nil
}

//go:noinline
//go:uintptrescapes
func heapPointer(pointer uintptr) uintptr { return pointer }
func ptrValue(value *wfpValue) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&value.value))
}
func free(pointer unsafe.Pointer) { fwFree.Call(uintptr(pointer)); runtime.KeepAlive(pointer) }
func openWFP() (uintptr, error) {
	var handle uintptr
	err := nativeCall(fwOpen, 0, 10, 0, 0, uintptr(unsafe.Pointer(&handle)))
	return handle, err
}

type nativeFirewall struct{}

func newNativeFirewall() firewall { return nativeFirewall{} }
func transaction(change func(uintptr) error) error {
	handle, err := openWFP()
	if err != nil {
		return err
	}
	defer nativeCall(fwClose, handle)
	if err = nativeCall(fwBegin, handle, 0); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			nativeCall(fwAbort, handle)
		}
	}()
	if err = change(handle); err != nil {
		return err
	}
	if err = nativeCall(fwCommit, handle); err != nil {
		return err
	}
	committed = true
	return nil
}

var ownerData = []byte("godusevpn-ultra/browser-engine-guard/v1")

func ownedBlob(blob wfpBlob) bool {
	return blob.size == uint32(len(ownerData)) && blob.data != nil && bytes.Equal(unsafe.Slice(blob.data, blob.size), ownerData)
}
func baseVerified(handle uintptr) (bool, error) {
	var provider *wfpProvider
	err := nativeCall(fwProviderGet, handle, uintptr(unsafe.Pointer(&providerID)), uintptr(unsafe.Pointer(&provider)))
	if errors.Is(err, objectMissing) {
		return false, nil
	}
	if err != nil || provider == nil {
		return false, ErrUnknown
	}
	defer free(unsafe.Pointer(&provider))
	if provider.key != providerID || provider.flags != 1 || !ownedBlob(provider.data) {
		return false, ErrUnknown
	}
	var sublayer *wfpSublayer
	err = nativeCall(fwSublayerGet, handle, uintptr(unsafe.Pointer(&sublayerID)), uintptr(unsafe.Pointer(&sublayer)))
	if errors.Is(err, sublayerMissing) {
		return false, nil
	}
	if err != nil || sublayer == nil {
		return false, ErrUnknown
	}
	defer free(unsafe.Pointer(&sublayer))
	if sublayer.key != sublayerID || sublayer.flags != 1 || sublayer.provider == nil || *sublayer.provider != providerID || sublayer.weight != 0xfffd || !ownedBlob(sublayer.data) {
		return false, ErrUnknown
	}
	return true, nil
}
func ensureBase(handle uintptr) error {
	verified, err := baseVerified(handle)
	if err != nil {
		return err
	}
	if verified {
		return nil
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return err
	}
	name := windows.StringToUTF16Ptr("GodUseVPN Ultra browser session isolation")
	provider := wfpProvider{key: providerID, display: wfpDisplay{name: name}, flags: 1, data: wfpBlob{uint32(len(ownerData)), &ownerData[0]}}
	if err = nativeCall(fwProviderAdd, handle, uintptr(unsafe.Pointer(&provider)), uintptr(unsafe.Pointer(sd))); err != nil && !errors.Is(err, syscall.Errno(0x80320009)) {
		return err
	}
	sublayer := wfpSublayer{key: sublayerID, display: wfpDisplay{name: name}, flags: 1, provider: &providerID, data: wfpBlob{uint32(len(ownerData)), &ownerData[0]}, weight: 0xfffd}
	if err = nativeCall(fwSublayerAdd, handle, uintptr(unsafe.Pointer(&sublayer)), uintptr(unsafe.Pointer(sd))); err != nil && !errors.Is(err, syscall.Errno(0x80320009)) {
		return err
	}
	runtime.KeepAlive(sd)
	verified, err = baseVerified(handle)
	if err != nil || !verified {
		return ErrUnknown
	}
	return nil
}
func applicationID(path string) ([]byte, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var result *wfpBlob
	if err = nativeCall(fwAppID, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&result))); err != nil {
		return nil, err
	}
	defer free(unsafe.Pointer(&result))
	if result == nil || result.size == 0 || result.size > 65536 || result.data == nil {
		return nil, ErrUnknown
	}
	return bytes.Clone(unsafe.Slice(result.data, result.size)), nil
}
func ruleKey(request InstallRequest, program string, index int) windows.GUID {
	sum := sha256.Sum256([]byte("godusevpn-ultra/browser-engine-guard/v1/" + request.SessionID + "/" + strings.ToLower(program) + "/" + fmt.Sprint(index)))
	id := windows.GUID{Data1: binary.BigEndian.Uint32(sum[:4]), Data2: binary.BigEndian.Uint16(sum[4:6]), Data3: binary.BigEndian.Uint16(sum[6:8])}
	copy(id.Data4[:], sum[8:16])
	id.Data3 = (id.Data3 & 0xfff) | 0x5000
	id.Data4[0] = (id.Data4[0] & 0x3f) | 0x80
	return id
}

type ruleMetadata struct {
	Owner   string         `json:"owner"`
	Request InstallRequest `json:"request"`
	Program string         `json:"program"`
	Index   int            `json:"index"`
}

func ruleData(r InstallRequest, p string, i int) []byte {
	data, _ := json.Marshal(ruleMetadata{"godusevpn-ultra/browser-engine-guard/v1", r, p, i})
	return data
}
func extraCondition(spec filterSpec) (wfpCondition, bool) {
	c := wfpCondition{match: 10}
	switch spec.field {
	case "remote-address-not":
		c.key = remoteAddressCondition
		c.value = wfpValue{3, uintptr(spec.number)}
	case "local-address-not":
		c.key = localAddressCondition
		c.value = wfpValue{3, uintptr(spec.number)}
	case "remote-port-not":
		c.key = remotePortCondition
		c.value = wfpValue{2, uintptr(spec.number)}
	case "local-port-not":
		c.key = localPortCondition
		c.value = wfpValue{2, uintptr(spec.number)}
	case "protocol-not":
		c.key = protocolCondition
		c.value = wfpValue{1, uintptr(spec.number)}
	default:
		return c, false
	}
	return c, true
}
func addRule(handle uintptr, request InstallRequest, program string, index int, spec filterSpec) error {
	app, err := applicationID(program)
	if err != nil {
		return err
	}
	appBlob := wfpBlob{uint32(len(app)), &app[0]}
	conditions := []wfpCondition{{key: appIDCondition, value: wfpValue{12, heapPointer(uintptr(unsafe.Pointer(&appBlob)))}}}
	if extra, exists := extraCondition(spec); exists {
		conditions = append(conditions, extra)
	}
	data := ruleData(request, program, index)
	name := windows.StringToUTF16Ptr("GodUseVPN Ultra isolated browser " + request.SessionID[:12])
	filter := wfpFilter{key: ruleKey(request, program, index), display: wfpDisplay{name: name}, flags: 1, provider: &providerID, data: wfpBlob{uint32(len(data)), &data[0]}, layer: layerIDs[spec.layer], sublayer: sublayerID, weight: wfpValue{1, 15}, count: uint32(len(conditions)), conditions: &conditions[0], action: wfpAction{kind: 0x1001}}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return err
	}
	var id uint64
	err = nativeCall(fwFilterAdd, handle, uintptr(unsafe.Pointer(&filter)), uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(&id)))
	runtime.KeepAlive(app)
	runtime.KeepAlive(&appBlob)
	runtime.KeepAlive(conditions)
	runtime.KeepAlive(data)
	runtime.KeepAlive(sd)
	return err
}
func (nativeFirewall) install(request InstallRequest, programs []string) error {
	if len(programs) == 0 || len(programs) > 256 {
		return ErrRefused
	}
	return transaction(func(handle uintptr) error {
		if err := ensureBase(handle); err != nil {
			return err
		}
		for _, program := range programs {
			for index, spec := range specs(request) {
				if err := addRule(handle, request, program, index, spec); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
func verifyRule(filter *wfpFilter, request InstallRequest, program string, index int, spec filterSpec, app []byte) bool {
	data := ruleData(request, program, index)
	extra, hasExtra := extraCondition(spec)
	count := uint32(1)
	if hasExtra {
		count++
	}
	if filter == nil || filter.key != ruleKey(request, program, index) || filter.flags != 1 || filter.provider == nil || *filter.provider != providerID || filter.layer != layerIDs[spec.layer] || filter.sublayer != sublayerID || filter.action.kind != 0x1001 || filter.weight.kind != 1 || filter.weight.value != 15 || filter.count != count || filter.conditions == nil || filter.data.size != uint32(len(data)) || filter.data.data == nil || !bytes.Equal(unsafe.Slice(filter.data.data, filter.data.size), data) {
		return false
	}
	conditions := unsafe.Slice(filter.conditions, filter.count)
	seenApp, seenExtra := false, false
	for _, condition := range conditions {
		if condition.key == appIDCondition {
			if seenApp || condition.match != 0 || condition.value.kind != 12 || ptrValue(&condition.value) == nil {
				return false
			}
			blob := (*wfpBlob)(ptrValue(&condition.value))
			if blob.size != uint32(len(app)) || blob.data == nil || !bytes.Equal(unsafe.Slice(blob.data, blob.size), app) {
				return false
			}
			seenApp = true
		} else if hasExtra && condition.key == extra.key {
			if seenExtra || condition.match != extra.match || condition.value != extra.value {
				return false
			}
			seenExtra = true
		} else {
			return false
		}
	}
	return seenApp && seenExtra == hasExtra
}
func verifyAt(handle uintptr, request InstallRequest, programs []string) (bool, error) {
	base, err := baseVerified(handle)
	if err != nil {
		return false, err
	}
	present, missing := 0, 0
	for _, program := range programs {
		app, err := applicationID(program)
		if err != nil {
			return false, ErrUnknown
		}
		for index, spec := range specs(request) {
			key := ruleKey(request, program, index)
			var filter *wfpFilter
			err := nativeCall(fwFilterGet, handle, uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&filter)))
			if errors.Is(err, filterMissing) {
				missing++
				continue
			}
			if err != nil || filter == nil {
				return false, ErrUnknown
			}
			valid := verifyRule(filter, request, program, index, spec, app)
			free(unsafe.Pointer(&filter))
			if !valid {
				return false, ErrUnknown
			}
			present++
		}
	}
	if present == 0 {
		return false, nil
	}
	if !base || missing != 0 {
		return false, ErrUnknown
	}
	return true, nil
}
func (nativeFirewall) verify(request InstallRequest, programs []string) (bool, error) {
	handle, err := openWFP()
	if err != nil {
		return false, ErrUnknown
	}
	defer nativeCall(fwClose, handle)
	return verifyAt(handle, request, programs)
}
func (nativeFirewall) remove(request InstallRequest, programs []string) error {
	return transaction(func(handle uintptr) error {
		// All expected keys must be intact or absent before deletion. A partial or
		// altered set is unknown and preserved for explicit recovery, not broadened.
		present, err := verifyAt(handle, request, programs)
		if err != nil {
			return err
		}
		if !present {
			return nil
		}
		for _, program := range programs {
			for index := range specs(request) {
				key := ruleKey(request, program, index)
				if err := nativeCall(fwFilterDelete, handle, uintptr(unsafe.Pointer(&key))); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
