//go:build windows

package privacy

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsWriterTokenAndScopeMatrix(t *testing.T) {
	machine := policy(t, "windows-clipboard-history").Changes[0]
	user := policy(t, "chrome-location").Changes[0]
	for _, tc := range []struct {
		name                    string
		change                  mutation
		machine, elevated, safe bool
		ok                      bool
	}{
		{"SYSTEM fixed HKLM", machine, true, true, false, true},
		{"noninteractive elevated HKLM", machine, true, true, false, true},
		{"unprivileged HKLM", machine, true, false, true, false},
		{"SYSTEM cannot use user writer", user, false, true, false, false},
		{"SYSTEM user writer cannot use HKLM", machine, false, true, false, false},
		{"machine writer cannot use HKCU", user, true, true, true, false},
		{"ordinary interactive HKCU", user, false, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := authorizeWindowsWrite(tc.change.Location, tc.change.Desired, tc.machine, tc.elevated, tc.safe)
			if (err == nil) != tc.ok {
				t.Fatal("token/scope authorization mismatch", err)
			}
		})
	}
	foreign := machine.Location
	foreign.Name = "FixtureForeignValue"
	if err := authorizeWindowsWrite(foreign, machine.Desired, true, true, false); err == nil {
		t.Fatal("elevated machine context admitted a foreign HKLM value")
	}
}

func TestMachineDescriptorRejectsNamedAccountsAndUnprotectedDirectories(t *testing.T) {
	for _, tc := range []struct {
		name, sddl string
		ok         bool
	}{
		{"roles only", machineStoreSDDL, true},
		{"named writer", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;S-1-5-21-111-222-333-1001)", false},
		{"named reader", "O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FR;;;S-1-5-21-111-222-333-1001)", false},
		{"user owner", "O:BUG:BUD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", false},
		{"inheriting directory", "O:BAG:BAD:(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(tc.sddl)
			if err != nil {
				t.Fatal(err)
			}
			if (trustedMachineDescriptor(sd, true, true) == nil) != tc.ok {
				t.Fatal("machine descriptor trusted the wrong identity or inheritance")
			}
		})
	}
}

func machineStoreFixture(t *testing.T) string {
	t.Helper()
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("role-owned temporary ACL fixture requires an already elevated test token; no UAC is requested")
	}
	parent := filepath.Join(t.TempDir(), "private-machine-parent")
	sd, err := windows.SecurityDescriptorFromString(machineStoreSDDL)
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(parent)
	if err != nil {
		t.Fatal(err)
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(name, &sa); err != nil {
		t.Fatal("could not create the temporary role-owned fixture", err)
	}
	return filepath.Join(parent, "machine-store")
}

func productionMachineStoreWithFakeRegistry(t *testing.T) (*Manager, *fakeBackend) {
	t.Helper()
	_, f := fakeManager(t)
	f.env.SafeUser, f.env.UserKey = false, ""
	m := NewMachine(machineStoreFixture(t))
	m.backend = machineBackend{f}
	return m, f
}

func TestProductionMachineStoreRoundtripHasOnlyRoleOwnersAndACLs(t *testing.T) {
	m, f := productionMachineStoreWithFakeRegistry(t)
	id := "windows-clipboard-history"
	if result, err := m.Apply(id); err != nil || result.Status != "configured" {
		t.Fatal("fake HKLM application with the production file store failed", result, err)
	}
	assertStore := func() {
		t.Helper()
		if err := validateMachineStoreRead(m.dir); err != nil {
			t.Fatal("machine store lost role-only ownership or ACLs", err)
		}
		entries, err := os.ReadDir(m.dir)
		if err != nil || len(entries) != 2 {
			t.Fatal("unexpected temporary machine-store inventory", err)
		}
		for _, entry := range entries {
			f, err := openMachineFile(filepath.Join(m.dir, entry.Name()), windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, windows.OPEN_EXISTING)
			if err != nil {
				t.Fatal("journal/lock has non-role owner or access", err)
			}
			f.Close()
		}
	}
	assertStore()
	f.env.Edition, f.env.Build = "ServerDatacenter", 1
	if result, err := m.Restore(id); err != nil || result.Status != "restored" || len(f.values) != 0 {
		t.Fatal("SYSTEM-like fake recovery failed in the production file store", result, err)
	}
	assertStore()
}

