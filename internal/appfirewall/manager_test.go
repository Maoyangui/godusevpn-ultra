package appfirewall

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// All tests use this in-memory backend. No test invokes Windows firewall writes.
type fakeBackend struct {
	admin               bool
	items               []installed
	mutations           int
	failAt              int
	commitError         bool
	readError           bool
	afterWriteReadError bool
	trace               []string
}

func (f *fakeBackend) supported() bool { return true }
func (f *fakeBackend) elevated() bool  { return f.admin }
func (f *fakeBackend) snapshot() ([]installed, []string, error) {
	if f.readError {
		return nil, nil, errors.New("fake read failure")
	}
	return append([]installed(nil), f.items...), nil, nil
}
func fakeFilters(r Rule) []installed {
	var out []installed
	for _, s := range specs(r) {
		out = append(out, installed{Rule: r, Key: r.ID + ":" + s.Suffix, Matches: true})
	}
	return out
}
func (f *fakeBackend) transaction(change func(*[]installed) error) error {
	var staged []installed
	return runTransaction(transactionHooks{
		begin: func() error {
			f.trace = append(f.trace, "begin")
			staged = append([]installed(nil), f.items...)
			return nil
		},
		commit: func() error {
			f.trace = append(f.trace, "commit")
			if f.commitError {
				return errors.New("fake commit failure")
			}
			f.items = staged
			if f.afterWriteReadError {
				f.readError = true
			}
			return nil
		},
		abort: func() error { f.trace = append(f.trace, "abort"); return nil },
	}, func() error { return change(&staged) })
}
func (f *fakeBackend) replace(r Rule) error {
	f.mutations++
	return f.transaction(func(staged *[]installed) error {
		var next []installed
		for _, item := range *staged {
			if item.Rule.ID != r.ID {
				next = append(next, item)
			}
		}
		*staged = next
		for i, item := range fakeFilters(r) {
			if f.failAt == i+1 {
				return errors.New("fake filter add failure")
			}
			*staged = append(*staged, item)
		}
		return nil
	})
}
func (f *fakeBackend) remove(id string) error {
	f.mutations++
	return f.transaction(func(staged *[]installed) error {
		var next []installed
		for _, item := range *staged {
			if item.Unknown || item.Rule.ID != id {
				next = append(next, item)
			}
		}
		*staged = next
		return nil
	})
}

func temporaryRule(t *testing.T) Rule {
	t.Helper()
	p := filepath.Join(t.TempDir(), "selected.exe")
	if err := os.WriteFile(p, []byte("fixture only"), 0600); err != nil {
		t.Fatal(err)
	}
	return Rule{ID: "app-1", Program: p, Mode: "block_lan"}
}

func TestApplyUsesAtomicReplacementAndReportsOnlyInstallation(t *testing.T) {
	r := temporaryRule(t)
	old := r
	old.Mode = "block_all"
	other := r
	other.ID = "unrelated"
	f := &fakeBackend{admin: true, items: append(fakeFilters(old), fakeFilters(other)...)}
	m := &Manager{backend: f}
	result, err := m.Apply(r)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || !result.Persistent || !result.Verified || result.State != "filters_installed" {
		t.Fatalf("wrong installation evidence: %+v", result)
	}
	report, err := m.Report()
	if err != nil {
		t.Fatal(err)
	}
	if report.Enforcement != "not_live_tested" || report.Loopback != "unprotected" {
		t.Fatalf("overclaims enforcement: %+v", report)
	}
	if len(report.Rules) != 2 || len(f.items) != len(specs(r))+len(specs(other)) {
		t.Fatalf("unexpected rule replacement: %+v", report)
	}
	if !reflect.DeepEqual(f.trace, []string{"begin", "commit"}) {
		t.Fatal(f.trace)
	}
	if !strings.Contains(result.Warning, "硬放行") {
		t.Fatal("must surface hard-permit conflict")
	}
}

func TestFailedTransactionsLeaveAllOriginalFilters(t *testing.T) {
	for _, commitFail := range []bool{false, true} {
		t.Run(fmt.Sprint(commitFail), func(t *testing.T) {
			r := temporaryRule(t)
			old := r
			old.Mode = "block_all"
			original := fakeFilters(old)
			f := &fakeBackend{admin: true, items: append([]installed(nil), original...), commitError: commitFail}
			if !commitFail {
				f.failAt = 2
			}
			result, err := (&Manager{backend: f}).Apply(r)
			if err == nil || result.Applied {
				t.Fatalf("failure reported as applied: %+v %v", result, err)
			}
			if !reflect.DeepEqual(f.items, original) {
				t.Fatal("partial replacement escaped failed transaction")
			}
			if len(f.trace) == 0 || f.trace[len(f.trace)-1] != "abort" {
				t.Fatal(f.trace)
			}
		})
	}
}

func TestTransactionAbortOnPanicAndBeginFailure(t *testing.T) {
	var trace []string
	h := transactionHooks{begin: func() error { trace = append(trace, "begin"); return nil }, commit: func() error { trace = append(trace, "commit"); return nil }, abort: func() error { trace = append(trace, "abort"); return nil }}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("missing panic")
			}
		}()
		_ = runTransaction(h, func() error { panic("fake") })
	}()
	if !reflect.DeepEqual(trace, []string{"begin", "abort"}) {
		t.Fatal(trace)
	}
	trace = nil
	h.begin = func() error { return errors.New("begin denied") }
	if err := runTransaction(h, func() error { t.Fatal("change ran after begin failure"); return nil }); err == nil {
		t.Fatal("expected failure")
	}
	if len(trace) != 0 {
		t.Fatal(trace)
	}
}

