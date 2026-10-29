//go:build windows && (amd64 || arm64)

package appfirewall

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These objects are intentionally unrelated to netmode/wfp's VPN guard.
var providerKey = mustGUID("b5f5775e-3e32-47a0-9c06-8d0e3a3946d1")
var sublayerKey = mustGUID("b5f5775f-3e32-47a0-9c06-8d0e3a3946d1")
var layers = map[string]windows.GUID{
	"out4": mustGUID("c38d57d1-05a7-4c33-904f-7fbceee60e82"), "out6": mustGUID("4a72393b-319f-44bc-84c3-ba54dcb3b6b4"),
	"in4": mustGUID("e1cd9fe7-f4b5-4273-96c0-592e487b8650"), "in6": mustGUID("a3b42c97-9f04-4672-b87e-cee9c483257f"),
}
var appCondition = mustGUID("d78e1e87-8644-4ea5-9437-d809ecefc971")
var addressCondition = mustGUID("b235ae9a-1d64-49b8-a44c-5ff3d9095045")
var localAddressCondition = mustGUID("d9ee00de-c1ef-4617-bfe3-ffd8f5a08957")
var flagsCondition = mustGUID("632ce23b-5167-435c-86d7-e903684aa80c")

// Native structures below follow fwpmtypes.h / fwptypes.h on Windows amd64/arm64.
// Kept local so this module cannot call or mutate the existing guard by accident.
type blob struct {
	Size uint32
	Data *byte
}
type display struct{ Name, Description *uint16 }
type value struct {
	Type  uint32
	Value uintptr
}

// WFP's value is a native tagged union. Read its pointer member as a pointer
// member, rather than arithmetic conversion from an integer (which would lose
// pointer provenance under checkptr and is flagged by go vet).
func unionPointer(v *value) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&v.Value)) }

// Tagged-union pointer members must live outside a movable goroutine stack.
// The caller must also KeepAlive the original pointer until the native call
// returns. KeepAlive alone guarantees liveness, not a stable heap allocation.
//
//go:noinline
//go:uintptrescapes
func stablePointer(p uintptr) uintptr { return p }

type condition struct {
	Key   windows.GUID
	Match uint32
	Value value
}
type action struct {
	Type uint32
	Key  windows.GUID
}
type filter struct {
	Key             windows.GUID
	Display         display
	Flags           uint32
	Provider        *windows.GUID
	Data            blob
	Layer, Sublayer windows.GUID
	Weight          value
	Count           uint32
	Conditions      *condition
	Action          action
	_               [4]byte
	Context         windows.GUID
	Reserved        *windows.GUID
	ID              uint64
	EffectiveWeight value
}
type provider struct {
	Key     windows.GUID
	Display display
	Flags   uint32
	Data    blob
	Service *uint16
}
type sublayer struct {
	Key      windows.GUID
	Display  display
	Flags    uint32
	Provider *windows.GUID
	Data     blob
	Weight   uint16
}
type enumTemplate struct {
	Provider    *windows.GUID
	Layer       windows.GUID
	Type, Flags uint32
	Context     uintptr
	Count       uint32
	Conditions  uintptr
	ActionMask  uint32
	Callout     *windows.GUID
}
type v4Mask struct{ Address, Mask uint32 }
type v6Mask struct {
	Address [16]byte
	Prefix  uint8
}
type metadata struct {
	Version int    `json:"version"`
	Owner   string `json:"owner"`
	Rule    Rule   `json:"rule"`
}

var dll = windows.NewLazySystemDLL("fwpuclnt.dll")
var pOpen = dll.NewProc("FwpmEngineOpen0")
var pClose = dll.NewProc("FwpmEngineClose0")
var pBegin = dll.NewProc("FwpmTransactionBegin0")
var pCommit = dll.NewProc("FwpmTransactionCommit0")
var pAbort = dll.NewProc("FwpmTransactionAbort0")
var pProviderAdd = dll.NewProc("FwpmProviderAdd0")
var pProviderGet = dll.NewProc("FwpmProviderGetByKey0")
var pSublayerAdd = dll.NewProc("FwpmSubLayerAdd0")
var pFilterAdd = dll.NewProc("FwpmFilterAdd0")
var pFilterDelete = dll.NewProc("FwpmFilterDeleteByKey0")
var pEnumCreate = dll.NewProc("FwpmFilterCreateEnumHandle0")
var pEnum = dll.NewProc("FwpmFilterEnum0")
var pEnumDestroy = dll.NewProc("FwpmFilterDestroyEnumHandle0")
var pAppID = dll.NewProc("FwpmGetAppIdFromFileName0")
var pFree = dll.NewProc("FwpmFreeMemory0")

