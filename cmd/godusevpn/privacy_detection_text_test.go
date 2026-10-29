package main

import (
	"strings"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/detection"
)

func reviewedTextApp(t *testing.T) (*App, PrivacyExportPreview) {
	t.Helper()
	app := &App{}
	app.privacySnapshot = &privacyDetectionSnapshot{id: strings.Repeat("a", 48), expires: time.Now().Add(time.Minute), report: detection.Report{GeneratedAt: "2026-10-27T01:00:00Z", Observer: "OBSERVER_CANARY", Platform: "PLATFORM_CANARY", Findings: []detection.Finding{{ID: "network-path", Title: "所选网络证据", Status: "unknown", Summary: "没有流量证明", Scope: "仅所选来源"}, {ID: "timezone", Title: "时区", Status: "observed", Summary: "ENVIRONMENT_CANARY"}, {ID: "locale", Title: "地区", Status: "observed", Summary: "LOCALE_CANARY"}, {ID: "unselected", Title: "不选项", Status: "possible", Summary: "UNSELECTED_CANARY"}}}}
	preview, err := app.PrivacyPreviewDetectionExport(PrivacyExportSelection{SnapshotID: app.privacySnapshot.id, Scope: "filtered", FindingIDs: []string{"network-path", "timezone", "locale"}})
	if err != nil {
		t.Fatal(err)
	}
	return app, preview
}

func TestPrivacyDetectionTextCopyMatchesVisibleReviewedSelection(t *testing.T) {
	app, jsonPreview := reviewedTextApp(t)
	heldPreview := app.privacyExport
	text, err := app.PrivacyDetectionTextPreview(jsonPreview.PreviewID)
	if err != nil || text.Count != 1 || text.PreviewID != jsonPreview.PreviewID || text.ExpiresAt != jsonPreview.ExpiresAt {
		t.Fatal(text, err)
	}
	for _, canary := range []string{"OBSERVER_CANARY", "PLATFORM_CANARY", "ENVIRONMENT_CANARY", "LOCALE_CANARY", "UNSELECTED_CANARY"} {
		if strings.Contains(text.Content, canary) {
			t.Fatal("bypassed selected/default fields", canary)
		}
	}
	copy, err := app.PrivacyDetectionTextForCopy(text.PreviewID, text.SHA256)
	if err != nil || copy != text.Content || app.privacyExport != heldPreview || string(app.privacyExport.content) != jsonPreview.Content {
		t.Fatal("copy differs, consumed or replaced JSON preview", err)
	}
	if _, err := app.PrivacyDetectionTextForCopy(text.PreviewID, strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong visible digest accepted")
	}
}

func TestPrivacyDetectionTextRefusesExpiredInvalidatedReplacedOrChangedPreview(t *testing.T) {
	for _, kind := range []string{"privacy-epoch", "preview-expired", "snapshot-expired", "preview-replaced", "snapshot-replaced", "content-changed", "missing-snapshot", "wrong-id"} {
		t.Run(kind, func(t *testing.T) {
			app, jsonPreview := reviewedTextApp(t)
			text, err := app.PrivacyDetectionTextPreview(jsonPreview.PreviewID)
			if err != nil {
				t.Fatal(err)
			}
			id := text.PreviewID
			switch kind {
			case "privacy-epoch":
				app.invalidatePrivacyEvidence()
			case "preview-expired":
				app.privacyExport.expires = time.Now().Add(-time.Second)
			case "snapshot-expired":
				app.privacySnapshot.expires = time.Now().Add(-time.Second)
			case "preview-replaced":
				app.privacyExport.id = strings.Repeat("b", 48)
			case "snapshot-replaced":
				app.privacySnapshot.id = strings.Repeat("b", 48)
			case "content-changed":
				app.privacyExport.content = []byte(strings.ReplaceAll(string(app.privacyExport.content), "没有流量证明", "这是新的结果"))
			case "missing-snapshot":
				app.privacySnapshot = nil
			case "wrong-id":
				id = strings.Repeat("c", 48)
			}
			if copy, err := app.PrivacyDetectionTextForCopy(id, text.SHA256); err == nil || copy != "" {
				t.Fatal("stale/changed text was copyable", copy, err)
			}
		})
	}
}

func TestPrivacyDetectionTextNeverCopiesWithoutExactValidDigest(t *testing.T) {
	app, jsonPreview := reviewedTextApp(t)
	for _, digest := range []string{"", strings.Repeat("g", 64), strings.Repeat("A", 64), strings.Repeat("0", 63), strings.Repeat("0", 64) + "\x00"} {
		if result, err := app.PrivacyDetectionTextForCopy(jsonPreview.PreviewID, digest); err == nil || result != "" {
			t.Fatal("invalid digest accepted")
		}
	}
	for _, id := range []string{"", "../source", strings.Repeat("A", 48), strings.Repeat("a", 49), strings.Repeat("g", 48)} {
		if result, err := app.PrivacyDetectionTextPreview(id); err == nil || result.Content != "" {
			t.Fatal("invalid preview token accepted")
		}
	}
}
