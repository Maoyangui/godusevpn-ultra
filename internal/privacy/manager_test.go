package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every test injects fakeBackend; no test writes the real Windows registry.
type fakeBackend struct {
	env            environment
	values         map[location]value
	writes         int
	failWrites     map[int]bool
	mutateThenFail bool
	beforeWrite    func(location, value)
	reads          []location
	readFailures   map[location]bool
}

func (f *fakeBackend) environment() environment { return f.env }
func (f *fakeBackend) read(l location) (value, error) {
	f.reads = append(f.reads, l)
	if f.readFailures[l] {
		return value{}, errors.New("simulated read denied")
	}
	return f.values[l], nil
}
func (f *fakeBackend) write(l location, v value) error {
	f.writes++
	if f.beforeWrite != nil {
		f.beforeWrite(l, v)
	}
	if f.failWrites[f.writes] && !f.mutateThenFail {
		return errors.New("simulated failure")
	}
	if v.Exists {
		f.values[l] = v
	} else {
		delete(f.values, l)
	}
	if f.failWrites[f.writes] {
		return errors.New("simulated post-write flush failure")
	}
	return nil
}
func fakeManager(t *testing.T) (*Manager, *fakeBackend) {
	t.Helper()
	f := &fakeBackend{env: environment{Platform: "windows", Admin: true, SafeUser: true, Edition: "Professional", Build: 26100, UserKey: "fake-user"}, values: map[location]value{}, failWrites: map[int]bool{}}
	m := &Manager{dir: t.TempDir(), backend: f, persist: durableWrite}
	return m, f
}
func policy(t *testing.T, id string) control {
	t.Helper()
	c, ok := findControl(id)
	if !ok {
		t.Fatal(id)
	}
	return c
}
func state(t *testing.T, m *Manager, id string) ControlState {
	t.Helper()
	r, err := m.Report()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r.Controls {
		if s.ID == id {
			return s
		}
	}
	t.Fatal(id)
	return ControlState{}
}

func TestApplyDurableBeforeWriteAndRestoreExactOriginal(t *testing.T) {
	m, f := fakeManager(t)
	c := policy(t, "chrome-webrtc")
	l := c.Changes[0].Location
	original := textValue("default_public_interface_only")
	f.values[l] = original
	f.beforeWrite = func(_ location, _ value) {
		j, err := m.load()
		if err != nil || len(j.Controls[c.State.ID].Changes) == 0 {
			t.Fatal("registry write happened without a durable backup", err)
		}
	}
	r, err := m.Apply(c.State.ID)
	if err != nil || !r.Changed {
		t.Fatal(r, err)
	}
	if !same(f.values[l], c.Changes[0].Desired) {
		t.Fatal("not applied")
	}
	if s := state(t, m, c.State.ID); s.Status != "configured" || !s.Managed {
		t.Fatal(s)
	}
	r, err = m.Restore(c.State.ID)
	if err != nil || !r.Changed || !same(f.values[l], original) {
		t.Fatal(r, err)
	}
	j, err := m.load()
	if err != nil || len(j.Controls) != 0 {
		t.Fatal("backup not completed", err)
	}
}

func TestExistingTargetNotClaimedOrRemoved(t *testing.T) {
	m, f := fakeManager(t)
	c := policy(t, "edge-sensors")
	x := c.Changes[0]
	f.values[x.Location] = x.Desired
	r, err := m.Apply(c.State.ID)
	if err != nil || r.Changed || f.writes != 0 {
		t.Fatal(r, err)
	}
	r, err = m.Restore(c.State.ID)
	if err != nil || r.Status != "unmanaged" || !same(f.values[x.Location], x.Desired) {
		t.Fatal(r, err)
	}
}

