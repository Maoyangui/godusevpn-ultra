package timeprivacy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// All mutation tests use this fake. No test calls tzutil or changes Windows.
type fakeBackend struct {
	on                  bool
	state               zoneState
	items               []Choice
	sets                int
	reads               int
	readFailures        map[int]bool
	setFailure          bool
	changeBeforeFailure bool
	beforeSet           func(zoneState)
}

func (f *fakeBackend) supported() bool            { return f.on }
func (f *fakeBackend) choices() ([]Choice, error) { return append([]Choice{}, f.items...), nil }
func (f *fakeBackend) current() (zoneState, error) {
	f.reads++
	if f.readFailures[f.reads] {
		return zoneState{}, errors.New("unknown read")
	}
	return f.state, nil
}
func (f *fakeBackend) set(s zoneState) error {
	f.sets++
	if f.beforeSet != nil {
		f.beforeSet(s)
	}
	if f.setFailure && !f.changeBeforeFailure {
		return errors.New("privilege denied")
	}
	f.state = s
	if f.setFailure {
		return errors.New("command failed after change")
	}
	return nil
}
func fakeManager(t *testing.T) (*Manager, *fakeBackend) {
	t.Helper()
	f := &fakeBackend{on: true, state: zoneState{"UTC", true}, items: []Choice{{"UTC", "UTC"}, {"China Standard Time", "China"}, {"Pacific Standard Time", "Pacific"}}, readFailures: map[int]bool{}}
	return &Manager{dir: t.TempDir(), backend: f, persist: durableWrite}, f
}

func TestApplyBacksUpBeforeChangeAndRestorePreservesDSTPreference(t *testing.T) {
	m, f := fakeManager(t)
	f.beforeSet = func(s zoneState) {
		j, err := m.load()
		if err != nil || j == nil || j.Before.ID != "UTC" {
			t.Fatal("system change without durable original", j, err)
		}
	}
	r, err := m.Apply("China Standard Time")
	if err != nil || !r.Changed || f.state.ID != "China Standard Time" || !f.state.DSTDisabled {
		t.Fatal(r, err, f.state)
	}
	report, err := m.Report()
	if err != nil || !report.Managed || report.Status != "configured" || report.CurrentID != f.state.ID {
		t.Fatal(report, err)
	}
	r, err = m.Restore()
	if err != nil || !r.Changed || f.state != (zoneState{"UTC", true}) {
		t.Fatal(r, err, f.state)
	}
	report, _ = m.Report()
	if report.Managed {
		t.Fatal("restoration did not complete")
	}
}

func TestOnlyWindowsEnumeratedIDAllowed(t *testing.T) {
	m, f := fakeManager(t)
	for _, id := range []string{"not a system ID", "UTC /g", "UTC_dstoff", "UTC\" /s", "UTC\x00", "/g"} {
		if _, err := m.Apply(id); err == nil {
			t.Fatal("unlisted or injected ID accepted", id)
		}
	}
	if f.sets != 0 {
		t.Fatal("unexpected setting")
	}
}

func TestSameTargetDoesNotClaimExternalSetting(t *testing.T) {
	m, f := fakeManager(t)
	r, err := m.Apply("UTC")
	if err != nil || r.Changed || f.sets != 0 {
		t.Fatal(r, err)
	}
	j, err := m.load()
	if err != nil || j != nil {
		t.Fatal(j, err)
	}
	r, err = m.Restore()
	if err != nil || r.Status != "unmanaged" {
		t.Fatal(r, err)
	}
}

func TestRepeatedSelectionKeepsFirstOriginal(t *testing.T) {
	m, f := fakeManager(t)
	for _, id := range []string{"China Standard Time", "Pacific Standard Time"} {
		if _, err := m.Apply(id); err != nil {
			t.Fatal(err)
		}
	}
	j, err := m.load()
	if err != nil || j.Before.ID != "UTC" || j.Desired.ID != "Pacific Standard Time" {
		t.Fatal(j, err)
	}
	if _, err = m.Restore(); err != nil || f.state.ID != "UTC" {
		t.Fatal(f.state, err)
	}
}

