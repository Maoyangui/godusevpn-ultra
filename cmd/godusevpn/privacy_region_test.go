package main

import (
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/regionprivacy"
)

type fakePrivacyRegion struct {
	app   *App
	calls []string
	err   error
	epoch uint64
}

func (f *fakePrivacyRegion) Report() (regionprivacy.Report, error) {
	f.calls = append(f.calls, "report")
	return regionprivacy.Report{CurrentCode: "US", Scope: regionprivacy.Scope}, f.err
}
func (f *fakePrivacyRegion) Apply(code string) (regionprivacy.Result, error) {
	f.calls = append(f.calls, "apply:"+code)
	f.epoch = f.app.privacyEvidenceEpoch
	return regionprivacy.Result{CurrentCode: code}, f.err
}
func (f *fakePrivacyRegion) Restore() (regionprivacy.Result, error) {
	f.calls = append(f.calls, "restore")
	f.epoch = f.app.privacyEvidenceEpoch
	return regionprivacy.Result{CurrentCode: "US"}, f.err
}
func TestPrivacyRegionBridgeUsesCurrentProcessManagerAndInvalidatesEvidence(t *testing.T) {
	original := newPrivacyRegionManager
	defer func() { newPrivacyRegionManager = original }()
	for _, failure := range []bool{false, true} {
		a := &App{}
		fake := &fakePrivacyRegion{app: a}
		if failure {
			fake.err = errors.New("fake region failure")
		}
		newPrivacyRegionManager = func() privacyRegionManager { return fake }
		report, err := a.PrivacyRegionReport()
		if report.CurrentCode != "US" || (err != nil) != failure || a.privacyEvidenceEpoch != 0 {
			t.Fatal(report, err)
		}
		result, err := a.PrivacyRegionApply("001")
		if result.CurrentCode != "001" || (err != nil) != failure || fake.epoch != 1 || a.privacyEvidenceEpoch != 2 {
			t.Fatal(result, err, fake.epoch, a.privacyEvidenceEpoch)
		}
		result, err = a.PrivacyRegionRestore()
		if result.CurrentCode != "US" || (err != nil) != failure || fake.epoch != 3 || a.privacyEvidenceEpoch != 4 {
			t.Fatal(result, err)
		}
		if len(fake.calls) != 3 || fake.calls[0] != "report" || fake.calls[1] != "apply:001" || fake.calls[2] != "restore" {
			t.Fatal(fake.calls)
		}
		// No helper is created, requested or used by these current-user calls.
		if a.privacyHost != nil {
			t.Fatal("per-user write routed through elevated helper")
		}
	}
}
