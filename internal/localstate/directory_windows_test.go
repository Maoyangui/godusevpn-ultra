//go:build windows

package localstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The filesystem and drive mapping are entirely synthetic. No test opens a
// network path, queries a remote provider, changes ACLs or creates an account.
type fakeDirectories struct {
	driveErr     error
	driveCalls   int
	dirs         map[string]bool
	openErrors   map[string]error
	afterCreate  map[string]error
	held         map[string]bool
	opened       []string
	created      []string
	closed       int
	createUnsafe bool
}

func fixture() *fakeDirectories {
	return &fakeDirectories{dirs: map[string]bool{`C:\`: true, `C:\Local`: true}, openErrors: map[string]error{}, held: map[string]bool{}}
}
func (f *fakeDirectories) localDrive(string) error { f.driveCalls++; return f.driveErr }
func (f *fakeDirectories) openDirectory(path string, _ bool) (func(), error) {
	f.opened = append(f.opened, path)
	if err := f.openErrors[path]; err != nil {
		return nil, err
	}
	if !f.dirs[path] {
		return nil, os.ErrNotExist
	}
	f.held[path] = true
	return func() { f.held[path] = false; f.closed++ }, nil
}
func (f *fakeDirectories) createDirectory(path string) error {
	if !f.held[filepath.Dir(path)] {
		f.createUnsafe = true
	}
	f.created = append(f.created, path)
	f.dirs[path] = true
	if err := f.afterCreate[path]; err != nil {
		f.openErrors[path] = err
	}
	return nil
}

func TestUnsupportedNamespacesNeverReachFilesystem(t *testing.T) {
	for _, dir := range []string{`\\example.invalid\fixture\private`, `\\?\C:\Local\private`, `\\.\C:\Local\private`, `\??\C:\Local\private`, `relative\private`, `C:relative`, `C:\`, `C:\Local:stream\private`, `C:\Local\..\private`, `C:\Local\CON`, `C:\Local\COM¹.txt`, `C:\Local\name.`, `C:\Local\name `} {
		t.Run(dir, func(t *testing.T) {
			for _, create := range []bool{false, true} {
				f := fixture()
				if _, err := acquireDirectory(dir, create, f); err == nil {
					t.Fatal("unsupported namespace accepted")
				}
				if f.driveCalls != 0 || len(f.opened) != 0 || len(f.created) != 0 {
					t.Fatal("rejected path reached a drive or filesystem operation")
				}
			}
		})
	}
}

func TestMappedRemoteOrUnknownDriveNeverOpensFiles(t *testing.T) {
	for _, reason := range []string{"network mapping", "unknown drive", "SUBST alias", "unavailable volume"} {
		for _, create := range []bool{false, true} {
			f := fixture()
			f.driveErr = errors.New(reason)
			if _, err := acquireDirectory(`C:\Local\private`, create, f); err == nil {
				t.Fatal("unverified drive accepted")
			}
			if f.driveCalls != 1 || len(f.opened) != 0 || len(f.created) != 0 {
				t.Fatal("remote/unverified drive reached a filesystem operation")
			}
		}
	}
}

func TestDeviceMappingClassifierIsConservative(t *testing.T) {
	for _, tc := range []struct {
		mapping string
		ok      bool
	}{{`\Device\HarddiskVolume3`, true}, {`\device\harddiskvolume120`, true}, {`\Device\Mup\fixture`, false}, {`\Device\LanmanRedirector\fixture`, false}, {`\??\C:\Local`, false}, {`\??\UNC\example.invalid\fixture`, false}, {`\Device\HarddiskVolume3\nested`, false}, {`\Device\HarddiskVolume`, false}, {"", false}} {
		if localDeviceMapping(tc.mapping) != tc.ok {
			t.Fatal("device mapping classification mismatch")
		}
	}
}

func TestReparseStopsBeforeDescendantOrCreation(t *testing.T) {
	for _, create := range []bool{false, true} {
		f := fixture()
		f.openErrors[`C:\Local`] = errors.New("synthetic reparse point")
		if _, err := acquireDirectory(`C:\Local\redirected\private`, create, f); err == nil {
			t.Fatal("reparse path accepted")
		}
		if len(f.opened) != 2 || len(f.created) != 0 || f.closed != 1 {
			t.Fatal("descendant touched or prior lease leaked")
		}
	}
}

func TestReadOnlyMissingDirectoryDoesNotCreateOrProbeDescendants(t *testing.T) {
	f := fixture()
	if _, err := acquireDirectory(`C:\Local\absent\private`, false, f); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing read-only store was not reported missing", err)
	}
	if len(f.created) != 0 || len(f.opened) != 3 || f.closed != 2 {
		t.Fatal("read-only check created/probed children or leaked leases")
	}
}

func TestCreationRequiresPinnedParentsAndReopensEachNewChild(t *testing.T) {
	f := fixture()
	release, err := acquireDirectory(`C:\Local\new\private`, true, f)
	if err != nil || f.createUnsafe || len(f.created) != 2 || len(f.opened) != 6 || f.closed != 0 {
		t.Fatal("unsafe or unverified creation sequence", err)
	}
	for _, path := range []string{`C:\`, `C:\Local`, `C:\Local\new`, `C:\Local\new\private`} {
		if !f.held[path] {
			t.Fatal("validated parent not held")
		}
	}
	release()
	release()
	if f.closed != 4 {
		t.Fatal("release is incomplete or non-idempotent")
	}
}

func TestLeafHelpersRejectRemoteNamespaceBeforeOpening(t *testing.T) {
	for _, path := range []string{`\\example.invalid\fixture\journal.json`, `\\?\UNC\example.invalid\fixture\journal.json`, `\\.\pipe\fixture`} {
		for _, create := range []bool{false, true} {
			if f, err := OpenRegular(path, create); err == nil || f != nil {
				t.Fatal("remote leaf passed the pure namespace guard")
			}
		}
	}
}

func TestCreationMustNotContinueAfterNewChildVerificationFailure(t *testing.T) {
	f := fixture()
	f.afterCreate = map[string]error{`C:\Local\new`: errors.New("synthetic untrusted directory")}
	if _, err := acquireDirectory(`C:\Local\new\private`, true, f); err == nil {
		t.Fatal("untrusted component accepted")
	}
	for _, path := range append(f.opened, f.created...) {
		if strings.HasSuffix(path, `\private`) {
			t.Fatal("descendant accessed after parent verification failure")
		}
	}
	if f.closed != 2 {
		t.Fatal("parent leases leaked")
	}
}

func openDirectoryWriter(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}

// Local TEMP directories only. The writer access needed to set a junction is
// denied while pinned; no reparse target, network or filesystem ACL is changed.
func TestPinnedLocalDirectoryDeniesWriterAndReleasesIt(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireDirectory(dir, false)
	if err != nil {
		t.Fatal("local TEMP pin failed", err)
	}
	h, err := openDirectoryWriter(dir)
	if err == nil {
		windows.CloseHandle(h)
		release()
		t.Fatal("pinned directory accepted the write handle needed for reparse mutation")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		release()
		t.Fatal("writer was not refused by the sharing lease", err)
	}
	release()
	h, err = openDirectoryWriter(dir)
	if err != nil {
		t.Fatal("released directory remains locked", err)
	}
	windows.CloseHandle(h)
}

func TestPreheldDirectoryWriterMakesAcquireFailClosed(t *testing.T) {
	dir := t.TempDir()
	h, err := openDirectoryWriter(dir)
	if err != nil {
		t.Fatal("could not obtain local synthetic writer", err)
	}
	defer windows.CloseHandle(h)
	if release, err := AcquireDirectory(dir, false); err == nil || release != nil {
		if release != nil {
			release()
		}
		t.Fatal("preheld writer did not reject the pin")
	}
}

func TestWriteLeaseActuallyPinsDirectoryDeletion(t *testing.T) {
	dir := t.TempDir()
	release, err := AcquireDirectory(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	writer, err := openDirectoryWriter(dir)
	if err != nil {
		t.Fatal("write-compatible lease blocked expected parent writes", err)
	}
	windows.CloseHandle(writer)
	p, _ := windows.UTF16PtrFromString(dir)
	h, err := windows.CreateFile(p, windows.DELETE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err == nil {
		windows.CloseHandle(h)
		t.Fatal("write-compatible lease did not actually prevent directory deletion/replacement")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("directory deletion was not refused by the sharing lease", err)
	}
}

func TestNestedPinsPermitChildCreationAndRegularFileReplacement(t *testing.T) {
	parent := t.TempDir()
	outer, err := AcquireDirectory(parent, false)
	if err != nil {
		t.Fatal(err)
	}
	defer outer()
	dir := filepath.Join(parent, "new", "private")
	inner, err := AcquireDirectory(dir, true)
	if err != nil {
		t.Fatal("nested pin could not create local children", err)
	}
	defer inner()
	before, after := filepath.Join(dir, "journal.tmp"), filepath.Join(dir, "journal.json")
	if err := os.WriteFile(after, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(before, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(before, after); err != nil {
		t.Fatal("directory pin blocked ordinary file replacement", err)
	}
	if b, err := os.ReadFile(after); err != nil || string(b) != "new" {
		t.Fatal("replacement content unavailable", err)
	}
}
