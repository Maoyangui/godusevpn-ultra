package locationadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

type fakeBackend struct {
	mu              sync.Mutex
	current         Snapshot
	prepared        Snapshot
	prepares        int
	writes          int
	atomicErr       error
	prepareErr      error
	preparedRaw     []byte
	reuseRaw        bool
	applyErr        error
	readErr         error
	mismatch        bool
	applies         int
	restores        int
	restoreErr      error
	restoreMismatch bool
	applyHook       func(context.Context)
	afterApplyHook  func()
	restoreHook     func(context.Context)
	prepareHook     func(Snapshot)
	mutateCASArgs   bool
}

func (f *fakeBackend) Read(context.Context) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readErr != nil {
		return Snapshot{}, f.readErr
	}
	return f.current, nil
}

func (f *fakeBackend) CheckAtomic(context.Context) error { return f.atomicErr }

func (f *fakeBackend) PrepareApply(_ context.Context, before Snapshot, p Position) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.prepareErr != nil {
		return Snapshot{}, f.prepareErr
	}
	if f.prepareHook != nil {
		f.prepareHook(before)
	}
	f.prepares++
	raw := f.preparedRaw
	if len(raw) == 0 {
		raw = []byte(fmt.Sprintf("fake durable owner generation %d", f.prepares))
	}
	f.prepared = Snapshot{Present: true, Position: p, Raw: raw}
	return f.prepared, nil
}

