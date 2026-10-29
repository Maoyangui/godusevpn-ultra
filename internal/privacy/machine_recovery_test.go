package privacy

import (
	"errors"
	"testing"
)

// The registry is always fake. These logic tests intentionally keep the normal
// temporary test store; the Windows ACL tests exercise the production store.
func fakeMachineManager(t *testing.T) (*Manager, *fakeBackend) {
	t.Helper()
	m, f := fakeManager(t)
	m.machineOnly = true
	m.backend = machineBackend{f}
	return m, f
}

func TestMachineSystemLikeRestoreIgnoresUserAndCurrentSKU(t *testing.T) {
	m, f := fakeMachineManager(t)
	id := "windows-clipboard-history"
	x := policy(t, id).Changes[0]
	before := value{Exists: true, Type: 3, Data: []byte("fixture-original")}
	f.values[x.Location] = before
	if _, err := m.Apply(id); err != nil {
		t.Fatal(err)
	}
	// A later elevated SYSTEM-like observer has no interactive user identity,
	// and the OS edition/build may have changed since the original application.
	f.env.SafeUser, f.env.UserKey = false, ""
	f.env.Edition, f.env.Build = "ServerDatacenter", 0
	s := state(t, m, id)
	if !s.Managed || s.ReadStatus != "match" || s.CanApply {
		t.Fatal("machine evidence or current-SKU eligibility is wrong", s)
	}
	if result, err := m.Restore(id); err != nil || result.Status != "restored" || !same(f.values[x.Location], before) {
		t.Fatal("SYSTEM-like recovery lost the original type/value", result, err)
	}
	j, err := m.load()
	if err != nil || len(j.Controls) != 0 {
		t.Fatal("completed machine restoration retained an entry", j, err)
	}
	for _, l := range f.reads {
		if !machineLocation(l) {
			t.Fatal("machine manager accessed a user or foreign key", l)
		}
	}
}

func TestMachineSystemLikeApplyAndRestoreAllRemainHKLMOnly(t *testing.T) {
	m, f := fakeMachineManager(t)
	f.env.SafeUser, f.env.UserKey = false, ""
	for _, id := range []string{"windows-advertising-id", "windows-clipboard-cloud"} {
		if _, err := m.Apply(id); err != nil {
			t.Fatal("supported elevated machine application required an interactive user", err)
		}
	}
	f.env.Edition, f.env.Build = "Core", 1
	results, err := m.RestoreAll()
	if err != nil || len(results) != 2 || len(f.values) != 0 {
		t.Fatal("current SKU prevented recovery of previously managed values", results, err)
	}
	for _, result := range results {
		if result.Status != "restored" {
			t.Fatal(result)
		}
	}
	reads, writes := len(f.reads), f.writes
	if _, err := m.Apply("chrome-location"); err == nil || len(f.reads) != reads || f.writes != writes {
		t.Fatal("machine manager accepted a browser operation")
	}
	if _, err := m.Apply("windows-advertising-id"); err == nil || f.writes != writes {
		t.Fatal("machine application ignored current SKU/build")
	}
}

func TestMachineRecoveryRequiresElevationBeforeStoreOrRegistry(t *testing.T) {
	m, f := fakeMachineManager(t)
	if _, err := m.Apply("windows-advertising-id"); err != nil {
		t.Fatal(err)
	}
	f.env.SafeUser, f.env.Admin = false, false
	f.reads = nil
	writes, locks := f.writes, 0
	m.lockStorage = func(string) (func(), error) { locks++; return func() {}, nil }
	if _, err := m.Restore("windows-advertising-id"); err == nil {
		t.Fatal("unprivileged restore accepted")
	}
	if _, err := m.RestoreAll(); err == nil {
		t.Fatal("unprivileged restore-all accepted")
	}
	r, err := m.Report()
	if err != nil || r.BackupHealthy || locks != 0 || f.writes != writes || len(f.reads) != 0 {
		t.Fatal("unprivileged machine scope touched storage/registry or claimed readable backup", r, err)
	}
}

func TestMachineSupportDoesNotOpenUserManagerToSystem(t *testing.T) {
	m, f := fakeManager(t)
	if _, err := m.Apply("chrome-location"); err != nil {
		t.Fatal(err)
	}
	f.env.SafeUser, f.env.Admin = false, true
	f.reads = nil
	writes := f.writes
	for _, id := range []string{"chrome-location", "windows-advertising-id"} {
		if _, err := m.Apply(id); err == nil {
			t.Fatal("SYSTEM-like user manager applied", id)
		}
		if _, err := m.Restore(id); err == nil {
			t.Fatal("SYSTEM-like user manager restored", id)
		}
	}
	if _, err := m.RestoreAll(); err == nil {
		t.Fatal("SYSTEM-like user restore-all accepted")
	}
	if _, err := m.Report(); err != nil || len(f.reads) != 0 || f.writes != writes {
		t.Fatal("machine support opened user registry access", err)
	}
}

func TestMachineScopeRejectsUserJournalAndForeignLocations(t *testing.T) {
	for _, mode := range []string{"user-binding", "user-location"} {
		t.Run(mode, func(t *testing.T) {
			m, f := fakeMachineManager(t)
			id := "windows-advertising-id"
			x := policy(t, id).Changes[0]
			j := journal{Version: 1, UserKey: m.owner(), Controls: map[string]savedControl{id: {Stage: "active", Changes: []savedChange{{x.Location, value{}, x.Desired}}}}}
			if mode == "user-binding" {
				j.UserKey = "fixture-user-journal"
			} else {
				saved := j.Controls[id]
				saved.Changes[0].Location.Hive = "HKCU"
				j.Controls[id] = saved
			}
			if err := m.save(j); err != nil {
				t.Fatal(err)
			}
			f.env.SafeUser = false
			if _, err := m.Restore(id); err == nil || f.writes != 0 {
				t.Fatal("machine scope adopted a user/forged journal", err)
			}
		})
	}
}

func TestMachineSystemLikeCrashRecoveryAndPersistenceFailure(t *testing.T) {
	m, f := fakeMachineManager(t)
	f.env.SafeUser = false
	id := "windows-online-speech"
	x := policy(t, id).Changes[0]
	j := journal{Version: 1, UserKey: m.owner(), Controls: map[string]savedControl{id: {Stage: "applying", Changes: []savedChange{{x.Location, value{}, x.Desired}}}}}
	if err := m.save(j); err != nil {
		t.Fatal(err)
	}
	f.values[x.Location] = x.Desired
	if result, err := m.Restore(id); err != nil || result.Status != "restored" || len(f.values) != 0 {
		t.Fatal("prepared machine backup was not recovered", result, err)
	}
	m.persist = func(string, []byte) error { return errors.New("fixture disk failure") }
	writes := f.writes
	if result, err := m.Apply(id); err == nil || result.Changed || f.writes != writes {
		t.Fatal("machine write proceeded without durable recovery data", result, err)
	}
}