func TestMachineTemporaryFileIsRoleOwnedAtCreation(t *testing.T) {
	dir := machineStoreFixture(t)
	if err := secureMachineDirectory(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".privacy-journal-fixture")
	f, err := openMachineFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, windows.CREATE_NEW)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil || trustedMachineDescriptor(sd, true, true) != nil {
		t.Fatal("temporary file was not atomically role-owned/private", err)
	}
}

func TestMachineStoreRejectsLegacyACLWithoutRepairOrJournalMutation(t *testing.T) {
	for _, target := range []string{"directory", journalName, "privacy.lock", ".privacy-journal-stale"} {
		t.Run(target, func(t *testing.T) {
			m, registry := productionMachineStoreWithFakeRegistry(t)
			if err := secureMachineDirectory(m.dir); err != nil {
				t.Fatal(err)
			}
			path := m.dir
			fixture := []byte("preserve-this-fixture-for-explicit-review")
			if target != "directory" {
				path = filepath.Join(m.dir, target)
				if err := os.WriteFile(path, fixture, 0600); err != nil {
					t.Fatal(err)
				}
			}
			legacy, err := windows.SecurityDescriptorFromString("O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;S-1-5-21-111-222-333-1001)")
			if err != nil {
				t.Fatal(err)
			}
			owner, _, _ := legacy.Owner()
			acl, _, _ := legacy.DACL()
			if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil); err != nil {
				t.Fatal("could not create synthetic legacy permissions", err)
			}
			readDescriptor := func() string {
				t.Helper()
				sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
				if err != nil {
					t.Fatal(err)
				}
				return sd.String()
			}
			before := readDescriptor()
			if err := validateMachineStoreRead(m.dir); err == nil {
				t.Fatal("legacy account-specific permissions were trusted")
			}
			// Unrelated operations may establish the protected helper directory
			// without accepting or rewriting a policy journal with a bad ACL.
			directoryErr := ValidateMachineStoreDirectory(m.dir)
			if (directoryErr == nil) != (target != "directory") {
				t.Fatal("policy-file trust leaked into the helper directory boundary", directoryErr)
			}
			report, err := m.Report()
			if err != nil || report.BackupHealthy {
				t.Fatal("legacy store was reported as a trusted backup", err)
			}
			for _, control := range report.Controls {
				if control.CanApply {
					t.Fatal("legacy store offered a new write")
				}
			}
			if _, err := m.Apply("windows-advertising-id"); err == nil || registry.writes != 0 {
				t.Fatal("legacy store permitted mutation", err)
			}
			if _, err := m.Restore("windows-advertising-id"); err == nil || registry.writes != 0 {
				t.Fatal("legacy store permitted restoration", err)
			}
			if after := readDescriptor(); after != before {
				t.Fatal("legacy ACL was silently repaired")
			}
			if target != "directory" {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != string(fixture) {
					t.Fatal("legacy journal/lock/temp bytes were modified", err)
				}
			}
		})
	}
}

func TestMachineStoreRejectsRemoteOrDevicePathsBeforeFilesystemAccess(t *testing.T) {
	for _, path := range []string{`\\example.invalid\fixture\machine`, `\\?\C:\fixture`, `C:\fixture:stream`, `relative\machine`} {
		if _, err := machineDirectoryPath(path, true); err == nil {
			t.Fatal("non-local or ambiguous machine path accepted")
		}
	}
}

func TestMachineStoreRejectsHardlinkedJournalBeforeRegistryMutation(t *testing.T) {
	m, registry := productionMachineStoreWithFakeRegistry(t)
	if err := secureMachineDirectory(m.dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(m.dir, journalName)
	f, err := openMachineFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, windows.CREATE_NEW)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("fixture-data"))
	f.Close()
	if err := os.Link(path, filepath.Join(filepath.Dir(m.dir), "fixture-hardlink")); err != nil {
		t.Skip("temporary filesystem does not support the hardlink fixture")
	}
	if err := validateMachineStoreRead(m.dir); err == nil {
		t.Fatal("multiply linked machine journal was trusted")
	}
	if _, err := m.Apply("windows-clipboard-history"); err == nil || registry.writes != 0 {
		t.Fatal("hardlinked journal allowed registry mutation", err)
	}
}
