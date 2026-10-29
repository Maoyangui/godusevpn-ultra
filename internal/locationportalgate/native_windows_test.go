//go:build windows && (amd64 || arm64)

package locationportalgate

import (
	"context"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These tests only construct tagged unions/SCM values in memory. They never
// open WFP/SCM, load a procedure, create a listener, or run the real product.
func syntheticFilter(s specification) (*filter, []condition) {
	conditions := conditionsFor(s.port)
	return &filter{key: s.key, flags: 1, provider: &providerID, data: blob{uint32(len(s.data)), &s.data[0]}, layer: s.layer, sublayer: sublayerID, weight: value{1, 15}, count: 3, conditions: &conditions[0], action: action{kind: 0x1001}}, conditions
}
func TestExactFourRulesUseBothFamiliesAndOnlyFixedTCPPorts(t *testing.T) {
	specs := specifications()
	if len(specs) != 4 {
		t.Fatal(specs)
	}
	seen := map[windows.GUID]bool{}
	coverage := map[windows.GUID]map[uint16]bool{}
	for _, s := range specs {
		if seen[s.key] || s.key == providerID || s.key == sublayerID {
			t.Fatal("namespace collision")
		}
		seen[s.key] = true
		if s.layer != in4 && s.layer != in6 || s.port != HTTPPort && s.port != HTTPSPort {
			t.Fatal("unbounded target", s)
		}
		if coverage[s.layer] == nil {
			coverage[s.layer] = map[uint16]bool{}
		}
		coverage[s.layer][s.port] = true
		f, conditions := syntheticFilter(s)
		if !verifiedFilter(f, s) {
			t.Fatal("exact rule rejected")
		}
		if conditions[0].key != protocolID || conditions[0].value.kind != 1 || conditions[0].value.data != 6 || conditions[1].key != localPortID || conditions[1].value.kind != 2 || conditions[1].value.data != uintptr(s.port) || conditions[2].key != flagsID || conditions[2].match != 8 || conditions[2].value.kind != 3 || conditions[2].value.data != 1 {
			t.Fatal("wrong union or loopback condition")
		}
	}
	if len(coverage) != 2 || len(coverage[in4]) != 2 || len(coverage[in6]) != 2 {
		t.Fatal("family/port gap", coverage)
	}
}
func TestReadbackRejectsWeakenedAlienOrDuplicateFilterFields(t *testing.T) {
	s := specifications()[0]
	for _, kind := range []string{"key", "provider-nil", "provider", "layer", "sublayer", "boot-time", "soft-action", "disabled", "unknown-flag", "metadata-size", "metadata", "weight-kind", "weight", "count", "nil-conditions", "permit", "action-key", "context", "reserved", "udp", "wrong-port", "wrong-loopback-match", "loopback-flags", "union-type", "duplicate", "extra-key"} {
		t.Run(kind, func(t *testing.T) {
			f, c := syntheticFilter(s)
			alien := providerID
			alien.Data1++
			switch kind {
			case "key":
				f.key = alien
			case "provider-nil":
				f.provider = nil
			case "provider":
				f.provider = &alien
			case "layer":
				f.layer = in6
			case "sublayer":
				f.sublayer = providerID
			case "boot-time":
				f.flags |= 2
			case "soft-action":
				f.flags |= 8
			case "disabled":
				f.flags |= 0x20
			case "unknown-flag":
				f.flags |= 0x80
			case "metadata-size":
				f.data.size++
			case "metadata":
				copy := append([]byte(nil), s.data...)
				copy[0] ^= 1
				f.data.data = &copy[0]
			case "weight-kind":
				f.weight.kind = 2
			case "weight":
				f.weight.data = 14
			case "count":
				f.count = 2
			case "nil-conditions":
				f.conditions = nil
			case "permit":
				f.action.kind = 0x1002
			case "action-key":
				f.action.key = alien
			case "context":
				f.context = alien
			case "reserved":
				f.reserved = &alien
			case "udp":
				c[0].value.data = 17
			case "wrong-port":
				c[1].value.data = 443
			case "wrong-loopback-match":
				c[2].match = 6
			case "loopback-flags":
				c[2].value.data = 0
			case "union-type":
				c[1].value.kind = 3
			case "duplicate":
				c[2] = c[1]
			case "extra-key":
				c[2].key = alien
			}
			if verifiedFilter(f, s) {
				t.Fatal("unverified protection accepted", kind)
			}
		})
	}
	f, _ := syntheticFilter(s)
	f.flags |= 0x40
	if !verifiedFilter(f, s) {
		t.Fatal("BFE indexed flag changes no protection and must be readable")
	}
	f, c := syntheticFilter(s)
	c[0], c[2] = c[2], c[0]
	if !verifiedFilter(f, s) {
		t.Fatal("order-independent equivalent conditions rejected")
	}
}
func TestProviderAndSublayerHavePrivateFixedPersistentIdentity(t *testing.T) {
	p := &provider{key: providerID, flags: 1, data: blob{uint32(len(ownerData)), &ownerData[0]}, service: windows.StringToUTF16Ptr(providerServiceName)}
	s := &sublayer{key: sublayerID, flags: 1, provider: &providerID, data: p.data, weight: 0xffff}
	if !verifiedProvider(p) || !verifiedSublayer(s) {
		t.Fatal("exact base rejected")
	}
	p.flags |= 0x10
	if verifiedProvider(p) {
		t.Fatal("disabled provider accepted")
	}
	p.flags = 1
	p.service = nil
	if verifiedProvider(p) {
		t.Fatal("unassociated provider accepted")
	}
	s.weight--
	if verifiedSublayer(s) {
		t.Fatal("weakened sublayer accepted")
	}
	s.weight++
	s.provider = nil
	if verifiedSublayer(s) {
		t.Fatal("unowned sublayer accepted")
	}
	if verifiedProvider(nil) || verifiedSublayer(nil) || blobMatches(blob{}, ownerData) {
		t.Fatal("null base accepted")
	}
}
func TestSCMRemovalProofRequiresDisabledStoppedAndNoPID(t *testing.T) {
	if !serviceStateSafe(windows.SERVICE_STOPPED, 0, windows.SERVICE_DISABLED) {
		t.Fatal("safe state rejected")
	}
	for _, test := range [][3]uint32{{windows.SERVICE_RUNNING, 0, windows.SERVICE_DISABLED}, {windows.SERVICE_STOP_PENDING, 1, windows.SERVICE_DISABLED}, {windows.SERVICE_STOPPED, 1, windows.SERVICE_DISABLED}, {windows.SERVICE_STOPPED, 0, windows.SERVICE_DEMAND_START}, {windows.SERVICE_STOPPED, 0, windows.SERVICE_AUTO_START}, {0, 0, 0}} {
		if serviceStateSafe(test[0], test[1], test[2]) {
			t.Fatal("unsafe service proof accepted", test)
		}
	}
}
func TestWFP64BitNativeLayoutAndConstructorHaveNoRuntimeProbe(t *testing.T) {
	var f filter
	if unsafe.Sizeof(value{}) != 16 || unsafe.Sizeof(condition{}) != 40 || unsafe.Sizeof(filter{}) != 200 || unsafe.Sizeof(provider{}) != 64 || unsafe.Sizeof(sublayer{}) != 72 || unsafe.Offsetof(f.action) != 128 || unsafe.Offsetof(f.context) != 152 || unsafe.Offsetof(f.effectiveWeight) != 184 {
		t.Fatal("native ABI mismatch")
	}
	g, err := NewGate()
	if err != nil || g == nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Cancellation is rejected by the Go bridge before its production opener.
	if r, err := g.Inspect(ctx); err == nil || r.Verified {
		t.Fatal("cancelled constructor reached native backend", r, err)
	}
}