func TestRestorePreservesExternalIDOrDSTChange(t *testing.T) {
	for _, mode := range []string{"id", "dst"} {
		t.Run(mode, func(t *testing.T) {
			m, f := fakeManager(t)
			if _, err := m.Apply("China Standard Time"); err != nil {
				t.Fatal(err)
			}
			if mode == "id" {
				f.state.ID = "Pacific Standard Time"
			} else {
				f.state.DSTDisabled = false
			}
			external := f.state
			sets := f.sets
			r, err := m.Restore()
			if err == nil || r.Status != "conflict" || f.sets != sets || f.state != external {
				t.Fatal(r, err)
			}
			j, jerr := m.load()
			if jerr != nil || j == nil {
				t.Fatal("conflict lost backup", jerr)
			}
		})
	}
}

func TestStorageFailureStopsBeforeSystemMutation(t *testing.T) {
	m, f := fakeManager(t)
	m.persist = func(string, []byte) error { return errors.New("disk full") }
	if _, err := m.Apply("China Standard Time"); err == nil || f.sets != 0 {
		t.Fatal("mutation before durable journal", err)
	}
}

func TestPrivilegeDeniedRetainsJournalAndCanCompleteNoOpRestore(t *testing.T) {
	m, f := fakeManager(t)
	f.setFailure = true
	r, err := m.Apply("China Standard Time")
	if err == nil || r.Status != "recovery_pending" || r.Changed {
		t.Fatal(r, err)
	}
	j, jerr := m.load()
	if jerr != nil || j == nil {
		t.Fatal(j, jerr)
	}
	r, err = m.Restore()
	if err != nil || r.Changed || r.Status != "restored" {
		t.Fatal(r, err)
	}
}

func TestCommandFailureAfterActualChangeRemainsRecoverable(t *testing.T) {
	m, f := fakeManager(t)
	f.setFailure = true
	f.changeBeforeFailure = true
	r, err := m.Apply("China Standard Time")
	if err == nil || r.Status != "recovery_pending" || !r.Changed {
		t.Fatal(r, err)
	}
	f.setFailure = false
	if _, err = m.Restore(); err != nil || f.state.ID != "UTC" {
		t.Fatal(f.state, err)
	}
}

func TestUnknownPostWriteReadPreservesPreparedJournal(t *testing.T) {
	m, f := fakeManager(t)
	f.readFailures[3] = true
	r, err := m.Apply("China Standard Time")
	if err == nil || r.Status != "recovery_pending" {
		t.Fatal(r, err)
	}
	j, jerr := m.load()
	if jerr != nil || j == nil || j.Stage != "applying" {
		t.Fatal(j, jerr)
	}
	if _, err = m.Restore(); err != nil || f.state.ID != "UTC" {
		t.Fatal(f.state, err)
	}
}

func TestCompletionSaveFailureRetainsCrashRecovery(t *testing.T) {
	m, f := fakeManager(t)
	calls := 0
	m.persist = func(p string, b []byte) error {
		calls++
		if calls == 2 {
			return errors.New("disk full")
		}
		return durableWrite(p, b)
	}
	r, err := m.Apply("China Standard Time")
	if err == nil || r.Status != "recovery_pending" || f.state.ID != "China Standard Time" {
		t.Fatal(r, err)
	}
	if _, err = m.Restore(); err != nil || f.state.ID != "UTC" {
		t.Fatal(f.state, err)
	}
}

