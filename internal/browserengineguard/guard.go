// Package browserengineguard owns only browser-session WFP filters. It never
// changes the VPN guard or the optional persistent appfirewall policy rules.
package browserengineguard

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrRefused     = errors.New("browser_engine_guard_refused")
	ErrUnknown     = errors.New("browser_engine_guard_state_unknown")
	ErrActive      = errors.New("browser_engine_guard_job_still_active")
	ErrUnsupported = errors.New("browser_engine_guard_unsupported")
	sessionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	chromePattern  = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}$`)
	foxPattern     = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}(?:\.[0-9]{1,3})?(?:esr)?$`)
)

type InstallRequest struct {
	SessionID   string `json:"sessionId"`
	EngineID    string `json:"engineId"`
	Version     string `json:"version"`
	JobHandle   uint64 `json:"jobHandle"`
	MainPID     uint32 `json:"mainPid"`
	MainCreated uint64 `json:"mainCreated"`
	RelayPort   uint16 `json:"relayPort"`
	ControlPort uint16 `json:"controlPort"`
}
type Result struct {
	SessionID  string      `json:"sessionId"`
	Installed  bool        `json:"installed"`
	Verified   bool        `json:"verified"`
	State      string      `json:"state"`
	Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
}

func validate(r InstallRequest) error {
	if !sessionPattern.MatchString(r.SessionID) || r.JobHandle == 0 || r.MainPID == 0 || r.MainCreated == 0 || r.RelayPort == 0 || r.ControlPort == r.RelayPort {
		return ErrRefused
	}
	switch r.EngineID {
	case "chrome-for-testing":
		if !chromePattern.MatchString(r.Version) || r.ControlPort != 0 {
			return ErrRefused
		}
	case "firefox":
		if !foxPattern.MatchString(r.Version) || r.ControlPort == 0 {
			return ErrRefused
		}
	default:
		return ErrRefused
	}
	return nil
}

// Each specification is a BLOCK. Their union permits only the intended tuple;
// there is no broad terminating PERMIT capable of bypassing the VPN sublayer.
type filterSpec struct {
	layer  string
	field  string
	number uint32
}

func specs(r InstallRequest) []filterSpec {
	result := []filterSpec{{"out4", "remote-address-not", 0x7f000001}, {"out4", "remote-port-not", uint32(r.RelayPort)}, {"out4", "protocol-not", 6}, {"out6", "", 0}, {"in6", "", 0}}
	if r.ControlPort == 0 {
		return append(result, filterSpec{"in4", "", 0})
	}
	return append(result, filterSpec{"in4", "local-address-not", 0x7f000001}, filterSpec{"in4", "remote-address-not", 0x7f000001}, filterSpec{"in4", "local-port-not", uint32(r.ControlPort)}, filterSpec{"in4", "protocol-not", 6})
}

// Validate the Windows namespace lexically before asking any filesystem API to
// open it. A KnownFolder value is user-controlled configuration, not proof of a
// local path. The drive probe is called only after this pure check succeeds.
func verifyLocalDirectory(path string, probe func(string) error) error {
	if len(path) < 4 || len(path) > 30000 || !((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) || path[1] != ':' || path[2] != '\\' {
		return ErrRefused
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, `/:*?"<>|`) {
			return ErrRefused
		}
		for _, character := range part {
			if character < 32 || character == 127 {
				return ErrRefused
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return ErrRefused
		}
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			tail := base[3:]
			if len([]rune(tail)) == 1 && strings.Contains("123456789¹²³", tail) {
				return ErrRefused
			}
		}
	}
	if probe == nil {
		return ErrRefused
	}
	return probe(path[:3])
}
func fixedVolumeMapping(mapping string) bool {
	const prefix = `\Device\HarddiskVolume`
	if !strings.HasPrefix(strings.ToLower(mapping), strings.ToLower(prefix)) || len(mapping) <= len(prefix) {
		return false
	}
	for _, character := range mapping[len(prefix):] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
