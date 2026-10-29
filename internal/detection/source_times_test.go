package detection

import "testing"

func TestSourceTimesDoNotMakeCachedOrManualEvidenceFresh(t *testing.T) {
	r := Report{GeneratedAt: "report-generation", Findings: []Finding{{ID: "policy-fixture", DetectedAt: "old-generated"}, {ID: "network-path", DetectedAt: "old-generated"}, {ID: "network-firewall-unknown"}, {ID: "locale"}, {ID: "browser-gpu"}, {ID: "selected-files"}}}
	bindSourceTimes(&r, map[string]string{"policies": "2026-01-01T01:00:00Z", "firewall": "2026-01-01T01:00:01Z", "facts": "not-a-timestamp"})
	if r.Findings[0].DetectedAt != "2026-01-01T01:00:00Z" || r.Findings[2].DetectedAt != "2026-01-01T01:00:01Z" {
		t.Fatal("independent source time lost")
	}
	for _, i := range []int{1, 3, 4, 5} {
		if r.Findings[i].DetectedAt != "" {
			t.Fatal("missing/invalid/static source was presented as fresh", r.Findings[i])
		}
	}
	if r.GeneratedAt != "report-generation" {
		t.Fatal("source binding changed report generation time")
	}
	r = Report{Findings: []Finding{{ID: "firewall-rule-1"}, {ID: "network-dns"}, {ID: "local-logs"}}}
	bindSourceTimes(&r, map[string]string{"firewall": "2026-01-01T01:00:01Z", "network": "2026-01-01T01:00:02Z"})
	for i, source := range []string{"firewall", "network", "network"} {
		if r.Findings[i].Source != source || r.Findings[i].DetectedAt == "" {
			t.Fatal("live source provenance omitted", r.Findings[i])
		}
	}
}
