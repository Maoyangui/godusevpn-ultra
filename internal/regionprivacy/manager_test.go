package regionprivacy

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const fakeSID = "S-1-5-21-123-456-789-1001"

type fakeBackend struct {
	value                       string
	list                        []Choice
	identityCalls, currentCalls int
	sets                        []string
	identityHook                func(int) (userIdentity, error)
	currentHook                 func(int) (string, error)
	setHook                     func(string) error
}

func (b *fakeBackend) identity() (userIdentity, error) {
	b.identityCalls++
	if b.identityHook != nil {
		return b.identityHook(b.identityCalls)
	}
	return userIdentity{sid: fakeSID, directory: "FAKE-NOT-A-SYSTEM-PATH"}, nil
}
func (b *fakeBackend) choices() ([]Choice, error) { return append([]Choice(nil), b.list...), nil }
func (b *fakeBackend) current() (string, error) {
	b.currentCalls++
	if b.currentHook != nil {
		return b.currentHook(b.currentCalls)
	}
	return b.value, nil
}
func (b *fakeBackend) set(code string) error {
	b.sets = append(b.sets, code)
	if b.setHook != nil {
		return b.setHook(code)
	}
	b.value = code
	return nil
}

type memoryStore struct {
	data           []byte
	writes, closes int
	hook           func(int, []byte) error
}

func (s *memoryStore) read() ([]byte, error) {
	if s.data == nil {
		return nil, os.ErrNotExist
	}
	return bytes.Clone(s.data), nil
}
func (s *memoryStore) save(data []byte) error {
	s.writes++
	if s.hook != nil {
		if err := s.hook(s.writes, bytes.Clone(data)); err != nil {
			return err
		}
	}
	s.data = bytes.Clone(data)
	return nil
}
func (s *memoryStore) close() { s.closes++ }
func fixture() (*Manager, *fakeBackend, *memoryStore) {
	b := &fakeBackend{value: "US", list: []Choice{{"US", "United States"}, {"DE", "Germany"}, {"JP", "Japan"}, {"001", "World"}}}
	s := &memoryStore{}
	m := &Manager{backend: b, open: func(id userIdentity, mode storeMode) (store, error) { return s, nil }}
	return m, b, s
}
func journalOf(t *testing.T, s *memoryStore) *transaction {
	t.Helper()
	j, err := load(s, fakeSID)
	if err != nil || j == nil {
		t.Fatal(j, err)
	}
	return j
}