func TestCrashWhileChangingAnAlreadyManagedZoneRestoresOriginal(t *testing.T) {
	m, f := fakeManager(t)
	if _, err := m.Apply("China Standard Time"); err != nil {
		t.Fatal(err)
	}
	j, err := m.load()
	if err != nil {
		t.Fatal(err)
	}
	j.Previous = f.state
	j.Desired.ID = "Pacific Standard Time"
	j.Stage = "applying"
	if err = m.save(j); err != nil {
		t.Fatal(err)
	}
	// A crash happened after prepare, before changing China -> Pacific. A first
	// recovery is denied, but a second must still accept the known previous value.
	f.setFailure = true
	r, err := m.Restore()
	if err == nil || r.Status != "recovery_pending" {
		t.Fatal(r, err)
	}
	f.setFailure = false
	r, err = m.Restore()
	if err != nil || f.state.ID != "UTC" {
		t.Fatal(r, err, f.state)
	}
}

func TestRestoreCommandFailureCanRetry(t *testing.T) {
	m, f := fakeManager(t)
	if _, err := m.Apply("China Standard Time"); err != nil {
		t.Fatal(err)
	}
	f.setFailure = true
	if _, err := m.Restore(); err == nil {
		t.Fatal("failure not reported")
	}
	f.setFailure = false
	if _, err := m.Restore(); err != nil || f.state.ID != "UTC" {
		t.Fatal(f.state, err)
	}
}

func TestRestoreCompletionFailureDoesNotRewriteOriginalOnRetry(t *testing.T) {
	m, f := fakeManager(t)
	if _, err := m.Apply("China Standard Time"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	m.persist = func(p string, b []byte) error {
		calls++
		if calls == 2 {
			return errors.New("disk full")
		}
		return durableWrite(p, b)
	}
	r, err := m.Restore()
	if err == nil || r.Status != "recovery_pending" || f.state.ID != "UTC" {
		t.Fatal(r, err)
	}
	sets := f.sets
	r, err = m.Restore()
	if err != nil || f.sets != sets {
		t.Fatal(r, err)
	}
}

func TestCorruptionBlocksApplyRestoreButReportRemainsUseful(t *testing.T) {
	m, f := fakeManager(t)
	if err := os.WriteFile(filepath.Join(m.dir, journalName), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := m.Report()
	if err != nil || r.Status != "backup_error" || r.CurrentID != "UTC" || len(r.Choices) != 3 {
		t.Fatal(r, err)
	}
	if _, err = m.Apply("China Standard Time"); err == nil {
		t.Fatal("corrupt backup accepted")
	}
	if _, err = m.Restore(); err == nil || f.sets != 0 {
		t.Fatal("corrupt restore accepted", err)
	}
}

func TestTamperedAndUnlistedBackupIDCannotBecomeArguments(t *testing.T) {
	m, f := fakeManager(t)
	j := &transaction{Before: zoneState{"not in Windows", false}, Previous: f.state, Desired: zoneState{"China Standard Time", true}, Stage: "active"}
	if err := m.save(j); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(); err == nil || f.sets != 0 {
		t.Fatal("unlisted original accepted", err)
	}
	j.Before.ID = "UTC\" /s"
	if err := m.save(j); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(); err == nil || f.sets != 0 {
		t.Fatal("malformed original accepted", err)
	}
}

func TestUnknownCurrentDoesNotMutate(t *testing.T) {
	m, f := fakeManager(t)
	f.readFailures[1] = true
	if _, err := m.Apply("China Standard Time"); err == nil || f.sets != 0 {
		t.Fatal("unknown current accepted", err)
	}
}

func TestUnsupportedPlatformShowsManualAndNeverSets(t *testing.T) {
	m, f := fakeManager(t)
	f.on = false
	r, err := m.Report()
	if err != nil || r.Supported || r.Status != "manual" {
		t.Fatal(r, err)
	}
	if _, err = m.Apply("China Standard Time"); err == nil {
		t.Fatal("unsupported apply")
	}
	if _, err = m.Restore(); err == nil || f.sets != 0 {
		t.Fatal("unsupported restore", err)
	}
}