//go:uintptrescapes
func call(proc *windows.LazyProc, args ...uintptr) error {
	r, _, _ := proc.Call(args...)
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}
func mustGUID(s string) windows.GUID {
	g, e := windows.GUIDFromString("{" + s + "}")
	if e != nil {
		panic(e)
	}
	return g
}
func keyFor(id, suffix string) windows.GUID {
	h := sha256.Sum256([]byte("godusevpn-ultra/appfirewall/v1/" + id + "/" + suffix))
	g := windows.GUID{Data1: binary.BigEndian.Uint32(h[:4]), Data2: binary.BigEndian.Uint16(h[4:6]), Data3: binary.BigEndian.Uint16(h[6:8])}
	copy(g.Data4[:], h[8:16])
	g.Data3 = (g.Data3 & 0x0fff) | 0x5000
	g.Data4[0] = (g.Data4[0] & 0x3f) | 0x80
	return g
}

type windowsBackend struct{}

func newBackend() backend              { return windowsBackend{} }
func (windowsBackend) supported() bool { return true }
func (windowsBackend) elevated() bool  { return windows.GetCurrentProcessToken().IsElevated() }
func systemDirectory() string {
	path, err := windows.GetWindowsDirectory()
	if err != nil {
		return ""
	}
	return path
}
func isReparsePoint(path string) (bool, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := windows.GetFileAttributes(p)
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, err
}
func open() (uintptr, error) {
	var h uintptr
	err := call(pOpen, 0, 10, 0, 0, uintptr(unsafe.Pointer(&h)))
	return h, err
}
func free(p unsafe.Pointer) { pFree.Call(uintptr(p)); runtime.KeepAlive(p) }

func addressKeyFor(r Rule, s filterSpec) windows.GUID {
	// An incoming discovery packet's destination is the local multicast/broadcast
	// address; its remote address is the unicast sender, unlike outgoing traffic.
	if r.Mode == "block_discovery" && (s.Layer == "in4" || s.Layer == "in6") {
		return localAddressCondition
	}
	return addressCondition
}
func appID(path string) ([]byte, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	var b *blob
	if e = call(pAppID, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&b))); e != nil {
		return nil, e
	}
	defer free(unsafe.Pointer(&b))
	if b == nil || b.Size == 0 || b.Size > 65536 || b.Data == nil {
		return nil, errors.New("invalid WFP app identity")
	}
	return bytes.Clone(unsafe.Slice(b.Data, b.Size)), nil
}
func (w windowsBackend) snapshot() ([]installed, []string, error) {
	h, e := open()
	if e != nil {
		return nil, nil, e
	}
	defer call(pClose, h)
	return snapshot(h)
}

func snapshot(h uintptr) ([]installed, []string, error) {
	var out []installed
	var warnings []string
	providerOK := false
	var p *provider
	e := call(pProviderGet, h, uintptr(unsafe.Pointer(&providerKey)), uintptr(unsafe.Pointer(&p)))
	if e == nil && p != nil {
		providerOK = p.Flags&1 != 0 && p.Flags&0x10 == 0
		free(unsafe.Pointer(&p))
		if !providerOK {
			warnings = append(warnings, "应用规则提供者未持久启用；不能认为规则生效")
		}
	} else if errors.Is(e, syscall.Errno(0x80320005)) {
		return nil, nil, nil
	} else if e != nil {
		return nil, nil, e
	} else {
		return nil, nil, errors.New("WFP provider query returned no object")
	}
	for _, layer := range layers {
		tpl := enumTemplate{Provider: &providerKey, Layer: layer, Type: 1, Flags: 0x18, ActionMask: 0xffffffff}
		var eh uintptr
		if e := call(pEnumCreate, h, uintptr(unsafe.Pointer(&tpl)), uintptr(unsafe.Pointer(&eh))); e != nil {
			return nil, nil, e
		}
		readErr := func() error {
			defer call(pEnumDestroy, h, eh)
			for {
				var entries **filter
				var n uint32
				if e := call(pEnum, h, eh, 128, uintptr(unsafe.Pointer(&entries)), uintptr(unsafe.Pointer(&n))); e != nil {
					return e
				}
				if n > 128 || n > 0 && entries == nil {
					return errors.New("invalid WFP enumeration")
				}
				if n == 0 {
					if entries != nil {
						free(unsafe.Pointer(&entries))
					}
					break
				}
				for _, f := range unsafe.Slice(entries, n) {
					if !isOwned(f) {
						warnings = append(warnings, "发现归属异常的应用过滤器；未触碰它")
						out = append(out, installed{Unknown: true})
						continue
					}
					m, err := readMetadata(f)
					if err != nil {
						warnings = append(warnings, "发现无法识别的专用过滤器；未将其计为保护，需管理员检查")
						out = append(out, installed{Key: f.Key.String(), Unknown: true})
						continue
					}
					matches := false
					for _, s := range specs(m.Rule) {
						if keyFor(m.Rule.ID, s.Suffix) == f.Key {
							matches = providerOK && matchesFilter(f, m.Rule, s)
							break
						}
					}
					out = append(out, installed{Rule: m.Rule, Key: f.Key.String(), Matches: matches})
				}
				free(unsafe.Pointer(&entries))
				if n < 128 {
					break
				}
			}
			return nil
		}()
		if readErr != nil {
			return nil, nil, readErr
		}
	}
	return out, warnings, nil
}