func (f *fakeBackend) CompareAndSwap(ctx context.Context, expected, replacement Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	applying := snapshotEqual(replacement, f.prepared) && replacement.Present
	if applying {
		f.applies++
		if f.applyHook != nil {
			f.applyHook(ctx)
		}
	} else {
		f.restores++
		if f.restoreHook != nil {
			f.restoreHook(ctx)
		}
	}
	// Hooks represent another writer immediately before the authoritative
	// comparison. The comparison and assignment below share the same lock.
	if !snapshotEqual(f.current, expected) {
		return ErrStateConflict
	}
	if applying && f.applyErr != nil {
		return f.applyErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !applying && f.restoreErr != nil {
		return f.restoreErr
	}
	if f.reuseRaw && len(f.current.Raw) != 0 {
		f.current.Raw[0] = 'X' // A backend may reuse its own Read buffer.
	}
	f.current = cloneSnapshot(replacement)
	f.writes++
	if applying && f.mismatch {
		f.current.Position.Latitude += .1
	}
	if !applying && f.restoreMismatch {
		f.current = Snapshot{Present: true, Position: Position{Latitude: 80, Longitude: 80}}
	}
	if f.mutateCASArgs {
		if len(expected.Raw) != 0 {
			expected.Raw[0] = 'Y'
		}
		if len(replacement.Raw) != 0 {
			replacement.Raw[0] = 'Z'
		}
	}
	if applying && f.afterApplyHook != nil {
		f.afterApplyHook()
	}
	return nil
}

func componentFixture(t *testing.T) (Component, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture-location-helper.exe")
	data := []byte("verified fixture")
	if err := os.WriteFile(p, data, 0700); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	return Component{ID: "fixture", Upstream: "https://example.invalid/location-helper", Version: "fixture", License: "MIT", ArtifactPath: p, SHA256: hex.EncodeToString(h[:]), RequiresAdmin: true}, p
}
func controllerFixture(t *testing.T, f Backend) Controller {
	c, _ := componentFixture(t)
	return Controller{Component: c, Backend: f, Platform: "windows", Admin: func() bool { return true }, BackupPath: filepath.Join(t.TempDir(), "state", "location.json")}
}

func TestInspectRequiresPinnedComponent(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	c.Component.SHA256 = ""
	r, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusUnverified || f.applies != 0 {
		t.Fatalf("status=%s reason=%q", r.Status, r.Reason)
	}
}

func TestComponentJSONDoesNotExposeLocalPath(t *testing.T) {
	c, _ := componentFixture(t)
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) == "" || string(b) == "null" || strings.Contains(string(b), c.ArtifactPath) {
		t.Fatalf("component JSON exposed local path: %s", b)
	}
}
func TestApplyBacksUpReadbackAndRestore(t *testing.T) {
	f := &fakeBackend{current: Snapshot{Present: true, Position: Position{Latitude: 1, Longitude: 2, AccuracyMeter: 4}, Raw: []byte("original-provider-state")}}
	c := controllerFixture(t, f)
	target := Position{Latitude: 35.1, Longitude: 139.7, AccuracyMeter: 25}
	result, err := c.Apply(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Status != StatusApplied || !result.Changed || f.applies != 1 {
		t.Fatalf("result=%+v applies=%d", result, f.applies)
	}
	if _, err := os.Stat(c.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	restored, err := c.Restore(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if restored.Report.Status != StatusReady || f.restores != 1 || !f.current.Position.equal(Position{Latitude: 1, Longitude: 2, AccuracyMeter: 4}) || string(f.current.Raw) != "original-provider-state" {
		t.Fatalf("restore=%+v backend=%+v", restored, f.current)
	}
}
func TestApplyMismatchAttemptsRollbackAndKeepsBackup(t *testing.T) {
	f := &fakeBackend{mismatch: true, current: Snapshot{Present: false}}
	c := controllerFixture(t, f)
	_, err := c.Apply(context.Background(), Position{Latitude: 1, Longitude: 2})
	if err == nil || f.restores != 1 {
		t.Fatalf("err=%v restores=%d", err, f.restores)
	}
	if _, e := os.Stat(c.BackupPath); e != nil {
		t.Fatalf("backup missing after mismatch: %v", e)
	}
}

func TestApplyErrorAttemptsRollbackAndKeepsBackup(t *testing.T) {
	f := &fakeBackend{applyErr: os.ErrPermission, current: Snapshot{Present: true, Position: Position{Latitude: 2, Longitude: 3}}}
	c := controllerFixture(t, f)
	_, err := c.Apply(context.Background(), Position{Latitude: 1, Longitude: 2})
	if err == nil || f.restores != 1 {
		t.Fatalf("err=%v restores=%d", err, f.restores)
	}
	if _, err := os.Stat(c.BackupPath); err != nil {
		t.Fatalf("backup missing after apply error: %v", err)
	}
}
func TestApplyRejectsUnsafeInputAndNoMutation(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	_, err := c.Apply(context.Background(), Position{Latitude: 91, Longitude: 2})
	if err == nil {
		t.Fatal("invalid latitude accepted")
	}
	if f.applies != 0 {
		t.Fatal("invalid input mutated backend")
	}
	c.Admin = func() bool { return false }
	if _, err = c.Apply(context.Background(), Position{Latitude: 1, Longitude: 2}); err == nil {
		t.Fatal("unauthorized apply accepted")
	}
	if f.applies != 0 {
		t.Fatal("unauthorized input mutated backend")
	}
}
func TestInspectMissingComponentIsNonMutating(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	c.Component.ArtifactPath = filepath.Join(t.TempDir(), "missing.exe")
	r, err := c.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusMissing || f.applies != 0 {
		t.Fatalf("status=%s applies=%d", r.Status, f.applies)
	}
}

func TestRepeatedApplyPreservesFirstSnapshot(t *testing.T) {
	original := Snapshot{Present: true, Position: Position{Latitude: 1, Longitude: 2}, Raw: []byte("initial-provider-state")}
	f := &fakeBackend{current: original}
	c := controllerFixture(t, f)
	for _, p := range []Position{{Latitude: 10, Longitude: 20}, {Latitude: 30, Longitude: 40}} {
		if _, err := c.Apply(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		b, err := c.loadBackup()
		if err != nil || !snapshotEqual(b.Snapshot, original) || b.Pending != nil || !snapshotEqual(b.Expected, f.current) {
			t.Fatalf("backup=%+v err=%v", b, err)
		}
	}
	if _, err := c.Restore(context.Background()); err != nil || !snapshotEqual(f.current, original) {
		t.Fatalf("restore err=%v current=%+v", err, f.current)
	}
	restored, err := c.Restore(context.Background())
	if err != nil || restored.Changed || f.restores != 1 {
		t.Fatalf("repeat restore=%+v err=%v restores=%d", restored, err, f.restores)
	}
}

func TestFailedSecondApplyRollsBackLastStateAndKeepsInitialSnapshot(t *testing.T) {
	original := Snapshot{Raw: []byte("original configuration")}
	f := &fakeBackend{current: original}
	c := controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
		t.Fatal(err)
	}
	last := f.current
	f.applyErr = os.ErrPermission
	if _, err := c.Apply(context.Background(), Position{Latitude: 30, Longitude: 40}); err == nil {
		t.Fatal("failed apply accepted")
	}
	b, err := c.loadBackup()
	if err != nil || !snapshotEqual(f.current, last) || !snapshotEqual(b.Snapshot, original) || b.Pending != nil {
		t.Fatalf("backup=%+v current=%+v err=%v", b, f.current, err)
	}
	if _, err := c.Restore(context.Background()); err != nil || !snapshotEqual(f.current, original) {
		t.Fatalf("restore err=%v current=%+v", err, f.current)
	}
}

func TestBackupIdentityMismatchRejectsMutations(t *testing.T) {
	changes := map[string]func(*Component){
		"id":      func(c *Component) { c.ID = "other" },
		"source":  func(c *Component) { c.Upstream = "https://example.invalid/other" },
		"version": func(c *Component) { c.Version = "other" },
		"license": func(c *Component) { c.License = "Apache-2.0" },
		"hash":    func(c *Component) { c.SHA256 = strings.Repeat("0", 64) },
		"admin":   func(c *Component) { c.RequiresAdmin = !c.RequiresAdmin },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			f := &fakeBackend{}
			c := controllerFixture(t, f)
			if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
				t.Fatal(err)
			}
			b, err := c.loadBackup()
			if err != nil {
				t.Fatal(err)
			}
			change(&b.Component)
			if err := c.writeBackup(b); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Restore(context.Background()); err == nil {
				t.Fatal("restore accepted mismatched component")
			}
			if _, err := c.Apply(context.Background(), Position{Latitude: 30, Longitude: 40}); err == nil {
				t.Fatal("apply accepted mismatched component")
			}
			if f.applies != 1 || f.restores != 0 {
				t.Fatalf("applies=%d restores=%d", f.applies, f.restores)
			}
		})
	}
}

