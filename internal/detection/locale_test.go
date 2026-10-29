package detection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLocaleBoundaryRejectsUnsafeInputBeforeVerifierAndInvalidOutput(t *testing.T) {
	for _, raw := range []string{"", "x", " en-US", "en-US ", "en-US\x00fixture", "en-US\nfixture", "fixture/secret", "中文", strings.Repeat("x", 85)} {
		calls := 0
		got := checkedLocale(raw, func(string) string { calls++; return "en-US" })
		if got != "" || calls != 0 {
			t.Fatalf("unsafe input reached verifier: %q", raw)
		}
	}
	if got := checkedLocale("en-us", func(string) string { return "en-US" }); got != "en-US" {
		t.Fatal("canonical name lost")
	}
	for _, answer := range []string{"", "en-US\nfixture", strings.Repeat("x", 85)} {
		if got := checkedLocale("en-US", func(string) string { return answer }); got != "" {
			t.Fatal("invalid verifier result accepted")
		}
	}
}

func TestMissingOrInvalidLocaleRemainsUnknownAndNeverExportsRawValue(t *testing.T) {
	for _, raw := range []string{"", "AUDIT_INVALID_LOCALE_SENTINEL", "zzzz-invalid-test", "en-US\nSYNTHETIC_EXPORT_SENTINEL", `C:\fixture-private\invalid-locale`} {
		r := Build(Input{Facts: Facts{Locale: raw}})
		found := false
		for _, f := range r.Findings {
			if f.ID == "locale" {
				found = true
				if f.Status != "unknown" || f.SettingsID != "language" {
					t.Fatalf("invalid locale became observed or lost guidance: %+v", f)
				}
			}
		}
		if !found {
			t.Fatal("missing locale finding disappeared")
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "" && strings.Contains(string(encoded), raw) {
			t.Fatalf("export retained invalid locale %q", raw)
		}
		for _, sentinel := range []string{"AUDIT_INVALID_LOCALE_SENTINEL", "SYNTHETIC_EXPORT_SENTINEL", "fixture-private"} {
			if strings.Contains(string(encoded), sentinel) {
				t.Fatal("export retained invalid input")
			}
		}
	}
}
