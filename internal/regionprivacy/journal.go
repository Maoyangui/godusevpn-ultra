package regionprivacy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const journalName = "region-journal.v1.json"
const maxJournal = 16384

type transaction struct {
	Owner    string `json:"ownerSid"`
	Before   string `json:"before"`
	Expected string `json:"expected"`
	Pending  string `json:"pending,omitempty"`
	Stage    string `json:"stage"`
}

func (j *transaction) accepts(code string) bool {
	return code == j.Expected || j.Pending != "" && code == j.Pending
}

type journalPayload struct {
	Version     int          `json:"version"`
	Transaction *transaction `json:"transaction"`
}
type envelope struct {
	Payload json.RawMessage `json:"payload"`
	SHA256  string          `json:"sha256"`
}

func validTransaction(j *transaction, owner string) bool {
	if j == nil || j.Owner != owner || !codePattern.MatchString(j.Before) || !codePattern.MatchString(j.Expected) {
		return false
	}
	switch j.Stage {
	case "active", "restored":
		return j.Pending == "" && (j.Stage != "restored" || j.Expected == j.Before)
	case "applying":
		return codePattern.MatchString(j.Pending)
	case "restoring":
		return j.Pending == j.Before
	}
	return false
}
func load(s store, owner string) (*transaction, error) {
	data, err := s.read()
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil || len(data) == 0 || len(data) > maxJournal {
		return nil, ErrStore
	}
	var outer envelope
	if strictJournal(data, &outer) != nil {
		return nil, ErrStore
	}
	digest := sha256.Sum256(outer.Payload)
	if outer.SHA256 != hex.EncodeToString(digest[:]) {
		return nil, ErrStore
	}
	var payload journalPayload
	if strictJournal(outer.Payload, &payload) != nil || payload.Version != 1 || !validTransaction(payload.Transaction, owner) {
		return nil, ErrStore
	}
	return payload.Transaction, nil
}
func save(s store, j *transaction) error {
	if j == nil || !validTransaction(j, j.Owner) {
		return ErrStore
	}
	payload, err := json.Marshal(journalPayload{Version: 1, Transaction: j})
	if err != nil {
		return ErrStore
	}
	digest := sha256.Sum256(payload)
	data, err := json.Marshal(envelope{Payload: payload, SHA256: hex.EncodeToString(digest[:])})
	if err != nil || len(data) > maxJournal {
		return ErrStore
	}
	return s.save(data)
}
func strictJournal(data []byte, value any) error {
	if !utf8.Valid(data) {
		return ErrStore
	}
	keys := json.NewDecoder(bytes.NewReader(data))
	if uniqueJournalKeys(keys, 0) != nil {
		return ErrStore
	}
	if _, err := keys.Token(); err != io.EOF {
		return ErrStore
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return ErrStore
	}
	return nil
}
func uniqueJournalKeys(decoder *json.Decoder, depth int) error {
	if depth > 5 {
		return ErrStore
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[strings.ToLower(name)] {
					return ErrStore
				}
				switch name {
				case "payload", "sha256", "version", "transaction", "ownerSid", "before", "expected", "pending", "stage":
				default:
					return ErrStore
				}
				seen[strings.ToLower(name)] = true
				if uniqueJournalKeys(decoder, depth+1) != nil {
					return ErrStore
				}
			}
		case '[':
			for decoder.More() {
				if uniqueJournalKeys(decoder, depth+1) != nil {
					return ErrStore
				}
			}
		default:
			return ErrStore
		}
		end, err := decoder.Token()
		if err != nil || delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
			return ErrStore
		}
	}
	return nil
}