func TestExternalStateConflictDoesNotOverwriteProviderOrBackup(t *testing.T) {
	for _, rawOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "coordinates", true: "provider ownership"}[rawOnly], func(t *testing.T) {
			f := &fakeBackend{}
			c := controllerFixture(t, f)
			if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(c.BackupPath)
			if err != nil {
				t.Fatal(err)
			}
			if rawOnly {
				f.current.Raw = []byte("another owner's session")
			} else {
				f.current.Position.Longitude = 21
			}
			external := f.current
			if _, err := c.Apply(context.Background(), Position{Latitude: 30, Longitude: 40}); err == nil {
				t.Fatal("apply overwrote external state")
			}
			if _, err := c.Restore(context.Background()); err == nil {
				t.Fatal("restore overwrote external state")
			}
			inspected, err := c.Inspect(context.Background())
			if err != nil || inspected.Status != StatusRollback {
				t.Fatalf("inspect=%+v err=%v", inspected, err)
			}
			after, err := os.ReadFile(c.BackupPath)
			if err != nil || string(after) != string(before) || !snapshotEqual(f.current, external) || f.applies != 1 || f.restores != 0 {
				t.Fatalf("external state or backup changed, err=%v applies=%d restores=%d", err, f.applies, f.restores)
			}
		})
	}
}