func TestApplyPersistsPendingBeforeSetterAndFreshReadback(t *testing.T) {
	m, b, s := fixture()
	b.setHook = func(code string) error {
		j := journalOf(t, s)
		if j.Stage != "applying" || j.Before != "US" || j.Expected != "US" || j.Pending != code || s.writes != 1 {
			t.Fatal("setter reached before durable exact pending", j)
		}
		b.value = code
		return nil
	}
	r, err := m.Apply("DE")
	if err != nil || r.Status != "applied" || !r.Changed || !r.Verified || r.CurrentCode != "DE" || len(b.sets) != 1 || b.currentCalls != 4 || s.closes != 1 {
		t.Fatal(r, err, b, s)
	}
	j := journalOf(t, s)
	if j.Before != "US" || j.Expected != "DE" || j.Pending != "" || j.Stage != "active" {
		t.Fatal(j)
	}
	report, err := m.Report()
	if err != nil || report.Status != "configured" || !report.Managed || !report.CanRestore || report.OriginalCode != "US" || report.AtomicExternalCAS || report.ObservedAt == "" {
		t.Fatal(report, err)
	}
}
func TestRepeatedApplyKeepsFirstOriginalAndRestoreDoesNotRecapture(t *testing.T) {
	m, b, s := fixture()
	for _, code := range []string{"DE", "JP"} {
		if _, err := m.Apply(code); err != nil {
			t.Fatal(err)
		}
	}
	if j := journalOf(t, s); j.Before != "US" || j.Expected != "JP" {
		t.Fatal(j)
	}
	r, err := m.Restore()
	if err != nil || !r.Verified || b.value != "US" {
		t.Fatal(r, err)
	}
	beforeWrites, beforeSets := s.writes, len(b.sets)
	r, err = m.Restore()
	if err != nil || r.Changed || !r.Verified || s.writes != beforeWrites || len(b.sets) != beforeSets {
		t.Fatal(r, err)
	}
	// A new explicit series after a verified restore gets a new starting value.
	if _, err = m.Apply("DE"); err != nil {
		t.Fatal(err)
	}
	if j := journalOf(t, s); j.Before != "US" || j.Expected != "DE" {
		t.Fatal(j)
	}
}
func TestNoChangeDoesNotClaimOrWriteExistingSetting(t *testing.T) {
	m, b, s := fixture()
	r, err := m.Apply("US")
	if err != nil || r.Changed || !r.Verified || s.writes != 0 || s.data != nil || len(b.sets) != 0 {
		t.Fatal(r, err)
	}
}
func TestApplyInputMustBeExactSystemCatalogEntry(t *testing.T) {
	for _, code := range []string{"", "us", " USA", "USA", "../../US", "US\x00", "US\n", "001\n", "1234", "ZZ", "00"} {
		t.Run(strings.ReplaceAll(code, "\x00", "NUL"), func(t *testing.T) {
			m, b, s := fixture()
			opened := 0
			m.open = func(userIdentity, storeMode) (store, error) { opened++; return s, nil }
			if _, err := m.Apply(code); !errors.Is(err, ErrInput) || len(b.sets) != 0 || opened != 0 {
				t.Fatal("invalid/unknown input reached write boundary", err, opened)
			}
		})
	}
	m, b, s := fixture()
	b.value = "ZZ"
	if _, err := m.Apply("DE"); !errors.Is(err, ErrReadback) || len(b.sets) != 0 || s.writes != 0 {
		t.Fatal(err)
	}
}
func TestNumericOriginalIsPreservedAndRestored(t *testing.T) {
	m, b, s := fixture()
	b.value = "001"
	if _, err := m.Apply("DE"); err != nil {
		t.Fatal(err)
	}
	if j := journalOf(t, s); j.Before != "001" {
		t.Fatal(j)
	}
	if _, err := m.Restore(); err != nil || b.value != "001" {
		t.Fatal(err, b.value)
	}
}
func TestExternalChangeNeverBlindlyOverwritten(t *testing.T) {
	for _, operation := range []string{"apply", "restore"} {
		t.Run(operation, func(t *testing.T) {
			m, b, s := fixture()
			if _, err := m.Apply("DE"); err != nil {
				t.Fatal(err)
			}
			old := bytes.Clone(s.data)
			b.value = "JP"
			calls := len(b.sets)
			var err error
			if operation == "apply" {
				_, err = m.Apply("US")
			} else {
				_, err = m.Restore()
			}
			if !errors.Is(err, ErrConflict) || len(b.sets) != calls || b.value != "JP" || !bytes.Equal(old, s.data) {
				t.Fatal("external state or recovery record overwritten", err)
			}
			r, _ := m.Report()
			if r.Status != "external_change" || r.CanApply || r.CanRestore {
				t.Fatal(r)
			}
		})
	}
}
func TestExternalChangeAfterPendingStopsBeforeSetter(t *testing.T) {
	m, b, s := fixture()
	s.hook = func(n int, _ []byte) error {
		if n == 1 {
			b.value = "JP"
		}
		return nil
	}
	r, err := m.Apply("DE")
	if !errors.Is(err, ErrConflict) || r.Verified || len(b.sets) != 0 || b.value != "JP" || journalOf(t, s).Stage != "applying" {
		t.Fatal(r, err)
	}
	if _, err = m.Restore(); !errors.Is(err, ErrConflict) || len(b.sets) != 0 {
		t.Fatal(err)
	}
}
func TestIdentityChangeAfterPendingNeverCallsSetter(t *testing.T) {
	m, b, s := fixture()
	b.identityHook = func(n int) (userIdentity, error) {
		id := userIdentity{sid: fakeSID, directory: "FAKE"}
		if n > 1 {
			id.sid = "S-1-5-21-OTHER"
		}
		return id, nil
	}
	r, err := m.Apply("DE")
	if !errors.Is(err, ErrUnsupported) || r.Verified || len(b.sets) != 0 || journalOf(t, s).Stage != "applying" {
		t.Fatal(r, err)
	}
}
func TestWriteFailureBeforeSetHasNoSystemMutation(t *testing.T) {
	m, b, s := fixture()
	s.hook = func(int, []byte) error { return ErrStore }
	if _, err := m.Apply("DE"); !errors.Is(err, ErrStore) || len(b.sets) != 0 || b.value != "US" || s.data != nil {
		t.Fatal(err)
	}
}

