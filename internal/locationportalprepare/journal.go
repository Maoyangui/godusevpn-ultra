package locationportalprepare

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strings"
)

const maxJournal = 65536

type envelope struct {
	Payload json.RawMessage `json:"payload"`
	SHA256  string          `json:"sha256"`
}

func validService(s serviceState) bool {
	return !s.Present && s.StartType == 0 && s.State == 0 || s.Present && s.StartType <= 4 && s.State >= 1 && s.State <= 7
}
func validState(s systemState) bool {
	if s.Capability > 7 || !validService(s.Service) || !validService(s.SSHBroker) || !validService(s.SSHProxy) {
		return false
	}
	for _, v := range []registryValue{s.DeveloperMode, s.DynamicPorts, s.HTTPPort, s.HTTPSPort, s.Authorizer} {
		if !v.Present && v.Value != 0 {
			return false
		}
	}
	if !s.PortalKeyPresent && s.PortalConfigSHA256 != "" {
		return false
	}
	return s.PortalConfigSHA256 == "" || validPin(s.PortalConfigSHA256)
}
func validJournal(j *journal) bool {
	if j == nil || j.Version != 1 || !validOwnerSID(j.Owner) || !validPin(j.TransactionID) || !cleanInitial(j.Before) || !validState(j.Before) || !validState(j.Expected) || len(j.CredentialBox) > 16384 {
		return false
	}
	if j.CertificateSHA256 != "" && !validPin(j.CertificateSHA256) {
		return false
	}
	if j.Stage == "ready" && (!fixedConfigured(j.Expected) || j.Expected.Service.State != 4 || !j.PortalWasStarted || !validPin(j.CertificateSHA256) || len(j.CredentialBox) == 0 || j.Pending != "") {
		return false
	}
	if j.Stage == "restored" && (!restoredState(j.Before, j.Expected) || j.Pending != "" || len(j.CredentialBox) != 0 || j.CertificateSHA256 != "") {
		return false
	}
	stages := map[string]bool{"backed_up": true, "gate_verified": true, "capability": true, "ssh_components": true, "developer_mode": true, "service_stop": true, "configuration": true, "service_start": true, "ready": true, "restore_stop": true, "restore_capability": true, "restore_settings": true, "restored": true}
	pending := map[string]bool{"": true, "gate": true, "capability": true, "ssh_components": true, "developer_mode": true, "service_stop": true, "configuration": true, "service_start": true, "restore_stop": true, "restore_capability": true, "restore_settings": true}
	return stages[j.Stage] && pending[j.Pending]
}
func validOwnerSID(s string) bool {
	if len(s) < 7 || len(s) > 256 || !strings.HasPrefix(s, "S-1-") {
		return false
	}
	for _, c := range s[4:] {
		if c != '-' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
func encodeJournal(j *journal) ([]byte, error) {
	if !validJournal(j) {
		return nil, ErrStore
	}
	p, e := json.Marshal(j)
	if e != nil {
		return nil, ErrStore
	}
	sum := sha256.Sum256(p)
	out, e := json.Marshal(envelope{p, hex.EncodeToString(sum[:])})
	if e != nil || len(out) > maxJournal {
		return nil, ErrStore
	}
	return out, nil
}
func decodeJournal(raw []byte) (*journal, error) {
	var e envelope
	if exactJSON(raw, &e) != nil || !validPin(e.SHA256) {
		return nil, ErrStore
	}
	sum := sha256.Sum256(e.Payload)
	if hex.EncodeToString(sum[:]) != e.SHA256 {
		return nil, ErrStore
	}
	var j journal
	if exactJSON(e.Payload, &j) != nil || !validJournal(&j) {
		return nil, ErrStore
	}
	return &j, nil
}
func exactJSON(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > maxJournal {
		return ErrStore
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if uniqueObject(d, 0) != nil {
		return ErrStore
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrStore
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return ErrStore
	}
	// Go accepts case aliases. Compare normalized JSON values with the exact
	// schema emitted by the typed value; duplicates were rejected separately.
	canonical, e := json.Marshal(out)
	if e != nil {
		return ErrStore
	}
	var input, typed any
	if json.Unmarshal(raw, &input) != nil || json.Unmarshal(canonical, &typed) != nil || !reflect.DeepEqual(input, typed) {
		return ErrStore
	}
	return nil
}
func uniqueObject(d *json.Decoder, depth int) error {
	if depth > 8 {
		return ErrStore
	}
	t, e := d.Token()
	if e != nil {
		return ErrStore
	}
	start, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if start == '{' {
		seen := map[string]bool{}
		for d.More() {
			t, e := d.Token()
			key, ok := t.(string)
			if e != nil || !ok || seen[strings.ToLower(key)] {
				return ErrStore
			}
			seen[strings.ToLower(key)] = true
			if uniqueObject(d, depth+1) != nil {
				return ErrStore
			}
		}
	} else if start == '[' {
		for d.More() {
			if uniqueObject(d, depth+1) != nil {
				return ErrStore
			}
		}
	} else {
		return ErrStore
	}
	end, e := d.Token()
	if e != nil || end != json.Delim(map[json.Delim]json.Delim{'{': '}', '[': ']'}[start]) {
		return ErrStore
	}
	return nil
}
