package regionprivacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func envelopeFor(payload string) []byte {
	digest := sha256.Sum256([]byte(payload))
	data, _ := json.Marshal(envelope{Payload: json.RawMessage(payload), SHA256: hex.EncodeToString(digest[:])})
	return data
}
func TestJournalRejectsCorruptionForeignOwnerAndAmbiguousJSON(t *testing.T) {
	valid := `{"version":1,"transaction":{"ownerSid":"` + fakeSID + `","before":"US","expected":"DE","stage":"active"}}`
	cases := map[string][]byte{
		"empty": {}, "large": bytes.Repeat([]byte("x"), maxJournal+1), "not-json": []byte("x"),
		"bad-sha":           []byte(`{"payload":` + valid + `,"sha256":"` + strings.Repeat("0", 64) + `"}`),
		"foreign":           envelopeFor(strings.ReplaceAll(valid, fakeSID, "S-1-5-21-OTHER")),
		"unknown":           envelopeFor(strings.Replace(valid, `"stage":"active"`, `"stage":"active","extra":true`, 1)),
		"duplicate":         envelopeFor(strings.Replace(valid, `"before":"US"`, `"before":"US","before":"JP"`, 1)),
		"case-duplicate":    envelopeFor(strings.Replace(valid, `"before":"US"`, `"before":"US","BEFORE":"JP"`, 1)),
		"single-case-alias": envelopeFor(strings.Replace(valid, `"before":"US"`, `"BEFORE":"US"`, 1)),
		"version":           envelopeFor(strings.Replace(valid, `"version":1`, `"version":2`, 1)),
		"stage":             envelopeFor(strings.Replace(valid, `"active"`, `"unknown"`, 1)),
		"active-pending":    envelopeFor(strings.Replace(valid, `"stage":"active"`, `"stage":"active","pending":"JP"`, 1)),
		"trailing":          append(envelopeFor(valid), []byte(" {}")...),
		"restore-mismatch":  envelopeFor(strings.Replace(valid, `"stage":"active"`, `"stage":"restored"`, 1)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			s := &memoryStore{data: data}
			if j, err := load(s, fakeSID); !errors.Is(err, ErrStore) || j != nil {
				t.Fatal("invalid recovery state accepted", j, err)
			}
		})
	}
}
func TestJournalSaveRejectsInvalidStateWithoutWriting(t *testing.T) {
	for _, j := range []*transaction{nil, {Owner: fakeSID, Before: "US", Expected: "DE", Stage: "restored"}, {Owner: fakeSID, Before: "US", Expected: "DE", Pending: "JP", Stage: "restoring"}, {Owner: fakeSID, Before: "US", Expected: "DE", Stage: "applying"}} {
		s := &memoryStore{}
		if err := save(s, j); !errors.Is(err, ErrStore) || s.writes != 0 {
			t.Fatal("invalid state saved", err)
		}
	}
	s := &memoryStore{hook: func(int, []byte) error { return ErrConflict }}
	if err := save(s, &transaction{Owner: fakeSID, Before: "US", Expected: "DE", Stage: "active"}); !errors.Is(err, ErrConflict) {
		t.Fatal("lost conditional-store conflict", err)
	}
}
func TestRecoveryRestoringStageCanCompleteWithoutSecondSetter(t *testing.T) {
	m, b, s := fixture()
	if err := save(s, &transaction{Owner: fakeSID, Before: "US", Expected: "DE", Pending: "US", Stage: "restoring"}); err != nil {
		t.Fatal(err)
	}
	r, err := m.Restore()
	if err != nil || !r.Verified || r.Changed || len(b.sets) != 0 || journalOf(t, s).Stage != "restored" {
		t.Fatal(r, err)
	}
}