func TestPendingSaveMayPersistThenFailAndStillNeverSets(t *testing.T) {
	m, b, s := fixture()
	s.hook = func(_ int, data []byte) error { s.data = bytes.Clone(data); return ErrStore }
	if _, err := m.Apply("DE"); !errors.Is(err, ErrStore) || len(b.sets) != 0 || b.value != "US" {
		t.Fatal(err)
	}
	if j := journalOf(t, s); j.Stage != "applying" || j.Before != "US" {
		t.Fatal(j)
	}
	s.hook = nil
	if r, err := m.Restore(); err != nil || !r.Verified || r.Changed || len(b.sets) != 0 {
		t.Fatal(r, err)
	}
}
func TestFinalWriteFailureRetainsRecoverablePending(t *testing.T) {
	m, b, s := fixture()
	s.hook = func(n int, _ []byte) error {
		if n == 2 {
			return ErrStore
		}
		return nil
	}
	r, err := m.Apply("DE")
	if !errors.Is(err, ErrStore) || r.Verified || !r.Changed || b.value != "DE" || journalOf(t, s).Stage != "applying" {
		t.Fatal(r, err)
	}
	report, _ := m.Report()
	if report.Status != "recovery_pending" || report.CanApply || !report.CanRestore {
		t.Fatal(report)
	}
	s.hook = nil
	if r, err = m.Restore(); err != nil || !r.Verified || b.value != "US" {
		t.Fatal(r, err)
	}
}