func readMetadata(f *filter) (metadata, error) {
	var m metadata
	if f.Data.Size == 0 || f.Data.Size > 16384 || f.Data.Data == nil {
		return m, errors.New("missing metadata")
	}
	err := json.Unmarshal(unsafe.Slice(f.Data.Data, f.Data.Size), &m)
	if err != nil || m.Version != 1 || m.Owner != "godusevpn-ultra/appfirewall" {
		return m, errors.New("unknown metadata")
	}
	if err = validateRule(m.Rule, false); err != nil {
		return m, err
	}
	return m, nil
}

func matchesFilter(f *filter, r Rule, s filterSpec) bool {
	app, err := appID(r.Program)
	if err != nil {
		return false
	}
	return matchesFilterWithIdentity(f, r, s, app)
}

func isOwned(f *filter) bool {
	return f != nil && f.Provider != nil && *f.Provider == providerKey && f.Sublayer == sublayerKey
}

func matchesFilterWithIdentity(f *filter, r Rule, s filterSpec, app []byte) bool {
	if !isOwned(f) || len(app) == 0 || f.Layer != layers[s.Layer] || f.Action.Type != 0x1001 || f.Flags&^uint32(0x40) != 1 || f.Count < 2 || f.Count > 3 || f.Conditions == nil {
		return false
	}
	wantAddr := s.Prefix != "" && s.Prefix != "v6"
	wantCount := uint32(2)
	if wantAddr {
		wantCount++
	}
	if f.Count != wantCount {
		return false
	}
	seenApp, seenFlags, seenAddr := false, false, false
	for _, c := range unsafe.Slice(f.Conditions, f.Count) {
		switch c.Key {
		case appCondition:
			if seenApp || c.Match != 0 || c.Value.Type != 12 || c.Value.Value == 0 {
				return false
			}
			b := (*blob)(unionPointer(&c.Value))
			if b.Size > 65536 || b.Data == nil || !bytes.Equal(app, unsafe.Slice(b.Data, b.Size)) {
				return false
			}
			seenApp = true
		case flagsCondition:
			if seenFlags || c.Match != 8 || c.Value.Type != 3 || c.Value.Value != 1 {
				return false
			}
			seenFlags = true // FLAGS_NONE_SET(IS_LOOPBACK).
		case addressCondition, localAddressCondition:
			if c.Key != addressKeyFor(r, s) || seenAddr || !wantAddr || c.Match != 0 || c.Value.Value == 0 {
				return false
			}
			prefix, err := netip.ParsePrefix(s.Prefix)
			if err != nil {
				return false
			}
			if prefix.Addr().Is4() {
				if c.Value.Type != 0x100 {
					return false
				}
				m := (*v4Mask)(unionPointer(&c.Value))
				a := prefix.Addr().As4()
				if m.Address != binary.BigEndian.Uint32(a[:]) || m.Mask != uint32(0xffffffff<<(32-prefix.Bits())) {
					return false
				}
			} else {
				if c.Value.Type != 0x101 {
					return false
				}
				m := (*v6Mask)(unionPointer(&c.Value))
				if m.Address != prefix.Addr().As16() || int(m.Prefix) != prefix.Bits() {
					return false
				}
			}
			seenAddr = true
		default:
			return false
		}
	}
	return seenApp && seenFlags && seenAddr == wantAddr
}

func transact(fn func(uintptr) error) error {
	h, e := open()
	if e != nil {
		return e
	}
	defer call(pClose, h)
	return runTransaction(transactionHooks{begin: func() error { return call(pBegin, h, 0) }, commit: func() error { return call(pCommit, h) }, abort: func() error { return call(pAbort, h) }}, func() error { return fn(h) })
}

