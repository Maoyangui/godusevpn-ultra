package browserenginehost

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

const maxCookieArchiveBytes = 2 << 20
const maxCookieArchiveItems = 10000

var errCookieArchive = errors.New("browser_cookie_archive_refused")

// Cookie archives are deliberately kept as the browser protocol's envelope:
// {"cookies":[{...}]}. Unknown cookie attributes are passed through to the
// native engine, while malformed or oversized archives are rejected before
// they can reach it.
func validateCookieArchive(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxCookieArchiveBytes || !json.Valid(raw) {
		return nil, errCookieArchive
	}
	var envelope struct {
		Cookies []json.RawMessage `json:"cookies"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&envelope) != nil || len(envelope.Cookies) > maxCookieArchiveItems {
		return nil, errCookieArchive
	}
	for _, item := range envelope.Cookies {
		if len(item) == 0 || len(item) > 32<<10 || !json.Valid(item) {
			return nil, errCookieArchive
		}
		var value map[string]any
		if json.Unmarshal(item, &value) != nil || value["name"] == nil || value["value"] == nil {
			return nil, errCookieArchive
		}
		name, nameOK := value["name"].(string)
		cookieValue, valueOK := value["value"].(string)
		if !nameOK || !valueOK || name == "" || len(name) > 4096 || len(cookieValue) > 64<<10 || strings.ContainsAny(name+cookieValue, "\x00\r\n") {
			return nil, errCookieArchive
		}
	}
	return envelope.Cookies, nil
}
