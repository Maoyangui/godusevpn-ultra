//go:build windows && (amd64 || arm64)

package privacyhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These tests never launch or elevate a process and never create a real pipe.
func TestHelperArgumentsRejectArbitraryPathsAndIdentities(t *testing.T) {
	args := []string{helperFlag, "--pipe", pipePrefix + strings.Repeat("a", 64), "--session", strings.Repeat("b", 64), "--parent", "1234", "--sid", "S-1-5-21-1-2-3-1001"}
	if _, err := parseConfig(args); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		i     int
		value string
	}{{2, `\\server\pipe\test`}, {2, pipePrefix + "..\\x"}, {4, "x"}, {6, "0"}, {8, "S-1-5-18"}, {7, "--directory"}} {
		testArgs := append([]string(nil), args...)
		testArgs[change.i] = change.value
		if _, err := parseConfig(testArgs); err == nil {
			t.Fatalf("accepted %v", testArgs)
		}
	}
	if handled, code := Handle([]string{helperFlag}); !handled || code == 0 {
		t.Fatal("malformed helper mode was not rejected")
	}
	if handled, _ := Handle([]string{"--ordinary-ui"}); handled {
		t.Fatal("intercepted normal UI")
	}
}

func TestStartRejectsForeignExecutableBeforeAnyElevation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "foreign.exe")
	if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := Start(context.Background(), p); err == nil || c != nil {
		t.Fatal("accepted arbitrary helper executable")
	}
}

func TestGUIAndCLISiblingIdentityIsFixedAndBidirectional(t *testing.T) {
	dir := t.TempDir()
	gui := filepath.Join(dir, "godusevpn.exe")
	cli := filepath.Join(dir, "godusevpn-cli.exe")
	other := filepath.Join(dir, "other.exe")
	foreignGUI := filepath.Join(t.TempDir(), "godusevpn.exe")
	for _, p := range []string{gui, cli, other, foreignGUI} {
		if err := os.WriteFile(p, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		a, b string
		ok   bool
	}{{gui, gui, true}, {cli, gui, true}, {gui, cli, true}, {other, gui, false}, {cli, foreignGUI, false}, {gui, "godusevpn-cli.exe", false}} {
		if got := sameProductExecutable(tc.a, tc.b); got != tc.ok {
			t.Errorf("pair trusted=%v expected=%v: %s / %s", got, tc.ok, tc.a, tc.b)
		}
	}
}

func TestShellExecuteABI(t *testing.T) {
	if unsafe.Sizeof(shellExecuteInfo{}) != 112 || unsafe.Offsetof(shellExecuteInfo{}.Process) != 104 {
		t.Fatal("invalid SHELLEXECUTEINFOW layout")
	}
}

func TestMaintenanceHelperModeIsFixedAndCarriesNoStoragePath(t *testing.T) {
	base := []string{helperFlag, "--pipe", pipePrefix + strings.Repeat("a", 64), "--session", strings.Repeat("b", 64), "--parent", "1234", "--sid", "S-1-5-21-1-2-3-1001"}
	args := append(append([]string{}, base...), "--mode", "maintenance")
	c, e := parseConfig(args)
	if e != nil || !c.maintenanceOnly {
		t.Fatal(c, e)
	}
	for _, tail := range [][]string{{"--mode", "all"}, {"--directory", `C:\arbitrary`}, {"--mode", "maintenance", "--path", `C:\arbitrary`}, {"--sid", "S-1-5-21-1-2-3-1002"}} {
		if _, e := parseConfig(append(append([]string{}, base...), tail...)); e == nil {
			t.Fatal("accepted arbitrary bootstrap", tail)
		}
	}
}

func TestPrivateStoreDescriptorRejectsUserOwnerOrWritePermissions(t *testing.T) {
	for _, tc := range []struct {
		sddl string
		ok   bool
	}{
		{privateSDDL, true}, {"O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;BU)", true},
		{"O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FW;;;BU)", false},
		{"O:BAG:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;SD;;;BU)", false},
		{"O:BUG:BUD:P(A;;FA;;;SY)(A;;FA;;;BA)", false},
	} {
		sd, err := windows.SecurityDescriptorFromString(tc.sddl)
		if err != nil {
			t.Fatal(err)
		}
		if got := trustedDescriptor(sd) == nil; got != tc.ok {
			t.Errorf("descriptor trusted=%v expected=%v: %s", got, tc.ok, tc.sddl)
		}
	}
}
