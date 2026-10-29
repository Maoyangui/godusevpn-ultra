package cli

import (
	"errors"
	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"github.com/Maoyangui/godusevpn/internal/timeprivacy"
	"strings"
	"testing"
)

func TestMachineWindowKeepsVerificationAndUninstallInsideLease(t *testing.T) {
	for _, fail := range []string{"", "initial_verify", "cleanup", "final_verify", "lost_lease", "uninstall"} {
		t.Run(fail, func(t *testing.T) {
			checks, verifies, cleans, removes := 0, 0, 0, 0
			_, err := runMachineWindow(func() error {
				checks++
				if fail == "lost_lease" && checks == 2 {
					return errors.New("lost")
				}
				return nil
			}, func() error {
				verifies++
				if fail == "initial_verify" && verifies == 1 || fail == "final_verify" && verifies == 2 {
					return errors.New("hash/identity changed")
				}
				return nil
			}, func() (any, error) {
				cleans++
				if fail == "cleanup" {
					return nil, errors.New("unknown state")
				}
				return "fixture", nil
			}, func() error {
				removes++
				if fail == "uninstall" {
					return errors.New("service kept")
				}
				return nil
			})
			if (err == nil) != (fail == "") {
				t.Fatal("unexpected result", err)
			}
			if fail == "initial_verify" && cleans != 0 {
				t.Fatal("unverified cleanup")
			}
			if fail != "" && fail != "uninstall" && removes != 0 {
				t.Fatal("uninstall followed incomplete cleanup/verification")
			}
			if fail == "" && (checks != 2 || verifies != 2 || cleans != 1 || removes != 1) {
				t.Fatal("window stages missing")
			}
		})
	}
}

func TestCleanupMachinePreservesUnknownOrManagedStateAsFailure(t *testing.T) {
	for _, kind := range []string{"unknown_filters", "managed_policy", "bad_backup", "read_error", "healthy"} {
		_, err := cleanupMachine(func(method string, p, out any) error {
			if kind == "read_error" {
				return errors.New("unreadable")
			}
			switch method {
			case "FirewallReport":
				r := out.(*appfirewall.Report)
				if kind == "unknown_filters" {
					r.UnknownFilters = 1
				}
			case "Report":
				r := out.(*privacy.Report)
				r.BackupHealthy = kind != "bad_backup"
				if kind == "managed_policy" {
					r.Controls = []privacy.ControlState{{ID: "fixture", Managed: true}}
				}
			}
			return nil
		})
		if (err == nil) != (kind == "healthy") {
			t.Fatalf("%s unexpectedly completed: %v", kind, err)
		}
	}
}

func TestUninstallManifestRejectsUnsafeOrIncompletePackages(t *testing.T) {
	required := []string{"godusevpn.exe", "godusevpn-cli.exe", "godusevpn-svc.exe", "ULTRA_PACKAGE.json", "Install-Ultra.ps1", "Uninstall-Ultra.ps1", "browser-extension/manifest.json"}
	hash := strings.Repeat("a", 64)
	hashes := map[string]string{"SHA256SUMS": hash}
	var lines []string
	for _, name := range required {
		hashes[name] = hash
		lines = append(lines, hash+"  "+name)
	}
	manifest := strings.Join(lines, "\n") + "\n"
	descriptor := []byte(`{"product":"GodUseVPN Ultra","namespace":"godusevpn-ultra"}`)
	if err := verifyUninstallManifest([]byte(manifest), descriptor, hashes); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{manifest + hash + "  ../outside\n", manifest + lines[0] + "\n", strings.Replace(manifest, hash, strings.Repeat("b", 64), 1), strings.Join(lines[1:], "\n"), "invalid"} {
		if verifyUninstallManifest([]byte(bad), descriptor, hashes) == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
	hashes["unlisted.exe"] = hash
	if verifyUninstallManifest([]byte(manifest), descriptor, hashes) == nil {
		t.Fatal("extra payload accepted")
	}
	delete(hashes, "unlisted.exe")
	delete(hashes, "SHA256SUMS")
	hashes["shadow-file"] = hash
	if verifyUninstallManifest([]byte(manifest), descriptor, hashes) == nil {
		t.Fatal("missing fixed hash manifest accepted")
	}
	delete(hashes, "shadow-file")
	hashes["SHA256SUMS"] = hash
	if verifyUninstallManifest([]byte(manifest), []byte(`{"product":"godusevpn"}`), hashes) == nil {
		t.Fatal("other product accepted")
	}
}

type fakeTimezone struct {
	reports, restores int
	err               error
}

func (f *fakeTimezone) Report() (timeprivacy.Report, error) {
	f.reports++
	return timeprivacy.Report{Status: "unknown"}, f.err
}
func (f *fakeTimezone) Restore() (timeprivacy.Result, error) {
	f.restores++
	return timeprivacy.Result{Status: "conflict", Changed: false}, f.err
}

func TestTimezoneCommandsAreIndependentAndKeepConflict(t *testing.T) {
	f := &fakeTimezone{err: errors.New("external change")}
	if _, e := timezoneOperation("timezone-report", f); e == nil || f.reports != 1 || f.restores != 0 {
		t.Fatal("report changed state")
	}
	r, e := timezoneOperation("timezone-restore", f)
	if e == nil || f.restores != 1 || r.(timezoneResult).Status != "conflict" || r.(timezoneResult).Changed {
		t.Fatal("conflict overwritten")
	}
	if _, e = timezoneOperation("timezone-apply", f); e == nil {
		t.Fatal("unexpected apply capability")
	}
}

func TestTimezoneBootstrapPreservesOriginalExecutionAndCloses(t *testing.T) {
	for _, kind := range []string{"cancelled", "connection_lost", "substitute_admin", "ready", "already_elevated", "gate_exists"} {
		boots, closes, restores := 0, 0, 0
		_, err := timezoneRestoreWithBootstrap(kind == "already_elevated", func() bool { return kind == "gate_exists" }, func() (func(), error) {
			boots++
			close := func() { closes++ }
			if kind == "cancelled" || kind == "connection_lost" {
				return close, errors.New("bootstrap unavailable")
			}
			return close, nil
		}, func() (any, error) { restores++; return "original-user-manager-only", nil })
		if kind == "cancelled" || kind == "connection_lost" {
			if err == nil || restores != 0 || closes != 1 {
				t.Fatal("failed bootstrap reached restore")
			}
		} else {
			if err != nil || restores != 1 {
				t.Fatal("original operation missing")
			}
		}
		if kind == "already_elevated" || kind == "gate_exists" {
			if boots != 0 {
				t.Fatal("unnecessary elevation")
			}
		} else if boots != 1 || closes != 1 {
			t.Fatal("bootstrap lifetime leaked")
		}
	}
}
