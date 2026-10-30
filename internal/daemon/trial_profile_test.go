package daemon

import (
	"testing"

	"github.com/Maoyangui/godusevpn/internal/settings"
)

func TestTrialProfileDetectionKeepsLegacyRowsReplaceable(t *testing.T) {
	cases := []struct {
		name string
		kind string
		want bool
	}{
		{"免费试用", "", true},
		{"Free trial", "", true},
		{"anything", trialProfileKind, true},
		{"normal", "", false},
	}
	for _, tc := range cases {
		if got := isTrialProfile(settings.Profile{Name: tc.name, Kind: tc.kind}); got != tc.want {
			t.Fatalf("isTrialProfile(%q,%q)=%v, want %v", tc.name, tc.kind, got, tc.want)
		}
	}
}

func TestTrialProfileIndexesFindsDuplicatesWithoutTouchingNormalRows(t *testing.T) {
	profiles := []settings.Profile{
		{ID: "normal", Name: "我的订阅"},
		{ID: "old", Name: "免费试用"},
		{ID: "new", Kind: trialProfileKind},
	}
	got := trialProfileIndexes(profiles)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("trial indexes=%v", got)
	}
}

func TestUpsertTrialProfileReplacesURLAndRemovesOnlyTrialDuplicates(t *testing.T) {
	s := settings.Default()
	s.Profiles = []settings.Profile{
		{ID: "normal", Name: "我的订阅", URL: "https://normal.example/sub"},
		{ID: "trial-old", Name: "免费试用", URL: "https://old.example/sub"},
		{ID: "trial-new", Kind: trialProfileKind, Name: "免费试用", URL: "https://new.example/sub"},
	}
	s.ActiveProfile = "trial-new"

	got, primary, removed := upsertTrialProfileSettings(s, "免费试用", "https://rotated.example/sub")
	if primary != "trial-new" {
		t.Fatalf("primary=%q, want active trial row", primary)
	}
	if len(removed) != 1 || removed[0] != "trial-old" {
		t.Fatalf("removed=%v, want only stale trial row", removed)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("profiles=%d, want normal + one trial", len(got.Profiles))
	}
	if got.Profiles[1].ID != "trial-new" || got.Profiles[1].URL != "https://rotated.example/sub" || got.Profiles[1].Kind != trialProfileKind {
		t.Fatalf("updated trial row=%+v", got.Profiles[1])
	}
	if got.Profiles[0].URL != "https://normal.example/sub" {
		t.Fatalf("normal subscription changed: %+v", got.Profiles[0])
	}
}

func TestUpsertTrialProfileAddsWithoutChangingExistingActiveProfile(t *testing.T) {
	s := settings.Default()
	s.Profiles = []settings.Profile{{ID: "normal", Name: "我的订阅", URL: "https://normal.example/sub"}}
	s.ActiveProfile = "normal"

	got, primary, removed := upsertTrialProfileSettings(s, "", "https://trial.example/sub")
	if primary == "" || len(removed) != 0 || got.ActiveProfile != "normal" {
		t.Fatalf("primary=%q removed=%v active=%q", primary, removed, got.ActiveProfile)
	}
	if len(got.Profiles) != 2 || got.Profiles[1].Kind != trialProfileKind || got.Profiles[1].Name != "免费试用" {
		t.Fatalf("profiles=%+v", got.Profiles)
	}
}
