package detection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

func finding(t *testing.T, r Report, id string) Finding {
	t.Helper()
	for _, f := range r.Findings {
		if f.ID == id {
			return f
		}
	}
	t.Fatal("missing", id)
	return Finding{}
}

func TestHelperAuthorizationRequiresExplicitDesktopSource(t *testing.T) {
	for _, tc := range []struct {
		mode                   string
		elevated, helper, want bool
	}{
		{"direct_cli", true, true, false}, {"direct_cli", false, true, false}, {"", true, true, false}, {"desktop_ui", false, true, true}, {"desktop_ui", true, false, false},
	} {
		r := Build(Input{ExecutionMode: tc.mode, HelperAuthorized: tc.helper, Policies: privacy.Report{Admin: true}, Facts: Facts{Elevated: tc.elevated}})
		if r.HelperAuthorized != tc.want || r.Admin != tc.elevated {
			t.Fatal("identity inferred", r)
		}
		if tc.mode == "direct_cli" && tc.elevated && !strings.Contains(r.Observer, "直接提升的 CLI") {
			t.Fatal(r.Observer)
		}
	}
}

func TestUnknownFilterCountIsIndependentOfRulesAndWarningText(t *testing.T) {
	r := Build(Input{Firewall: appfirewall.Report{UnknownFilters: 3}})
	f := finding(t, r, "network-firewall-unknown")
	if f.Count == nil || *f.Count != 3 || f.Status != "unknown" {
		t.Fatal(f)
	}
	b, _ := json.Marshal(f)
	if !strings.Contains(string(b), `"count":3`) {
		t.Fatal(string(b))
	}
	r = Build(Input{Firewall: appfirewall.Report{Warnings: []string{"there are 99 unknown filters"}}})
	for _, f := range r.Findings {
		if f.ID == "network-firewall-unknown" {
			t.Fatal("guessed number from warnings")
		}
	}
}

func TestStorageEvidenceUnknownWhenAbsentAndBoundedWhenConfigured(t *testing.T) {
	days := 7
	for _, n := range []Network{{}, {Available: false, SettingsAvailable: true, LocalDNSMode: "doh", RemoteDNSMode: "doh", LogLevel: "info", LogDays: &days}, {Available: true, SettingsAvailable: true, LocalDNSMode: "secret-resolver", RemoteDNSMode: "secret-remote", LogLevel: "secret-token", LogDays: &days}} {
		r := Build(Input{Network: n})
		for _, id := range []string{"network-dns", "local-logs"} {
			f := finding(t, r, id)
			if f.Status != "unknown" || !strings.Contains(f.Summary, "没有可用") {
				t.Fatal(f)
			}
		}
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), "secret-") {
			t.Fatal("raw config escaped")
		}
	}
	for _, keep := range []int{0, 7} {
		r := Build(Input{Network: Network{Available: true, SettingsAvailable: true, LocalDNSMode: "system", RemoteDNSMode: "doh", LogLevel: "debug", LogDays: &keep}, SourceTimes: map[string]string{"network": "2026-10-19T00:00:00Z"}})
		for _, id := range []string{"network-dns", "local-logs"} {
			f := finding(t, r, id)
			if f.Status != "unknown" || f.DetectedAt == "" || !strings.Contains(f.Scope, "配置") {
				t.Fatal(f)
			}
		}
		f := finding(t, r, "credentials-storage")
		if f.DetectedAt != "" || !strings.Contains(f.Summary, "尚未实现 DPAPI") {
			t.Fatal(f)
		}
		if strings.Contains(finding(t, r, "selected-files").Summary, "未选择") {
			t.Fatal("invented file selection")
		}
	}
}
