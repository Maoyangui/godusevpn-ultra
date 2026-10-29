package detection

import (
	"encoding/json"
	"strings"
	"testing"
)

// Supplied locale names only: never read LocaleName/default locale/user data.
func TestKnownWindowsLocaleNamesAreCanonicalLocalEvidence(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"zh-CN", "zh-CN"}, {"en-us", "en-US"}, {"de-DE", "de-DE"}} {
		if got := normalizedLocale(tc.input); got != tc.want {
			t.Fatalf("supplied name %q returned %q", tc.input, got)
		}
		r := Build(Input{Facts: Facts{Locale: tc.input}})
		found := false
		for _, f := range r.Findings {
			if f.ID == "locale" {
				found = true
				if f.Status != "observed" || !strings.Contains(f.Summary, tc.want) || !strings.Contains(f.Scope, "浏览器语言") {
					t.Fatalf("valid configured format or scope lost: %+v", f)
				}
			}
		}
		if !found {
			t.Fatal("valid locale disappeared")
		}
		if _, err := json.Marshal(r); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []string{"AUDIT_INVALID_LOCALE_SENTINEL", "zzzz-invalid-test", "en-INVALID_UNREGISTERED"} {
		if got := normalizedLocale(invalid); got != "" {
			t.Fatalf("unregistered/invalid supplied name accepted: %q=>%q", invalid, got)
		}
	}
}
