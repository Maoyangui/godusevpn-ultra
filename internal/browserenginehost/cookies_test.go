package browserenginehost

import (
	"encoding/json"
	"testing"
)

func TestValidateCookieArchive(t *testing.T) {
	raw := json.RawMessage(`{"cookies":[{"name":"sid","value":"abc","domain":"example.com","path":"/","secure":true}]}`)
	items, err := validateCookieArchive(raw)
	if err != nil || len(items) != 1 {
		t.Fatalf("valid archive rejected: %v (%d)", err, len(items))
	}
	for _, bad := range []json.RawMessage{
		json.RawMessage(`{"cookies":[{"name":"","value":"x"}]}`),
		json.RawMessage(`{"cookies":[{"name":"sid"}]}`),
		json.RawMessage(`{"cookies":"not-an-array"}`),
	} {
		if _, err := validateCookieArchive(bad); err == nil {
			t.Fatalf("invalid archive accepted: %s", bad)
		}
	}
}
