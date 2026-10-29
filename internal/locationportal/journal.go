package locationportal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

const journalName = "location-portal.v1.json"
const maxJournal = 16384

type userIdentity struct{ sid, directory string }
type store interface {
	read() ([]byte, error)
	save([]byte) error
	close()
}
type storeMode uint8

const (
	storeReadOnly storeMode = iota
	storeUpdate
	storeCreate
)

type openStore func(userIdentity, storeMode) (store, error)

// A backup never contains a password, username or CSRF token. It only owns an
// override that started disabled. There is no undocumented cache-delete call.
type transaction struct {
	Owner       string    `json:"ownerSid"`
	Port        int       `json:"port"`
	Certificate string    `json:"certificateSHA256"`
	Stage       string    `json:"stage"`
	Before      *Position `json:"before"`
	Expected    *Position `json:"expected,omitempty"`
	Pending     *Position `json:"pending,omitempty"`
	WrittenAt   string    `json:"writtenAt,omitempty"`
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
	if j == nil || j.Owner != owner || j.Port < 1024 || j.Port > 65535 || len(j.Certificate) != 64 || j.Before == nil || !validPosition(*j.Before) {
		return false
	}
	if b, e := hex.DecodeString(j.Certificate); e != nil || len(b) != 32 || strings.ToLower(j.Certificate) != j.Certificate {
		return false
	}
	if j.Expected != nil && !validPosition(*j.Expected) || j.Pending != nil && !validPosition(*j.Pending) {
		return false
	}
	if j.WrittenAt != "" {
		if _, e := time.Parse(time.RFC3339Nano, j.WrittenAt); e != nil {
			return false
		}
	}
	switch j.Stage {
	case "applying":
		return j.Pending != nil
	case "active":
		return j.Expected != nil && j.Pending == nil && j.WrittenAt != ""
	case "restoring":
		return j.Pending == nil
	case "restored":
		return j.Pending == nil && j.Expected == nil
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
	if strict(data, &outer) != nil {
		return nil, ErrStore
	}
	digest := sha256.Sum256(outer.Payload)
	if outer.SHA256 != hex.EncodeToString(digest[:]) {
		return nil, ErrStore
	}
	var payload journalPayload
	if strict(outer.Payload, &payload) != nil || payload.Version != 1 || !validTransaction(payload.Transaction, owner) {
		return nil, ErrStore
	}
	return payload.Transaction, nil
}
func save(s store, j *transaction) error {
	if j == nil || !validTransaction(j, j.Owner) {
		return ErrStore
	}
	p, e := json.Marshal(journalPayload{1, j})
	if e != nil {
		return ErrStore
	}
	h := sha256.Sum256(p)
	data, e := json.Marshal(envelope{p, hex.EncodeToString(h[:])})
	if e != nil || len(data) > maxJournal {
		return ErrStore
	}
	return s.save(data)
}

// Every object key has exact casing and is unique before Go's case-insensitive
// struct decoder runs. The accepted vocabulary contains no arbitrary URI/args.
func strict(data []byte, value any) error {
	if len(data) == 0 || len(data) > maxJournal || !utf8.Valid(data) {
		return ErrInput
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if unique(d, 0) != nil {
		return ErrInput
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInput
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(new(any)) != io.EOF {
		return ErrInput
	}
	return nil
}
func unique(d *json.Decoder, depth int) error {
	if depth > 7 {
		return ErrInput
	}
	token, e := d.Token()
	if e != nil {
		return ErrInput
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			t, e := d.Token()
			key, ok := t.(string)
			if e != nil || !ok || seen[strings.ToLower(key)] {
				return ErrInput
			}
			switch key {
			case "payload", "sha256", "version", "transaction", "ownerSid", "port", "certificateSHA256", "stage", "before", "expected", "pending", "writtenAt", "latitude", "longitude", "username", "password", "points", "intervalSeconds", "timeoutSeconds", "schemaVersion", "requestedAfter", "status", "position", "timestamp", "accuracyMetres", "positionSource":
			default:
				return ErrInput
			}
			seen[strings.ToLower(key)] = true
			if unique(d, depth+1) != nil {
				return ErrInput
			}
		}
	case '[':
		for d.More() {
			if unique(d, depth+1) != nil {
				return ErrInput
			}
		}
	default:
		return ErrInput
	}
	end, e := d.Token()
	if e != nil || delimiter == '{' && end != json.Delim('}') || delimiter == '[' && end != json.Delim(']') {
		return ErrInput
	}
	return nil
}
func ParseConfig(raw string) (Config, error) {
	var c Config
	if strict([]byte(raw), &c) != nil || !validConfig(c) {
		return Config{}, ErrInput
	}
	return c, nil
}
