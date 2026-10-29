package detection

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func ownedFinding(t *testing.T, in OwnedBrowser) Finding {
	t.Helper()
	r := Report{Findings: []Finding{{ID: "browser-gpu", Status: "unknown", Summary: "external browser remains unknown"}}}
	AppendOwnedBrowser(&r, in)
	if len(r.Findings) != 2 || r.Findings[0].Status != "unknown" || r.Findings[0].Summary != "external browser remains unknown" {
		t.Fatal("external browser evidence changed", r)
	}
	return r.Findings[1]
}

func TestOwnedBrowserNeverTurnsControllerStateIntoProtection(t *testing.T) {
	stamp := time.Date(2026, 10, 27, 1, 2, 3, 0, time.FixedZone("fixture", 8*3600))
	for _, state := range []string{"starting", "cancelling", "start_failed", "start_cancelled", "closed", "closing", "closing_or_unknown", "closed_filters_unknown", "ready", "UNKNOWN_SECRET_STATE"} {
		t.Run(state, func(t *testing.T) {
			in := OwnedBrowser{Tracked: true, Engine: "firefox", State: state, ReadAt: stamp}
			in.Ready, in.CanNavigate = state == "ready", state == "ready"
			f := ownedFinding(t, in)
			if f.Status == "restricted" || f.Source != "browser-session" || f.DetectedAt != stamp.UTC().Format(time.RFC3339Nano) || f.SettingsID != "browser" {
				t.Fatal(f)
			}
			unknown := state == "closing" || state == "closing_or_unknown" || state == "closed_filters_unknown" || state == "UNKNOWN_SECRET_STATE"
			if (f.Status == "unknown") != unknown || !strings.Contains(f.Scope, "不是当前网站") || strings.Contains(f.Summary+f.Evidence, "UNKNOWN_SECRET_STATE") {
				t.Fatal("unknown state relabeled or raw data included", f)
			}
		})
	}
	for _, in := range []OwnedBrowser{{Tracked: true, Engine: "firefox", State: "ready", Ready: true}, {Tracked: true, State: "closed", CanNavigate: true}, {Tracked: true, Engine: "SECRET_ENGINE", State: "ready", Ready: true, CanNavigate: true}} {
		if f := ownedFinding(t, in); f.Status != "unknown" || strings.Contains(f.Summary+f.Evidence, "SECRET_ENGINE") {
			t.Fatal("inconsistent state trusted", f)
		}
	}
	if f := ownedFinding(t, OwnedBrowser{}); f.DetectedAt != "" || strings.Contains(f.Summary, "已关闭") || !strings.Contains(f.Scope, "遗留") {
		t.Fatal("no tracked owner became global cleanup proof", f)
	}
}

func TestOwnedBrowserGraphicsRecordIsBoundedHistoricalEvidence(t *testing.T) {
	base := OwnedBrowser{Tracked: true, Engine: "firefox", State: "ready", Ready: true, CanNavigate: true, NativeGraphicsRecord: true, GraphicsWindowScopes: 4, GraphicsContexts: 16}
	for _, absent := range []int{0, 2, 4} {
		in := base
		in.OffscreenAbsentScopes, in.GraphicsContexts = absent, 16-2*absent
		f := ownedFinding(t, in)
		if !strings.Contains(f.Evidence, "本次启动保留") || !strings.Contains(f.Evidence, "未重测当前站点") || !strings.Contains(f.Evidence, "未验证 Worker API") {
			t.Fatal("history was presented as a new runtime measurement", f)
		}
	}
	for _, change := range []func(*OwnedBrowser){
		func(v *OwnedBrowser) { v.State, v.Ready, v.CanNavigate = "closed_filters_unknown", false, false },
		func(v *OwnedBrowser) { v.GraphicsWindowScopes = 3 },
		func(v *OwnedBrowser) { v.GraphicsContexts = 1 },
		func(v *OwnedBrowser) { v.OffscreenAbsentScopes = 5 },
		func(v *OwnedBrowser) { v.Engine = "chrome-for-testing" },
		func(v *OwnedBrowser) { v.NativeGraphicsRecord = false },
	} {
		in := base
		change(&in)
		if f := ownedFinding(t, in); strings.Contains(f.Evidence, "本次启动保留") {
			t.Fatal("malformed or stale graphics record published", f)
		}
	}
	f := ownedFinding(t, OwnedBrowser{Tracked: true, State: "SECRET_RAW_ERROR_OR_PATH"})
	encoded, err := json.Marshal(f)
	if err != nil || strings.Contains(string(encoded), "SECRET_RAW_ERROR_OR_PATH") {
		t.Fatal("unbounded controller value serialized", string(encoded), err)
	}
}