func TestExternalChangeDuringFinalCommitIsNotReportedAsSuccess(t *testing.T) {
	m, b, s := fixture()
	s.hook = func(n int, _ []byte) error {
		if n == 2 {
			b.value = "JP"
		}
		return nil
	}
	r, err := m.Apply("DE")
	if !errors.Is(err, ErrReadback) || r.Verified || b.value != "JP" || len(b.sets) != 1 {
		t.Fatal(r, err)
	}
	report, _ := m.Report()
	if report.Status != "external_change" || report.CanRestore {
		t.Fatal(report)
	}
}
func TestPendingRecoveryAcceptsOnlyKnownPreOrTargetValue(t *testing.T) {
	for _, value := range []string{"US", "DE", "JP"} {
		t.Run(value, func(t *testing.T) {
			m, b, s := fixture()
			if err := save(s, &transaction{Owner: fakeSID, Before: "US", Expected: "US", Pending: "DE", Stage: "applying"}); err != nil {
				t.Fatal(err)
			}
			b.value = value
			if _, err := m.Apply("JP"); value == "JP" && !errors.Is(err, ErrConflict) || value != "JP" && !errors.Is(err, ErrPending) {
				t.Fatal(err)
			}
			r, err := m.Restore()
			if value == "JP" {
				if !errors.Is(err, ErrConflict) || len(b.sets) != 0 {
					t.Fatal(r, err)
				}
				return
			}
			if err != nil || !r.Verified || b.value != "US" || journalOf(t, s).Stage != "restored" {
				t.Fatal(r, err)
			}
		})
	}
}
func TestSetterFailureOrExternalAfterSetNeverRollsBack(t *testing.T) {
	for _, kind := range []string{"error-before-change", "error-after-change", "external-after-change", "fresh-read-error"} {
		t.Run(kind, func(t *testing.T) {
			m, b, s := fixture()
			b.setHook = func(code string) error {
				switch kind {
				case "error-before-change":
					return ErrReadback
				case "error-after-change":
					b.value = code
					return ErrReadback
				case "external-after-change":
					b.value = "JP"
				default:
					b.value = code
				}
				return nil
			}
			if kind == "fresh-read-error" {
				b.currentHook = func(n int) (string, error) {
					if n == 3 {
						return "", ErrReadback
					}
					return b.value, nil
				}
			}
			r, err := m.Apply("DE")
			if !errors.Is(err, ErrReadback) || r.Verified || len(b.sets) != 1 || journalOf(t, s).Stage != "applying" {
				t.Fatal(r, err, b.sets)
			}
			if kind == "external-after-change" && b.value != "JP" {
				t.Fatal("blind rollback")
			}
		})
	}
}
func TestReportMissingStoreAndRestoreNeverCreateDirectories(t *testing.T) {
	m, _, _ := fixture()
	m.open = func(_ userIdentity, mode storeMode) (store, error) {
		if mode == storeCreate {
			t.Fatal("read/restore requested creation")
		}
		return nil, os.ErrNotExist
	}
	r, err := m.Report()
	if err != nil || r.Status != "available" || !r.CanApply || r.Managed {
		t.Fatal(r, err)
	}
	result, err := m.Restore()
	if err != nil || result.Changed || result.Verified {
		t.Fatal(result, err)
	}
}
func TestReportBackupFailureDoesNotInventSuccessfulObservation(t *testing.T) {
	m, b, s := fixture()
	s.data = []byte("CORRUPT")
	r, err := m.Report()
	if err != nil || r.Status != "backup_error" || r.CurrentCode != "US" || r.ObservedAt == "" || r.CanApply || r.CanRestore {
		t.Fatal(r, err)
	}
	b.currentHook = func(int) (string, error) { return "", ErrReadback }
	r, _ = m.Report()
	if r.Status != "unknown" || r.CurrentCode != "" || r.ObservedAt != "" || r.CanApply {
		t.Fatal(r)
	}
}
func TestOperationSerializesIndependentManagers(t *testing.T) {
	m, b, s := fixture()
	entered, release := make(chan struct{}), make(chan struct{})
	b.setHook = func(code string) error { close(entered); <-release; b.value = code; return nil }
	var wg sync.WaitGroup
	wg.Add(2)
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		defer wg.Done()
		defer close(firstDone)
		if _, err := m.Apply("DE"); err != nil {
			t.Error(err)
		}
	}()
	<-entered
	m2 := &Manager{backend: b, open: m.open}
	go func() {
		defer wg.Done()
		defer close(secondDone)
		if _, err := m2.Report(); err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-secondDone:
		t.Fatal("independent manager bypassed product lock")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	<-firstDone
	if journalOf(t, s).Expected != "DE" {
		t.Fatal("lost serialized result")
	}
}

func TestCheckedChoicesRejectsEmptyDuplicateUnknownAndUnbounded(t *testing.T) {
	for _, choices := range [][]Choice{nil, {{"US", ""}}, {{"us", "x"}}, {{"US", "x"}, {"US", "y"}}, {{"US", strings.Repeat("x", 513)}}, make([]Choice, 1025)} {
		if _, err := checkedChoices(choices); err == nil {
			t.Fatal("invalid catalog accepted")
		}
	}
}
