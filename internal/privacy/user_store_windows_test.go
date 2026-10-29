//go:build windows

package privacy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteUserJournalNeverReachesLeafIOAndKeepsPolicyEvidence(t *testing.T) {
	for _, dir := range []string{`\\example.invalid\fixture\privacy-user`, `\\?\C:\fixture\privacy-user`, `\\.\pipe\fixture\privacy-user`} {
		t.Run(dir, func(t *testing.T) {
			m, f := fakeManager(t)
			m.dir = dir
			opens, persists := 0, 0
			m.openJournal = func(string) (*os.File, error) { opens++; return nil, errors.New("leaf I/O must not be called") }
			m.persist = func(string, []byte) error { persists++; return errors.New("persistence must not be called") }
			x := policy(t, "chrome-location").Changes[0]
			f.values[x.Location] = x.Desired
			r, err := m.Report()
			if err != nil || r.BackupHealthy || opens != 0 || len(f.reads) == 0 {
				t.Fatal("remote journal was touched or safe registry evidence lost", err)
			}
			found := false
			for _, c := range r.Controls {
				if c.ID == "chrome-location" {
					found = c.ReadStatus == "match" && !c.CanApply
				}
			}
			if !found || strings.Contains(strings.Join(r.Warnings, " "), "example.invalid") {
				t.Fatal("scope/error evidence wrong or raw path leaked")
			}
			if _, err := m.Apply("chrome-location"); err == nil {
				t.Fatal("remote store allowed policy application")
			}
			if _, err := m.Restore("chrome-location"); err == nil {
				t.Fatal("remote store allowed restoration")
			}
			if _, err := m.RestoreAll(); err == nil || opens != 0 || persists != 0 || f.writes != 0 {
				t.Fatal("remote store allowed leaf I/O or policy mutation")
			}
			if err := durableWrite(filepath.Join(dir, journalName), []byte("fixture")); err == nil {
				t.Fatal("direct persistence accepted an unsupported namespace")
			}
		})
	}
}

func TestMissingLocalUserStoreReportDoesNotCreateDirectory(t *testing.T) {
	m, _ := fakeManager(t)
	m.dir = filepath.Join(m.dir, "not-created", "privacy-user")
	r, err := m.Report()
	if err != nil || !r.BackupHealthy {
		t.Fatal("supported missing local store was not treated as empty", err)
	}
	if _, err := os.Stat(filepath.Dir(m.dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only report created local state")
	}
}

func TestLocalUserStoreProductionLockPersistenceAndRestoreRoundtrip(t *testing.T) {
	m, f := fakeManager(t)
	m.dir = filepath.Join(m.dir, "new-user-config", "privacy-user")
	x := policy(t, "chrome-location").Changes[0]
	before := value{Exists: true, Type: 3, Data: []byte("synthetic-original")}
	f.values[x.Location] = before
	if _, err := m.Apply("chrome-location"); err != nil {
		t.Fatal("production user lock/ACL/atomic persistence failed", err)
	}
	for _, name := range []string{journalName, "privacy.lock"} {
		if info, err := os.Stat(filepath.Join(m.dir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatal("production user store did not create expected ordinary state", err)
		}
	}
	if r, err := m.Report(); err != nil || !r.BackupHealthy {
		t.Fatal("read lease rejected the saved production user journal", err)
	}
	if result, err := m.Restore("chrome-location"); err != nil || result.Status != "restored" || !same(f.values[x.Location], before) {
		t.Fatal("production user restore/persistence failed", result, err)
	}
}