func ensureBase(h uintptr) error {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return err
	}
	name, _ := windows.UTF16PtrFromString("godusevpn ultra optional application privacy")
	p := provider{Key: providerKey, Display: display{Name: name}, Flags: 1}
	if err = call(pProviderAdd, h, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(sd))); err != nil && !errors.Is(err, syscall.Errno(0x80320009)) {
		return err
	}
	s := sublayer{Key: sublayerKey, Display: display{Name: name}, Flags: 1, Provider: &providerKey, Weight: 0xfffe}
	err = call(pSublayerAdd, h, uintptr(unsafe.Pointer(&s)), uintptr(unsafe.Pointer(sd)))
	runtime.KeepAlive(sd)
	if errors.Is(err, syscall.Errno(0x80320009)) {
		return nil
	}
	return err
}

// deleteID enumerates only our provider, checks BOTH ownership keys, then checks
// metadata and the deterministic rule key before deleting. No global cleanup.
func deleteID(h uintptr, id string) error {
	fs, _, err := snapshot(h)
	if err != nil {
		return err
	}
	for _, f := range fs {
		if f.Rule.ID != id {
			continue
		}
		g, err := windows.GUIDFromString(f.Key)
		if err != nil {
			return err
		}
		owned := false
		for _, s := range specs(f.Rule) {
			if keyFor(id, s.Suffix) == g {
				owned = true
				break
			}
		}
		if !owned {
			return errors.New("规则键异常；拒绝删除未知对象")
		}
		if err = call(pFilterDelete, h, uintptr(unsafe.Pointer(&g))); err != nil {
			return err
		}
	}
	return nil
}

func (windowsBackend) remove(id string) error {
	return transact(func(h uintptr) error { return deleteID(h, id) })
}
func (windowsBackend) replace(r Rule) error {
	return transact(func(h uintptr) error {
		if err := deleteID(h, r.ID); err != nil {
			return err
		}
		if err := ensureBase(h); err != nil {
			return err
		}
		for _, s := range specs(r) {
			if err := addFilter(h, r, s); err != nil {
				return err
			}
		}
		return nil
	})
}

func addFilter(h uintptr, r Rule, s filterSpec) error {
	app, err := appID(r.Program)
	if err != nil {
		return err
	}
	appBlob := blob{Size: uint32(len(app)), Data: &app[0]}
	conditions := []condition{{Key: appCondition, Value: value{Type: 12, Value: stablePointer(uintptr(unsafe.Pointer(&appBlob)))}}, {Key: flagsCondition, Match: 8, Value: value{Type: 3, Value: 1}}}
	var m4 v4Mask
	var m6 v6Mask
	if s.Prefix != "" && s.Prefix != "v6" {
		p, err := netip.ParsePrefix(s.Prefix)
		if err != nil {
			return err
		}
		c := condition{Key: addressKeyFor(r, s)}
		if p.Addr().Is4() {
			a := p.Addr().As4()
			m4 = v4Mask{Address: binary.BigEndian.Uint32(a[:]), Mask: uint32(0xffffffff << (32 - p.Bits()))}
			c.Value = value{Type: 0x100, Value: stablePointer(uintptr(unsafe.Pointer(&m4)))}
		} else {
			m6 = v6Mask{Address: p.Addr().As16(), Prefix: uint8(p.Bits())}
			c.Value = value{Type: 0x101, Value: stablePointer(uintptr(unsafe.Pointer(&m6)))}
		}
		conditions = append(conditions, c)
	}
	data, err := json.Marshal(metadata{Version: 1, Owner: "godusevpn-ultra/appfirewall", Rule: r})
	if err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString("godusevpn ultra app privacy " + r.ID)
	if err != nil {
		return err
	}
	f := filter{Key: keyFor(r.ID, s.Suffix), Display: display{Name: name}, Flags: 1, Provider: &providerKey, Data: blob{Size: uint32(len(data)), Data: &data[0]}, Layer: layers[s.Layer], Sublayer: sublayerKey, Weight: value{Type: 1, Value: 15}, Count: uint32(len(conditions)), Conditions: &conditions[0], Action: action{Type: 0x1001}}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return err
	}
	var filterID uint64
	err = call(pFilterAdd, h, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(&filterID)))
	runtime.KeepAlive(app)
	runtime.KeepAlive(&appBlob)
	runtime.KeepAlive(conditions)
	runtime.KeepAlive(&m4)
	runtime.KeepAlive(&m6)
	runtime.KeepAlive(sd)
	runtime.KeepAlive(data)
	if err != nil {
		return fmt.Errorf("添加 %s: %w", s.Suffix, err)
	}
	return nil
}