func TestUnusableTempBackupStoragePreventsMutation(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	blocker := filepath.Join(t.TempDir(), "parent-is-a-file")
	if err := os.WriteFile(blocker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	c.BackupPath = filepath.Join(blocker, "location.json")
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err == nil {
		t.Fatal("unwritable backup accepted")
	}
	if f.applies != 0 || f.restores != 0 {
		t.Fatalf("mutated provider: %+v", f)
	}
	data, err := os.ReadFile(blocker)
	if err != nil || string(data) != "keep" {
		t.Fatalf("storage blocker changed: %q, %v", data, err)
	}
}

func TestReceiptWriteFailureRollsBackAndRetainsRecoveryJournal(t *testing.T) {
	original := Snapshot{Raw: []byte("original-provider-configuration")}
	f := &fakeBackend{current: original}
	c := controllerFixture(t, f)
	stateDir := filepath.Dir(c.BackupPath)
	retainedDir := filepath.Join(filepath.Dir(stateDir), "retained-state")
	f.applyHook = func(context.Context) {
		// Make storage unavailable after the pre-mutation journal was saved,
		// retaining the journal rather than deleting or corrupting it.
		if err := os.Rename(stateDir, retainedDir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stateDir, []byte("temporary storage failure"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20})
	if err == nil || r.Report.Status != StatusRollback || f.restores != 1 || !snapshotEqual(f.current, original) {
		t.Fatalf("result=%+v current=%+v restores=%d err=%v", r, f.current, f.restores, err)
	}
	if err := os.Remove(stateDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(retainedDir, stateDir); err != nil {
		t.Fatal(err)
	}
	b, err := c.loadBackup()
	if err != nil || b.Pending == nil || !snapshotEqual(b.Snapshot, original) {
		t.Fatalf("backup=%+v err=%v", b, err)
	}
	if _, err := c.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptOrLegacyBackupCannotBeReplacedByApply(t *testing.T) {
	for _, content := range []string{"broken JSON", `{"version":1}`, `{"version":2,"snapshot":{"present":true,"position":{"latitude":100}}}`} {
		t.Run(content, func(t *testing.T) {
			f := &fakeBackend{}
			c := controllerFixture(t, f)
			if err := os.MkdirAll(filepath.Dir(c.BackupPath), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(c.BackupPath, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err == nil {
				t.Fatal("invalid backup replaced")
			}
			if _, err := c.Restore(context.Background()); err == nil {
				t.Fatal("invalid backup restored")
			}
			data, err := os.ReadFile(c.BackupPath)
			if err != nil || string(data) != content || f.applies != 0 || f.restores != 0 {
				t.Fatalf("backup/provider changed err=%v", err)
			}
		})
	}
}

func TestPendingTransactionAllowsRestoreButRejectsFurtherApply(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	target := Snapshot{Present: true, Position: Position{Latitude: 10, Longitude: 20}, Raw: []byte("prepared owned generation")}
	b := backup{Version: backupVersion, Component: c.Component, Snapshot: f.current, Expected: f.current, Pending: &target}
	if err := c.writeBackup(b); err != nil {
		t.Fatal(err)
	}
	f.current = target // Simulates a crash after provider mutation and before receipt commit.
	if _, err := c.Apply(context.Background(), Position{Latitude: 30, Longitude: 40}); err == nil {
		t.Fatal("incomplete transaction ignored")
	}
	if _, err := c.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !snapshotEqual(f.current, b.Snapshot) || f.applies != 0 || f.restores != 1 {
		t.Fatalf("provider=%+v", f)
	}
}

func TestRollbackUsesUncancelledContextAndVerifiesReadback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeBackend{applyErr: context.Canceled, applyHook: func(context.Context) { cancel() }}
	c := controllerFixture(t, f)
	r, err := c.Apply(ctx, Position{Latitude: 10, Longitude: 20})
	if !errors.Is(err, context.Canceled) || f.restores != 1 || !strings.Contains(r.Report.Reason, "已恢复本次操作前状态并独立读回") {
		t.Fatalf("result=%+v err=%v restores=%d", r, err, f.restores)
	}
	b, err := c.loadBackup()
	if err != nil || b.Pending != nil {
		t.Fatalf("backup=%+v err=%v", b, err)
	}
}

func TestRollbackDoesNotClaimSuccessWithoutMatchingReadback(t *testing.T) {
	f := &fakeBackend{restoreMismatch: true}
	f.applyHook = func(context.Context) { f.readErr = errors.New("readback unavailable after apply") }
	c := controllerFixture(t, f)
	r, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20})
	if err == nil || r.Report.Status != StatusRollback || !strings.Contains(r.Report.Reason, "未通过独立读回") {
		t.Fatalf("result=%+v err=%v", r, err)
	}
	b, err := c.loadBackup()
	if err != nil || b.Pending == nil {
		t.Fatalf("pending receipt lost: %+v %v", b, err)
	}
	if _, err := c.Restore(context.Background()); err == nil {
		t.Fatal("unknown provider state overwritten")
	}
}

func TestRestoreRejectsChangedArtifactAndReadFailure(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
		t.Fatal(err)
	}
	f.readErr = errors.New("readback unavailable")
	if _, err := c.Restore(context.Background()); err == nil || f.restores != 0 {
		t.Fatalf("read failure err=%v restores=%d", err, f.restores)
	}
	f.readErr = nil
	if err := os.WriteFile(c.Component.ArtifactPath, []byte("replaced binary"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Restore(context.Background()); err == nil || f.restores != 0 {
		t.Fatalf("changed artifact err=%v restores=%d", err, f.restores)
	}
}

func TestOperationTimeExternalChangesSurviveApplyCleanup(t *testing.T) {
	external := Snapshot{Present: true, Position: Position{Latitude: 70, Longitude: 80}, Raw: []byte("external owner generation")}
	for _, phase := range []string{"apply error", "after apply", "inside cleanup CAS"} {
		t.Run(phase, func(t *testing.T) {
			f := &fakeBackend{current: Snapshot{Raw: []byte("initial owner")}}
			c := controllerFixture(t, f)
			switch phase {
			case "apply error":
				f.applyErr = os.ErrPermission
				f.applyHook = func(context.Context) { f.current = cloneSnapshot(external) }
			case "after apply":
				f.afterApplyHook = func() { f.current = cloneSnapshot(external) }
			case "inside cleanup CAS":
				f.afterApplyHook = func() { f.readErr = errors.New("one unavailable readback") }
				f.restoreHook = func(context.Context) { f.current = cloneSnapshot(external); f.readErr = nil }
			}
			result, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20})
			if err == nil || result.Report.Status != StatusRollback || !snapshotEqual(f.current, external) {
				t.Fatalf("external state overwritten: current=%+v result=%+v err=%v", f.current, result, err)
			}
			wantWrites := 1
			if phase == "apply error" {
				wantWrites = 0
			}
			if f.writes != wantWrites {
				t.Fatalf("cleanup performed an unowned write: writes=%d want=%d", f.writes, wantWrites)
			}
			b, err := c.loadBackup()
			if err != nil || b.Pending == nil || len(b.Pending.Raw) == 0 || string(b.Snapshot.Raw) != "initial owner" {
				t.Fatalf("recovery journal missing: %+v %v", b, err)
			}
		})
	}
}

func TestExternalChangeBetweenRestoreReadAndCASIsNotOverwritten(t *testing.T) {
	f := &fakeBackend{}
	c := controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
		t.Fatal(err)
	}
	external := Snapshot{Present: true, Position: f.current.Position, Raw: []byte("another owner with the same coordinates")}
	f.restoreHook = func(context.Context) { f.current = cloneSnapshot(external) }
	if _, err := c.Restore(context.Background()); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("restore conflict err=%v", err)
	}
	if !snapshotEqual(f.current, external) || f.writes != 1 {
		t.Fatalf("external owner overwritten: %+v writes=%d", f.current, f.writes)
	}
}

