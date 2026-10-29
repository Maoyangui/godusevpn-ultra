package detection

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

func TestConfigurationNeverBecomesRuntimeProtection(t *testing.T) {
	r := Build(Input{Policies: privacy.Report{Controls: []privacy.ControlState{{ID: "chrome-webgl", Title: "WebGL", Status: "configured", Category: "browser"}}}, Firewall: appfirewall.Report{Rules: []appfirewall.RuleStatus{{Rule: appfirewall.Rule{ID: "r", Program: `C:\private-person\secret.exe`}, State: "filters_installed"}}}})
	for _, f := range r.Findings {
		if f.Status == "restricted" {
			t.Fatalf("configuration promoted to protection: %+v", f)
		}
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "private-person") || strings.Contains(string(b), "secret.exe") {
		t.Fatal("report exported program path")
	}
}

func TestAdminObservationAndBrowserScopeRemainExplicit(t *testing.T) {
	r := Build(Input{Facts: Facts{Platform: "windows", Elevated: true, TimeZone: "CST", Locale: "zh-CN", Camera: "Deny"}})
	if !strings.Contains(r.Observer, "管理员") {
		t.Fatal(r.Observer)
	}
	for _, f := range r.Findings {
		if f.Audience == "website" && f.Status != "unknown" {
			t.Fatalf("invented browser observation: %+v", f)
		}
		if f.ID == "camera" && f.Status == "restricted" {
			t.Fatal("parent Deny is not universal deny")
		}
	}
}

func TestExplicitAllowAndMissingEvidence(t *testing.T) {
	r := Build(Input{Facts: Facts{Location: "Allow"}, Network: Network{Available: true, Mode: "rule", LANBypass: true}})
	want := map[string]string{"location": "allowed", "camera": "unknown", "network-path": "possible", "native-hardware": "possible"}
	for _, f := range r.Findings {
		if status, ok := want[f.ID]; ok {
			if f.Status != status {
				t.Errorf("%s=%s want %s", f.ID, f.Status, status)
			}
			delete(want, f.ID)
		}
	}
	if len(want) != 0 {
		t.Fatal(want)
	}
}

func TestBoundedConsentFactsExposeNotificationAndClipboardOnlyAsObservations(t *testing.T) {
	r := Build(Input{Facts: Facts{Notifications: "Allow", Clipboard: "Deny"}})
	want := map[string]string{"notifications": "allowed", "clipboard": "unknown"}
	for _, f := range r.Findings {
		if status, ok := want[f.ID]; ok {
			if f.Status != status {
				t.Fatalf("%s=%s want %s", f.ID, f.Status, status)
			}
			if f.ID == "notifications" && !strings.Contains(f.Evidence, "默认访问记录") {
				t.Fatal("notification evidence did not identify bounded default record")
			}
			delete(want, f.ID)
		}
	}
	if len(want) != 0 {
		t.Fatal("missing consent findings", want)
	}
}

func TestReportTimeUsesClientTimezone(t *testing.T) {
	r := Build(Input{At: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)})
	if r.GeneratedAt != "2026-10-19T08:00:00+08:00" {
		t.Fatal(r.GeneratedAt)
	}
}

func TestHelperPrivilegeIsNotObserverPrivilege(t *testing.T) {
	r := Build(Input{ExecutionMode: "desktop_ui", HelperAuthorized: true, Policies: privacy.Report{Admin: true, Warnings: []string{`backup C:\Users\private-name\journal.json failed`}}, Facts: Facts{Elevated: false}, Firewall: appfirewall.Report{Warnings: []string{"仲裁边界"}}})
	if r.Admin || !r.HelperAuthorized {
		t.Fatal("helper and observer identity were conflated")
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "private-name") {
		t.Fatal("export leaked local path")
	}
	if !strings.Contains(string(b), "仲裁边界") {
		t.Fatal("firewall warning lost")
	}
}

func TestMissingNetworkEvidenceNeverBecomesAnIPv6OrTunnelClaim(t *testing.T) {
	r := Build(Input{Network: Network{Available: false, Status: "connected", Mode: "global", NoDirect: true, Guard: "on", IPv6: false}})
	for _, f := range r.Findings {
		if f.ID == "network-path" && (f.Status != "unknown" || strings.Contains(f.Summary, "状态：connected")) {
			t.Fatal("stale unreachable service state was used", f)
		}
		if f.ID == "network-ipv6" && (f.Status != "unknown" || strings.Contains(f.Summary, "限制 IPv6")) {
			t.Fatal("missing snapshot became disabled IPv6 evidence", f)
		}
	}
}