func TestPostCommitReadFailureDoesNotPretendRollback(t *testing.T) {
	r := temporaryRule(t)
	f := &fakeBackend{admin: true, afterWriteReadError: true}
	result, err := (&Manager{backend: f}).Apply(r)
	if err == nil || !result.Applied || result.Verified || result.State != "installed_unverified" || len(f.items) == 0 {
		t.Fatalf("must distinguish committed from verified: %+v %v", result, err)
	}
}

func TestMissingAdministratorAndReadFailureDoNotMutate(t *testing.T) {
	r := temporaryRule(t)
	for _, admin := range []bool{false, true} {
		f := &fakeBackend{admin: admin, readError: admin}
		_, err := (&Manager{backend: f}).Apply(r)
		if err == nil || f.mutations != 0 {
			t.Fatalf("unexpected mutation admin=%v: %v", admin, err)
		}
	}
}

func TestRemoveOnlySelectedIDAndRequiresSuccessfulCommit(t *testing.T) {
	r := temporaryRule(t)
	other := r
	other.ID = "other"
	f := &fakeBackend{admin: true, items: append(fakeFilters(r), fakeFilters(other)...)}
	m := &Manager{backend: f}
	f.commitError = true
	if _, err := m.Remove(r.ID); err == nil || len(f.items) != len(specs(r))*2 {
		t.Fatal("failed remove changed state")
	}
	f.commitError = false
	result, err := m.Remove(r.ID)
	if err != nil || !result.Verified || result.State != "removed" {
		t.Fatalf("%+v %v", result, err)
	}
	if !reflect.DeepEqual(f.items, fakeFilters(other)) {
		t.Fatal("removed unrelated filters")
	}
	before := f.mutations
	if _, err = m.Remove("*"); err == nil || f.mutations != before {
		t.Fatal("accepted wildcard deletion")
	}
}

func TestReportRejectsPartialDisabledAndConflictingMetadata(t *testing.T) {
	r := temporaryRule(t)
	for _, kind := range []string{"missing", "mismatch", "duplicate", "metadata"} {
		t.Run(kind, func(t *testing.T) {
			items := fakeFilters(r)
			switch kind {
			case "missing":
				items = items[:len(items)-1]
			case "mismatch":
				items[0].Matches = false
			case "duplicate":
				items[1].Key = items[0].Key
			case "metadata":
				items[0].Rule.Program += "changed"
			}
			report, err := (&Manager{backend: &fakeBackend{admin: true, items: items}}).Report()
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Rules) != 1 || report.Rules[0].State != "partial" {
				t.Fatalf("overclaimed: %+v", report)
			}
		})
	}
}

func TestProgramValidationRefusesDangerousTargetsAndHasNoShellSurface(t *testing.T) {
	r := temporaryRule(t)
	for _, target := range []string{"relative.exe", "*", filepath.Join(filepath.Dir(r.Program), "godusevpn-svc.exe"), filepath.Join(filepath.Dir(r.Program), "sing-box.exe"), r.Program + "\x00", "\\\\server\\share\\foo.exe"} {
		r.Program = target
		if err := validateRule(r, false); err == nil {
			t.Errorf("accepted unsafe target %q", target)
		}
	}
	r = temporaryRule(t)
	r.ID = "x; Remove-All"
	if err := validateRule(r, false); err == nil {
		t.Fatal("accepted unsafe ID")
	}
	r = temporaryRule(t)
	r.Mode = "allow_global"
	if err := validateRule(r, false); err == nil {
		t.Fatal("accepted global policy")
	}
}

func TestSpecificationsCoverBothFamiliesAndDirections(t *testing.T) {
	for mode, count := range map[string]int{"block_all": 4, "block_lan": 12, "block_discovery": 6} {
		ss := specs(Rule{Mode: mode})
		if len(ss) != count {
			t.Fatal(mode, len(ss))
		}
		seen := map[string]bool{}
		for _, s := range ss {
			if seen[s.Suffix] {
				t.Fatal("duplicate specification")
			}
			seen[s.Suffix] = true
			if strings.Contains(s.Prefix, "127.") || s.Prefix == "::1/128" {
				t.Fatal("must not claim loopback protection")
			}
		}
	}
	if len(specs(Rule{Mode: "unknown"})) != 0 {
		t.Fatal("unexpected global fallback")
	}
}

func TestUIJSONFieldContract(t *testing.T) {
	b, err := json.Marshal(RuleStatus{InstalledFilters: 2, ExpectedFilters: 4})
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, `"installedFilters":2`) || !strings.Contains(text, `"expectedFilters":4`) || strings.Contains(text, "installed_filters") {
		t.Fatal(text)
	}
}

func TestUnknownPersistentFiltersRemainCountedSeparately(t *testing.T) {
	r := temporaryRule(t)
	items := append(fakeFilters(r), installed{Key: "unknown-owned-key", Unknown: true}, installed{Unknown: true})
	f := &fakeBackend{admin: true, items: items}
	m := &Manager{backend: f}
	report, err := m.Report()
	if err != nil {
		t.Fatal(err)
	}
	if report.UnknownFilters != 2 || len(report.Rules) != 1 || report.Rules[0].State != "filters_installed" {
		t.Fatalf("lost independent unknown evidence: %+v", report)
	}
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"unknownFilters":2`) {
		t.Fatal(string(b))
	}
	if result, e := m.Remove(r.ID); e == nil || result.Verified || result.State != "unknown_remnants" {
		t.Fatal("unknown remnants were falsely completed", result, e)
	}
	report, err = m.Report()
	if err != nil {
		t.Fatal(err)
	}
	if report.UnknownFilters != 2 || len(report.Rules) != 0 {
		t.Fatalf("removing a known rule hid unknown retained filters: %+v", report)
	}
}