func TestNonemptyPreparedOwnershipSurvivesCrashAndRestart(t *testing.T) {
	original := Snapshot{Raw: []byte("original provider configuration")}
	f := &fakeBackend{current: cloneSnapshot(original)}
	c := controllerFixture(t, f)
	target, err := f.PrepareApply(context.Background(), original, Position{Latitude: 10, Longitude: 20})
	if err != nil || len(target.Raw) == 0 {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	b := backup{Version: backupVersion, Component: c.Component, Snapshot: cloneSnapshot(original), Expected: cloneSnapshot(original), Pending: snapshotPointer(target)}
	if err := c.writeBackup(b); err != nil {
		t.Fatal(err)
	}
	if err := f.CompareAndSwap(context.Background(), original, target); err != nil {
		t.Fatal(err)
	}
	// No successful-apply receipt is written. A fresh backend and controller
	// recover using the complete target ownership persisted before the write.
	restarted := &fakeBackend{current: cloneSnapshot(f.current)}
	recovered := c
	recovered.Backend = restarted
	if _, err := recovered.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !snapshotEqual(restarted.current, original) || restarted.writes != 1 {
		t.Fatalf("recovery=%+v writes=%d", restarted.current, restarted.writes)
	}
	b, err = c.loadBackup()
	if err != nil || b.Pending != nil || !snapshotEqual(b.Snapshot, original) {
		t.Fatalf("backup=%+v err=%v", b, err)
	}
}

func TestInterruptedRestoreOfInterruptedApplyRetainsActualSource(t *testing.T) {
	original := Snapshot{Raw: []byte("initial provider configuration")}
	previous := Snapshot{Present: true, Position: Position{Latitude: 1, Longitude: 2}, Raw: []byte("previous owned generation")}
	pending := Snapshot{Present: true, Position: Position{Latitude: 10, Longitude: 20}, Raw: []byte("pending owned generation")}
	f := &fakeBackend{current: cloneSnapshot(pending), restoreErr: errors.New("simulated interruption before restore CAS")}
	c := controllerFixture(t, f)
	b := backup{Version: backupVersion, Component: c.Component, Snapshot: cloneSnapshot(original), Expected: cloneSnapshot(previous), Pending: snapshotPointer(pending)}
	if err := c.writeBackup(b); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Restore(context.Background()); err == nil {
		t.Fatal("restore failure ignored")
	}
	b, err := c.loadBackup()
	if err != nil || !snapshotEqual(b.Expected, pending) || b.Pending == nil || !snapshotEqual(*b.Pending, original) {
		t.Fatalf("actual restore source was lost: %+v err=%v", b, err)
	}
	restarted := &fakeBackend{current: cloneSnapshot(pending)}
	c.Backend = restarted
	if _, err := c.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !snapshotEqual(restarted.current, original) {
		t.Fatalf("current=%+v", restarted.current)
	}
}

type legacyReadWriteBackend struct{ reads, writes int }

func (b *legacyReadWriteBackend) Read(context.Context) (Snapshot, error) {
	b.reads++
	return Snapshot{}, nil
}
func (b *legacyReadWriteBackend) Apply(context.Context, Position) error   { b.writes++; return nil }
func (b *legacyReadWriteBackend) Restore(context.Context, Snapshot) error { b.writes++; return nil }

func TestBackendWithoutOSAtomicContractIsRejectedBeforeMutation(t *testing.T) {
	b := &legacyReadWriteBackend{}
	c := controllerFixture(t, b)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); !errors.Is(err, ErrAtomicUnsupported) {
		t.Fatalf("apply err=%v", err)
	}
	if _, err := c.Restore(context.Background()); !errors.Is(err, ErrAtomicUnsupported) {
		t.Fatalf("restore err=%v", err)
	}
	if b.reads != 0 || b.writes != 0 {
		t.Fatalf("legacy backend reached: %+v", b)
	}
	if _, err := os.Stat(c.BackupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected backup err=%v", err)
	}
	f := &fakeBackend{atomicErr: ErrAtomicUnsupported}
	c = controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); !errors.Is(err, ErrAtomicUnsupported) {
		t.Fatalf("runtime capability err=%v", err)
	}
	if f.prepares != 0 || f.applies != 0 || f.writes != 0 {
		t.Fatalf("unsupported backend changed: %+v", f)
	}
	if _, err := os.Stat(c.BackupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected backup err=%v", err)
	}
}

