package cli

import (
	"github.com/Maoyangui/godusevpn/internal/detection"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

const userJournalScopeNote = "管理归属与备份健康仅来自当前观察账户日志；机器日志未检查。"

// Additive fields keep existing JSON consumers working while making the source
// of Managed/BackupHealthy explicit. They describe this CLI read, not the health
// or existence of any machine journal. No helper or elevated read is performed.
type userJournalReportScope struct {
	BackupScope           string `json:"backupScope"`
	ManagedScope          string `json:"managedScope"`
	MachineJournalChecked bool   `json:"machineJournalChecked"`
	MachineJournalStatus  string `json:"machineJournalStatus"`
}

func currentUserJournalScope() userJournalReportScope {
	return userJournalReportScope{BackupScope: "current_process_user", ManagedScope: "current_process_user_journal", MachineJournalChecked: false, MachineJournalStatus: "not_inspected"}
}

type userPrivacyReport struct {
	privacy.Report
	userJournalReportScope
}

type userDetectionReport struct {
	detection.Report
	userJournalReportScope
}

func qualifyUserJournal(r privacy.Report) privacy.Report {
	// Do not append into the manager's caller-visible backing array.
	r.Warnings = append(append([]string{}, r.Warnings...), userJournalScopeNote)
	return r
}

func readUserPrivacyReport(read func() (privacy.Report, error)) (userPrivacyReport, error) {
	r, err := read()
	return userPrivacyReport{qualifyUserJournal(r), currentUserJournalScope()}, err
}
