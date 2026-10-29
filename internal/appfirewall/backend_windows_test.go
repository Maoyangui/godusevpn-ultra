//go:build windows && (amd64 || arm64)

package appfirewall

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"unsafe"
)

// ABI and policy tests only. They do not open WFP or modify any live rule.
func TestNativeABILayout(t *testing.T) {
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"blob", unsafe.Sizeof(blob{}), 16}, {"filter", unsafe.Sizeof(filter{}), 200}, {"value", unsafe.Sizeof(value{}), 16},
		{"condition", unsafe.Sizeof(condition{}), 40}, {"provider", unsafe.Sizeof(provider{}), 64}, {"sublayer", unsafe.Sizeof(sublayer{}), 72},
		{"filter.provider", unsafe.Offsetof(filter{}.Provider), 40}, {"filter.layer", unsafe.Offsetof(filter{}.Layer), 64},
		{"filter.conditions", unsafe.Offsetof(filter{}.Conditions), 120}, {"filter.action", unsafe.Offsetof(filter{}.Action), 128}, {"filter.ID", unsafe.Offsetof(filter{}.ID), 176},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestIndependentPersistentIdentityAndDeterministicKeys(t *testing.T) {
	if providerKey == sublayerKey {
		t.Fatal("provider and sublayer collide")
	}
	for _, guard := range []string{"6f6d9e2c-3a41-4b8e-9d55-676f64757365", "6f6d9e2d-3a41-4b8e-9d55-676f64757365", "6f6d9e2e-3a41-4b8e-9d55-676f64757365", "6f6d9e2f-3a41-4b8e-9d55-676f64757365"} {
		g := mustGUID(guard)
		if g == providerKey || g == sublayerKey {
			t.Fatal("shares original guard ownership")
		}
	}
	a := keyFor("app-a", "out4:10.0.0.0/8")
	if a != keyFor("app-a", "out4:10.0.0.0/8") || a == keyFor("app-b", "out4:10.0.0.0/8") || a == keyFor("app-a", "in4:10.0.0.0/8") {
		t.Fatal("unstable rule identity")
	}
	if flagsCondition != mustGUID("632ce23b-5167-435c-86d7-e903684aa80c") {
		t.Fatal("wrong FWPM_CONDITION_FLAGS")
	}
}

func TestIncomingDiscoveryMatchesDestinationInsteadOfSender(t *testing.T) {
	r := Rule{Mode: "block_discovery"}
	if addressKeyFor(r, filterSpec{Layer: "in4"}) != localAddressCondition || addressKeyFor(r, filterSpec{Layer: "in6"}) != localAddressCondition {
		t.Fatal("inbound discovery must use destination")
	}
	if addressKeyFor(r, filterSpec{Layer: "out4"}) != addressCondition || addressKeyFor(Rule{Mode: "block_lan"}, filterSpec{Layer: "in4"}) != addressCondition {
		t.Fatal("wrong remote address restriction")
	}
}

func TestTargetValidationDoesNotTrustSystemRootEnvironment(t *testing.T) {
	win := systemDirectory()
	if win == "" {
		t.Skip("cannot query Windows directory")
	}
	t.Setenv("SystemRoot", `C:\fake-root`)
	if err := validateRule(Rule{ID: "x", Program: filepath.Join(win, "System32", "svchost.exe"), Mode: "block_all"}, false); err == nil {
		t.Fatal("system service target accepted")
	}
	for _, p := range []string{`C:\test.exe:alternate.exe`, `\\?\C:\test.exe`, `\\.\C:\test.exe`} {
		if err := validateRule(Rule{ID: "x", Program: p, Mode: "block_all"}, false); err == nil {
			t.Errorf("accepted device/stream target %q", p)
		}
	}
	if self, err := os.Executable(); err == nil {
		if err = validateRule(Rule{ID: "x", Program: self, Mode: "block_all"}, false); err == nil {
			t.Fatal("current executable accepted")
		}
	}
}

func TestInstalledEvidenceRequiresExactConditions(t *testing.T) {
	r := Rule{ID: "fixture", Program: `C:\apps\selected.exe`, Mode: "block_lan"}
	s := filterSpec{Layer: "out4", Prefix: "10.0.0.0/8", Suffix: "out4:10.0.0.0/8"}
	for _, mutation := range []string{"none", "provider", "sublayer", "appid", "prefix", "mask", "loopback", "disabled", "permit", "layer", "missing", "duplicate"} {
		t.Run(mutation, func(t *testing.T) {
			app := []byte{1, 2, 3, 4}
			appBlob := blob{Size: 4, Data: &app[0]}
			addr := v4Mask{Address: binary.BigEndian.Uint32([]byte{10, 0, 0, 0}), Mask: 0xff000000}
			cs := []condition{{Key: appCondition, Value: value{Type: 12, Value: stablePointer(uintptr(unsafe.Pointer(&appBlob)))}}, {Key: flagsCondition, Match: 8, Value: value{Type: 3, Value: 1}}, {Key: addressCondition, Value: value{Type: 0x100, Value: stablePointer(uintptr(unsafe.Pointer(&addr)))}}}
			f := filter{Key: keyFor(r.ID, s.Suffix), Provider: &providerKey, Sublayer: sublayerKey, Layer: layers[s.Layer], Flags: 1, Action: action{Type: 0x1001}, Count: 3, Conditions: &cs[0]}
			expectedIdentity := []byte{1, 2, 3, 4}
			switch mutation {
			case "provider":
				other := mustGUID("00000000-0000-0000-0000-000000000001")
				f.Provider = &other
			case "sublayer":
				f.Sublayer = providerKey
			case "appid":
				expectedIdentity[0] = 9
			case "prefix":
				addr.Address = 0
			case "mask":
				addr.Mask = 0
			case "loopback":
				cs[1].Match = 6
			case "disabled":
				f.Flags |= 0x20
			case "permit":
				f.Action.Type = 0x1002
			case "layer":
				f.Layer = layers["in4"]
			case "missing":
				f.Count = 2
			case "duplicate":
				cs[1] = cs[0]
			}
			if got := matchesFilterWithIdentity(&f, r, s, expectedIdentity); got != (mutation == "none") {
				t.Fatalf("evidence accepted=%v mutation=%s", got, mutation)
			}
		})
	}
}

func TestNativeUnionPointersEscapeMovableStack(t *testing.T) {
	cmd := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-gcflags=-m", ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("escape analysis failed: %v\n%s", err, output)
	}
	for _, name := range []string{"appBlob", "m4", "m6", "entries", "n", "tpl", "filterID"} {
		pattern := `backend_windows\.go:\d+:\d+: moved to heap: ` + name + `\b`
		if !regexp.MustCompile(pattern).Match(output) {
			t.Errorf("native pointer remains on movable stack: %s\n%s", name, output)
		}
	}
}