func TestInvalidPreparedTargetIsRejectedBeforeJournalOrCAS(t *testing.T) {
	f := &invalidPreparedBackend{fakeBackend: &fakeBackend{}}
	c := controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err == nil {
		t.Fatal("target without ownership accepted")
	}
	if f.applies != 0 || f.writes != 0 {
		t.Fatalf("invalid preparation mutated: %+v", f)
	}
	if _, err := os.Stat(c.BackupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected journal err=%v", err)
	}
}

type invalidPreparedBackend struct{ *fakeBackend }

func (f *invalidPreparedBackend) PrepareApply(_ context.Context, _ Snapshot, p Position) (Snapshot, error) {
	return Snapshot{Present: true, Position: p}, nil
}

func TestBackendRawBufferReuseCannotChangeOriginalBackup(t *testing.T) {
	original := Snapshot{Raw: []byte("initial mutable provider buffer")}
	f := &fakeBackend{current: cloneSnapshot(original), reuseRaw: true, mutateCASArgs: true}
	f.prepareHook = func(before Snapshot) { before.Raw[0] = 'P' }
	c := controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
		t.Fatal(err)
	}
	b, err := c.loadBackup()
	if err != nil || !snapshotEqual(b.Snapshot, original) || !snapshotEqual(b.Expected, f.current) {
		t.Fatalf("aliased backup=%+v err=%v", b, err)
	}
	if _, err := c.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !snapshotEqual(f.current, original) {
		t.Fatalf("restoration Raw was mutated: %+v", f.current)
	}
}

