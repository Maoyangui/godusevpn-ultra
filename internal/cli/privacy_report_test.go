package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/detection"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

func assertUserJournalScope(t *testing.T, report any) []byte {
	t.Helper()
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err = json.Unmarshal(b, &value); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"backupScope": `"current_process_user"`, "managedScope": `"current_process_user_journal"`, "machineJournalChecked": "false", "machineJournalStatus": `"not_inspected"`} {
		if string(value[key]) != want {
			t.Fatalf("%s=%s expected %s", key, value[key], want)
		}
	}
	if _, ok := value["machineBackupHealthy"]; ok {
		t.Fatal("unchecked machine health acquired a boolean")
	}
	return b
}

func TestCliUserJournalScopeIsAdditiveAndNeverInfersMachineOwnership(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		fake := privacy.Report{Platform: "windows", Admin: true, ObserverElevated: true, BackupHealthy: healthy, Controls: []privacy.ControlState{{ID: "windows-example", RequiresAdmin: true, Status: "configured", Managed: false}}, Warnings: []string{"synthetic fixture warning"}}
		reads := 0
		r, err := readUserPrivacyReport(func() (privacy.Report, error) { reads++; return fake, nil })
		if err != nil || reads != 1 {
			t.Fatal("not a single user-report read", err)
		}
		b := assertUserJournalScope(t, r)
		// Old consumers can decode the original shape and obtain the same fields.
		var legacy privacy.Report
		if err = json.Unmarshal(b, &legacy); err != nil {
			t.Fatal(err)
		}
		if legacy.BackupHealthy != healthy || !legacy.Admin || !legacy.ObserverElevated || len(legacy.Controls) != 1 || legacy.Controls[0].Managed || legacy.Controls[0].Status != "configured" {
			t.Fatal("legacy facts changed or machine management invented")
		}
		if !strings.Contains(string(b), userJournalScopeNote) {
			t.Fatal("missing human-readable scope")
		}
		if len(fake.Warnings) != 1 {
			t.Fatal("source warnings modified")
		}
	}
}

func TestCliEmptyAndFailedReportsKeepExplicitJournalScope(t *testing.T) {
	for _, failure := range []error{nil, errors.New("synthetic read failure")} {
		reads := 0
		r, err := readUserPrivacyReport(func() (privacy.Report, error) { reads++; return privacy.Report{}, failure })
		if reads != 1 || !errors.Is(err, failure) {
			t.Fatal("read outcome lost")
		}
		assertUserJournalScope(t, r)
		if r.BackupHealthy || r.MachineJournalChecked || r.MachineJournalStatus != "not_inspected" {
			t.Fatal("empty/failed report promoted machine health")
		}
	}
}

func TestCliDetectionExportUsesSameUserJournalQualification(t *testing.T) {
	p := qualifyUserJournal(privacy.Report{BackupHealthy: true, Controls: []privacy.ControlState{{ID: "windows-example", RequiresAdmin: true, Category: "windows", Status: "configured", Managed: false}}})
	r := userDetectionReport{detection.Build(detection.Input{Policies: p}), currentUserJournalScope()}
	b := assertUserJournalScope(t, r)
	if !strings.Contains(string(b), userJournalScopeNote) {
		t.Fatal("detection export lost journal limit")
	}
	var legacy detection.Report
	if err := json.Unmarshal(b, &legacy); err != nil || len(legacy.Findings) == 0 {
		t.Fatal("legacy detection JSON broken", err)
	}
}
