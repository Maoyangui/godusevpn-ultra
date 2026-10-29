//go:build windows

package timeprivacy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteTimeJournalRejectsLeafIOAndKeepsCurrentFact(t *testing.T) {
	for _, dir := range []string{`\\example.invalid\fixture\privacy-time`, `\\?\C:\fixture\privacy-time`, `\\.\pipe\fixture\privacy-time`} {
		t.Run(dir, func(t *testing.T) {
			m, f := fakeManager(t)
			m.dir = dir
			opens, persists := 0, 0
			if _, err := m.loadUsing(func(string) (*os.File, error) { opens++; return nil, errors.New("leaf I/O must not be called") }); err == nil || opens != 0 {
				t.Fatal("remote journal reached the leaf filesystem stub")
			}
			m.persist = func(string, []byte) error { persists++; return errors.New("must not persist") }
			r, err := m.Report()
			if err != nil || r.Status != "backup_error" || r.Managed || r.CurrentID != "UTC" || !r.DaylightSavingDisabled {
				t.Fatal("current fact lost or inaccessible backup reported safe", r, err)
			}
			if _, err := m.Apply("China Standard Time"); err == nil {
				t.Fatal("remote store allowed a time-zone write")
			}
			if _, err := m.Restore(); err == nil || f.sets != 0 || persists != 0 {
				t.Fatal("remote store allowed restore/persistence")
			}
			if err := durableWrite(filepath.Join(dir, journalName), []byte("fixture")); err == nil {
				t.Fatal("direct persistence accepted remote namespace")
			}
		})
	}
}

func TestMissingLocalTimeStoreReportDoesNotCreateDirectory(t *testing.T) {
	m, _ := fakeManager(t)
	m.dir = filepath.Join(m.dir, "not-created", "privacy-time")
	r, err := m.Report()
	if err != nil || r.Status != "available" || r.CurrentID != "UTC" {
		t.Fatal("missing local store lost current facts", r, err)
	}
	if _, err := os.Stat(filepath.Dir(m.dir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only time report created state")
	}
}

func TestLocalTimeStoreProductionLockPersistenceAndRestoreRoundtrip(t *testing.T) {
	m, f := fakeManager(t)
	m.dir = filepath.Join(m.dir, "new-user-config", "privacy-time")
	before := f.state
	if _, err := m.Apply("China Standard Time"); err != nil {
		t.Fatal("production time lock/ACL/atomic persistence failed", err)
	}
	for _, name := range []string{journalName, "timezone.lock"} {
		if info, err := os.Stat(filepath.Join(m.dir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatal("production time store missing ordinary state", err)
		}
	}
	if r, err := m.Report(); err != nil || r.Status != "configured" || !r.Managed {
		t.Fatal("read lease rejected the saved production time journal", r, err)
	}
	if result, err := m.Restore(); err != nil || result.Status != "restored" || f.state != before {
		t.Fatal("production time restore/persistence failed", result, err)
	}
}