func TestAbsentOriginalRestoresByDeletingOnlyValue(t *testing.T) {
	m, f := fakeManager(t)
	id := "chrome-location"
	c := policy(t, id)
	l := c.Changes[0].Location
	if _, err := m.Apply(id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(id); err != nil {
		t.Fatal(err)
	}
	if _, exists := f.values[l]; exists {
		t.Fatal("original missing value was not removed")
	}
}

func TestRestorePreservesExternalChangeAndRetainsBackup(t *testing.T) {
	m, f := fakeManager(t)
	id := "chrome-suggestions"
	l := policy(t, id).Changes[0].Location
	if _, err := m.Apply(id); err != nil {
		t.Fatal(err)
	}
	external := dword(77)
	f.values[l] = external
	writes := f.writes
	r, err := m.Restore(id)
	if err == nil || r.Status != "conflict" || f.writes != writes || !same(f.values[l], external) {
		t.Fatal(r, err)
	}
	j, err := m.load()
	if err != nil || len(j.Controls) != 1 {
		t.Fatal("must preserve recovery information", err)
	}
	// User restores the known applied state; retry can now restore the original.
	f.values[l] = policy(t, id).Changes[0].Desired
	if _, err = m.Restore(id); err != nil {
		t.Fatal(err)
	}
}

func TestCrashAfterWriteBeforeCompletionCanRestore(t *testing.T) {
	m, f := fakeManager(t)
	id := "edge-notifications"
	x := policy(t, id).Changes[0]
	j := journal{1, "fake-user", map[string]savedControl{id: {Stage: "applying", Changes: []savedChange{{x.Location, value{}, x.Desired}}}}}
	if err := m.save(j); err != nil {
		t.Fatal(err)
	}
	f.values[x.Location] = x.Desired
	if s := state(t, m, id); s.Status != "recovery_pending" {
		t.Fatal(s)
	}
	if _, err := m.Apply(id); err == nil {
		t.Fatal("must not replace the original crash backup")
	}
	if r, err := m.Restore(id); err != nil || r.Status != "restored" {
		t.Fatal(r, err)
	}
}

func TestCrashBeforeRegistryWriteRestorationIsNoOp(t *testing.T) {
	m, f := fakeManager(t)
	id := "edge-camera"
	x := policy(t, id).Changes[0]
	before := dword(1)
	f.values[x.Location] = before
	j := journal{1, "fake-user", map[string]savedControl{id: {Stage: "applying", Changes: []savedChange{{x.Location, before, x.Desired}}}}}
	if err := m.save(j); err != nil {
		t.Fatal(err)
	}
	r, err := m.Restore(id)
	if err != nil || r.Changed || f.writes != 0 {
		t.Fatal(r, err)
	}
}

func TestPersistenceFailurePreventsRegistryMutation(t *testing.T) {
	m, f := fakeManager(t)
	m.persist = func(string, []byte) error { return errors.New("disk full") }
	if r, err := m.Apply("chrome-location"); err == nil || r.Changed || f.writes != 0 {
		t.Fatal(r, err)
	}
}

func TestWriteThenFlushFailureRollsBack(t *testing.T) {
	m, f := fakeManager(t)
	f.failWrites[1] = true
	f.mutateThenFail = true
	r, err := m.Apply("chrome-location")
	if err == nil || r.Status != "rolled_back" || len(f.values) != 0 {
		t.Fatal(r, err, f.values)
	}
	j, jerr := m.load()
	if jerr != nil || len(j.Controls) != 0 {
		t.Fatal(j, jerr)
	}
}

func TestFailedRollbackRetainsRecoverableJournal(t *testing.T) {
	m, f := fakeManager(t)
	f.failWrites[1] = true
	f.failWrites[2] = true
	f.mutateThenFail = true
	r, err := m.Apply("chrome-location")
	if err == nil || r.Status != "recovery_pending" {
		t.Fatal(r, err)
	}
	// The second write applied the original but reported failure. Retrying detects
	// the original and safely completes without writing the value again.
	f.failWrites = map[int]bool{}
	writes := f.writes
	r, err = m.Restore("chrome-location")
	if err != nil || r.Status != "restored" || f.writes != writes {
		t.Fatal(r, err)
	}
}

func TestCompletionSaveFailureRollsBackUsingPreparedBackup(t *testing.T) {
	m, f := fakeManager(t)
	calls := 0
	m.persist = func(p string, b []byte) error {
		calls++
		if calls == 2 {
			return errors.New("completion save failed")
		}
		return durableWrite(p, b)
	}
	r, err := m.Apply("chrome-location")
	if err == nil || r.Status != "rolled_back" || len(f.values) != 0 {
		t.Fatal(r, err)
	}
}

func TestRestoreAllContinuesAfterOneFailure(t *testing.T) {
	m, f := fakeManager(t)
	for _, id := range []string{"chrome-location", "edge-location"} {
		if _, err := m.Apply(id); err != nil {
			t.Fatal(err)
		}
	}
	f.failWrites[3] = true
	results, err := m.RestoreAll()
	if err == nil || len(results) != 2 || results[1].Status != "restored" {
		t.Fatal(results, err)
	}
	j, jerr := m.load()
	if jerr != nil || len(j.Controls) != 1 {
		t.Fatal(j, jerr)
	}
	f.failWrites = map[int]bool{}
	results, err = m.RestoreAll()
	if err != nil || len(results) != 1 || results[0].Status != "restored" {
		t.Fatal(results, err)
	}
}

func TestCorruptBackupKeepsReportReadableAndBlocksWrites(t *testing.T) {
	m, f := fakeManager(t)
	if err := os.WriteFile(filepath.Join(m.dir, journalName), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := m.Report()
	if err != nil || r.BackupHealthy || len(r.Controls) == 0 {
		t.Fatal(r, err)
	}
	for _, s := range r.Controls {
		if s.CanApply {
			t.Fatal("corrupt backup offered write", s.ID)
		}
	}
	if _, err = m.Apply("chrome-location"); err == nil {
		t.Fatal("must reject write")
	}
	if _, err = m.Restore("chrome-location"); err == nil {
		t.Fatal("must reject restore")
	}
	if f.writes != 0 {
		t.Fatal("registry mutated")
	}
}

func TestValidChecksumCannotAuthorizeForeignRegistryPath(t *testing.T) {
	m, f := fakeManager(t)
	id := "chrome-location"
	x := policy(t, id).Changes[0]
	x.Location.Path = `SOFTWARE\Unrelated`
	j := journal{1, "fake-user", map[string]savedControl{id: {Stage: "active", Changes: []savedChange{{x.Location, value{}, x.Desired}}}}}
	if err := m.save(j); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Restore(id); err == nil || f.writes != 0 {
		t.Fatal("foreign path accepted", err)
	}
}

func TestBackupUserBindingAndChecksum(t *testing.T) {
	for _, mode := range []string{"user", "checksum"} {
		t.Run(mode, func(t *testing.T) {
			m, f := fakeManager(t)
			if _, err := m.Apply("chrome-location"); err != nil {
				t.Fatal(err)
			}
			if mode == "user" {
				f.env.UserKey = "another-user"
			} else {
				p := filepath.Join(m.dir, journalName)
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				var e envelope
				if err = json.Unmarshal(b, &e); err != nil {
					t.Fatal(err)
				}
				e.SHA256 = strings.Repeat("0", 64)
				b, _ = json.Marshal(e)
				if err = os.WriteFile(p, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			writes := f.writes
			if _, err := m.Restore("chrome-location"); err == nil || f.writes != writes {
				t.Fatal("invalid backup accepted", err)
			}
		})
	}
}

func TestAdminPlatformAndUserBoundaries(t *testing.T) {
	m, f := fakeManager(t)
	f.env.Admin = false
	if _, err := m.Apply("chrome-location"); err != nil {
		t.Fatal("HKCU should not need administrator", err)
	}
	if _, err := m.Apply("windows-advertising-id"); err == nil {
		t.Fatal("machine write accepted without admin")
	}
	f.env.SafeUser = false
	if _, err := m.Apply("edge-location"); err == nil {
		t.Fatal("service user accepted")
	}
	f.env.SafeUser = true
	f.env.Admin = true
	f.env.Edition = "Core"
	if _, err := m.Apply("windows-advertising-id"); err == nil {
		t.Fatal("unverified Home policy accepted")
	}
	f.env.Platform = "darwin"
	if _, err := m.Apply("edge-location"); err == nil {
		t.Fatal("non-Windows accepted")
	}
}

func TestMachinePolicyNotOverwrittenByUserPolicy(t *testing.T) {
	m, f := fakeManager(t)
	l := policy(t, "chrome-webrtc").Changes[0].Location
	l.Hive = "HKLM"
	f.values[l] = textValue("default")
	s := state(t, m, "chrome-webrtc")
	if s.CanApply || s.Status != "policy_conflict" {
		t.Fatal(s)
	}
	if _, err := m.Apply("chrome-webrtc"); err == nil || f.writes != 0 {
		t.Fatal("machine policy overridden", err)
	}
}

func TestCatalogBoundariesAndNoSensitiveReport(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range catalog() {
		if seen[c.State.ID] || len(c.State.Sources) == 0 {
			t.Fatal("invalid catalog", c.State.ID)
		}
		seen[c.State.ID] = true
		for _, x := range c.Changes {
			if !validValue(x.Desired) || !strings.HasPrefix(x.Location.Path, `SOFTWARE\Policies\`) {
				t.Fatal(x)
			}
			for _, forbidden := range []string{"SafeBrowsing", "SmartScreen", "WindowsUpdate", "ProxyServer", "NameServer", "Root", "WebGpu"} {
				if strings.Contains(x.Location.Name, forbidden) {
					t.Fatal("security/tunnel/unsupported policy", x.Location.Name)
				}
			}
		}
	}
	m, _ := fakeManager(t)
	r, err := m.Report()
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-user", "userKey", "\\\"data\\\"", "before"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("report exposed backup internals", secret)
		}
	}
}

func TestMalformedJournalPayloadWithCorrectHashRejected(t *testing.T) {
	m, f := fakeManager(t)
	p := []byte(`{"version":1,"userKey":"fake-user","controls":{},"unexpected":true}`)
	sum := sha256.Sum256(p)
	b, _ := json.Marshal(envelope{p, hex.EncodeToString(sum[:])})
	if err := os.WriteFile(filepath.Join(m.dir, journalName), b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply("chrome-location"); err == nil || f.writes != 0 {
		t.Fatal("unknown schema accepted", err)
	}
}

func TestMachineHelperNeverReadsUserKeysAndSupportsOtherAdministrator(t *testing.T) {
	m, f := fakeManager(t)
	m.machineOnly = true
	m.backend = machineBackend{f}
	r, err := m.Report()
	if err != nil || len(r.Controls) != 4 || len(r.Capabilities) != 0 {
		t.Fatal(r, err)
	}
	for _, l := range f.reads {
		if l.Hive != "HKLM" {
			t.Fatal("helper read user policy", l)
		}
	}
	reads := len(f.reads)
	if _, err := m.Apply("chrome-location"); err == nil || len(f.reads) != reads {
		t.Fatal("helper accepted browser operation")
	}
	if _, err := m.Apply("windows-advertising-id"); err != nil {
		t.Fatal(err)
	}
	f.env.UserKey = "another-authorized-admin"
	if _, err := m.Restore("windows-advertising-id"); err != nil {
		t.Fatal("machine restoration incorrectly bound to original UAC identity", err)
	}
	for _, l := range f.reads {
		if l.Hive != "HKLM" {
			t.Fatal("helper read user policy", l)
		}
	}
}

func TestDecliningUACDoesNotPreventReadingMachinePolicyEvidence(t *testing.T) {
	m, f := fakeManager(t)
	f.env.Admin = false
	id := "windows-advertising-id"
	x := policy(t, id).Changes[0]
	f.values[x.Location] = x.Desired
	s := state(t, m, id)
	if s.CanApply || s.Status != "manual" || s.ReadStatus != "match" {
		t.Fatal("read and write permissions were conflated", s)
	}
	read := false
	for _, l := range f.reads {
		if l == x.Location {
			read = true
		}
	}
	if !read || f.writes != 0 {
		t.Fatal("machine evidence not read locally, or a write occurred")
	}
	if _, err := m.Apply(id); err == nil || f.writes != 0 {
		t.Fatal("readonly evidence enabled write", err)
	}
}

func TestUnverifiedEditionCanReadButDoesNotClaimApplicability(t *testing.T) {
	m, f := fakeManager(t)
	f.env.Edition = "Core"
	id := "windows-clipboard-cloud"
	x := policy(t, id).Changes[0]
	f.values[x.Location] = x.Desired
	s := state(t, m, id)
	if s.CanApply || s.Status != "manual" || s.ReadStatus != "match" {
		t.Fatal("Home registry match promoted to supported policy", s)
	}
}

func TestUnreadableManagedPolicyIsUnknownNotExternalChange(t *testing.T) {
	m, f := fakeManager(t)
	id := "chrome-location"
	if _, err := m.Apply(id); err != nil {
		t.Fatal(err)
	}
	l := policy(t, id).Changes[0].Location
	f.readFailures = map[location]bool{l: true}
	s := state(t, m, id)
	if s.Status != "unknown" || s.ReadStatus != "unknown" || s.CanApply || !s.Managed {
		t.Fatal("read failure became proof of external change", s)
	}
}

func TestUnsafeUserReportDoesNotReadEvenManagedUserKeys(t *testing.T) {
	m, f := fakeManager(t)
	id := "chrome-location"
	if _, err := m.Apply(id); err != nil {
		t.Fatal(err)
	}
	f.env.SafeUser = false
	f.reads = nil
	s := state(t, m, id)
	if s.CanApply || s.Status != "unknown" || len(f.reads) != 0 {
		t.Fatal("unsafe context read managed user policy", s, f.reads)
	}
}