func TestPreparedResultBufferIsCopiedBeforeBackendReusesIt(t *testing.T) {
	f := &fakeBackend{preparedRaw: []byte("prepared mutable ownership token")}
	originalToken := string(f.preparedRaw)
	f.afterApplyHook = func() { f.preparedRaw[0] = 'X' }
	c := controllerFixture(t, f)
	if _, err := c.Apply(context.Background(), Position{Latitude: 10, Longitude: 20}); err != nil {
		t.Fatal(err)
	}
	b, err := c.loadBackup()
	if err != nil || string(b.Expected.Raw) != originalToken {
		t.Fatalf("prepared token aliased: %+v %v", b, err)
	}
}

func TestInspectRejectsInvalidProviderSnapshot(t *testing.T) {
	f := &fakeBackend{current: Snapshot{Present: true, Position: Position{Latitude: 100}}}
	c := controllerFixture(t, f)
	r, err := c.Inspect(context.Background())
	if err != nil || r.Status != StatusUnverified || r.Position != nil || f.writes != 0 {
		t.Fatalf("inspect=%+v err=%v", r, err)
	}
}

func TestRejectRemoteAndWindowsAmbiguousPathsBeforeFileAccess(t *testing.T) {
	paths := []string{`\\example.invalid\share\component.exe`, `//example.invalid/share/component.exe`, `//?/UNC/example.invalid/share/backup.json`, `\\?\C:\component.exe`, `\\.\GLOBALROOT\component.exe`}
	if runtime.GOOS == "windows" {
		paths = append(paths, `C:\safe\backup.json:other-stream`, `C:\safe\..\backup.json`, `C:\safe\NUL.json`, `C:\safe\file.`, `C:\safe\file `)
	}
	for _, path := range paths {
		if err := validateLocalAbsolutePath(path); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
}