func TestReachableServiceIsNotConnectedTunnelOrInstalledGuard(t *testing.T) {
	for _, n := range []Network{
		{Available: true, Status: "disconnected", Mode: "global", NoDirect: true, Guard: "on"},
		{Available: true, Status: "connected", Mode: "global", NoDirect: true, Guard: ""},
	} {
		r := Build(Input{Network: n})
		for _, f := range r.Findings {
			if f.ID == "network-path" && f.Status != "possible" {
				t.Fatal("missing connection/guard evidence hidden", f)
			}
		}
	}
	r := Build(Input{Network: Network{Available: true, Status: "connected", Mode: "global", NoDirect: true, Guard: "on"}})
	for _, f := range r.Findings {
		if f.ID == "network-path" && f.Status != "unknown" {
			t.Fatal("even a connected snapshot is not end-to-end proof", f)
		}
	}
}

func TestEmptyFirewallResultIsNotClaimedAsZeroExistingRules(t *testing.T) {
	for _, supported := range []bool{false, true} {
		r := Build(Input{Firewall: appfirewall.Report{Supported: supported, Warnings: []string{"本次查询失败"}}})
		for _, f := range r.Findings {
			if f.ID == "network-firewall" && (f.Status != "unknown" || strings.Contains(f.Summary, "检测到 0")) {
				t.Fatal("absence of evidence became zero rules", f)
			}
		}
	}
}

func TestExportRedactsPathsAcrossAllEvidenceAndOmitsCustomRuleIDs(t *testing.T) {
	r := Build(Input{
		Facts:    Facts{Warnings: []string{`read C:\Users\private-facts\facts.json failed`}},
		Policies: privacy.Report{Warnings: []string{`read C:/Users/private-forward/backup.json failed`}, Controls: []privacy.ControlState{{ID: "known-policy", Category: "windows", Detail: `failure \\private-server\private-share\secret.log`, Scope: `selected /home/private-unix/profile`}}},
		Firewall: appfirewall.Report{Rules: []appfirewall.RuleStatus{{Rule: appfirewall.Rule{ID: "private-custom-name", Program: `C:\private-program\browser.exe`}, Detail: `read file:///C:/private-fileurl/fault.log failed`}}},
	})
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-facts", "private-forward", "private-server", "private-share", "private-unix", "private-custom-name", "private-program", "private-fileurl"} {
		if strings.Contains(string(b), private) {
			t.Fatal("export leaked selected identifier/path", private, string(b))
		}
	}
	for _, f := range r.Findings {
		if strings.HasPrefix(f.ID, "firewall-") && f.ID != "firewall-rule-1" {
			t.Fatal("rule identifier was not report-local", f.ID)
		}
	}
}

func TestPathRedactionPreservesOfficialHTTPLinks(t *testing.T) {
	url := "https://learn.microsoft.com/en-us/windows/privacy"
	if got := redactLocalPaths("参考 " + url); got != "参考 "+url {
		t.Fatal("HTTP URL mistaken for drive path", got)
	}
}

func TestGuidanceNeverBecomesAnExecutablePolicyFinding(t *testing.T) {
	r := Build(Input{Policies: privacy.Report{Controls: []privacy.ControlState{
		{ID: "hardware-identifiers", Category: "guidance", Status: "unsupported", Scope: "native apps"},
		{ID: "fingerprint", Category: "guidance", Status: "manual", Scope: "controlled browser"},
		{ID: "chrome-location", Category: "browser", Status: "configured"},
		{ID: "windows-advertising-id", Category: "windows", Status: "manual"},
	}}})
	seen := map[string]bool{}
	for _, f := range r.Findings {
		seen[f.ID] = true
		if f.ControlID == "hardware-identifiers" || f.ControlID == "fingerprint" {
			t.Fatal("guidance gained executable action", f)
		}
	}
	if seen["policy-hardware-identifiers"] || seen["policy-fingerprint"] || !seen["policy-chrome-location"] || !seen["policy-windows-advertising-id"] {
		t.Fatal("policy/guidance boundary incorrect", seen)
	}
}

func TestReadOnlyPolicyMatchIsEvidenceWithoutWriteOrRuntimeClaim(t *testing.T) {
	r := Build(Input{Policies: privacy.Report{Controls: []privacy.ControlState{{ID: "windows-advertising-id", Category: "windows", Status: "manual", ReadStatus: "match", CanApply: false}}}})
	for _, f := range r.Findings {
		if f.ID == "policy-windows-advertising-id" && (f.Status != "unknown" || !strings.Contains(f.Summary, "只读取得")) {
			t.Fatal("read-only policy evidence misrepresented", f)
		}
	}
}
