package privacyhost

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

var errBrowserGuardRefused = errors.New("browser_guard_operation_refused")

func browserGuardMethod(method string) bool {
	return method == "BrowserGuardInstall" || method == "BrowserGuardReport" || method == "BrowserGuardRemove" || method == "BrowserGuardStatus"
}
func validBrowserGuardSession(id string) bool {
	if len(id) != 64 || strings.ToLower(id) != id {
		return false
	}
	value, err := hex.DecodeString(id)
	return err == nil && len(value) == 32
}

// This is deliberately a flat fixed request schema. No provider/filter IDs,
// paths, root folders, identities, process lists, flags or success booleans can
// be supplied. The native manager derives authority from its held GUI handle.
func decodeBrowserGuardInstall(p json.RawMessage) (browserengineguard.InstallRequest, error) {
	var request browserengineguard.InstallRequest
	decoder := json.NewDecoder(bytes.NewReader(p))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return request, errBrowserGuardRefused
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[strings.ToLower(key)] {
			return request, errBrowserGuardRefused
		}
		seen[strings.ToLower(key)] = true
		switch key {
		case "sessionId", "engineId", "version", "jobHandle", "mainPid", "mainCreated", "relayPort", "controlPort":
		default:
			return request, errBrowserGuardRefused
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return request, errBrowserGuardRefused
		}
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return request, errBrowserGuardRefused
	}
	if _, err := decoder.Token(); err != io.EOF {
		return request, errBrowserGuardRefused
	}
	if strictDecode(p, &request) != nil || !validBrowserGuardSession(request.SessionID) || request.JobHandle == 0 || request.MainPID == 0 || request.MainCreated == 0 || request.RelayPort == 0 {
		return browserengineguard.InstallRequest{}, errBrowserGuardRefused
	}
	// Detailed version, code, parent, Job and relay ownership checks are native
	// manager responsibilities. The dispatcher cannot upgrade a claimed PID to
	// trusted identity and does not offer an arbitrary application firewall API.
	if request.EngineID != "chrome-for-testing" && request.EngineID != "firefox" {
		return browserengineguard.InstallRequest{}, errBrowserGuardRefused
	}
	return request, nil
}
func (d dispatcher) callBrowserGuard(method string, p json.RawMessage) (any, error) {
	capability := browserengineguard.Capability{Available: d.browser != nil}
	if d.browser == nil {
		var failure error
		if browserengineguard.ValidDiagnostic(d.browserDiagnostic) {
			failure = browserengineguard.ErrorWithDiagnostic(*d.browserDiagnostic)
		}
		capability = browserengineguard.UnavailableCapability(failure)
	}
	if method == "BrowserGuardStatus" {
		if !nullParams(p) {
			return nil, errBrowserGuardRefused
		}
		return capability, nil
	}
	var result browserengineguard.Result
	var err error
	var sessionID string
	if method == "BrowserGuardInstall" {
		request, decodeErr := decodeBrowserGuardInstall(p)
		if decodeErr != nil {
			return nil, decodeErr
		}
		sessionID = request.SessionID
		if d.browser == nil {
			// This factory never acquired a native manager or invoked its WFP
			// backend. A framed refusal is not an installed/removed filter claim.
			return browserengineguard.Result{SessionID: request.SessionID, Verified: true, State: "install_not_attempted", Diagnostic: capability.Diagnostic}, nil
		}
		result, err = d.browser.Install(request)
	} else {
		var id string
		if strictDecode(p, &id) != nil || !validBrowserGuardSession(id) {
			return nil, errBrowserGuardRefused
		}
		if d.browser == nil {
			return browserengineguard.Result{SessionID: id, State: "unknown_filters_retained", Diagnostic: capability.Diagnostic}, errBrowserGuardRefused
		}
		if method == "BrowserGuardReport" {
			result, err = d.browser.Report(id)
		} else if method == "BrowserGuardRemove" {
			result, err = d.browser.Remove(id)
		} else {
			return nil, errBrowserGuardRefused
		}
	}
	if err != nil {
		// A complete, fixed-schema install refusal is a normal RPC response,
		// not success installing filters. The UI bridge returns its typed failure
		// after checking session identity. Transport/decode failures remain errors
		// and can never certify that installation was not attempted.
		if method == "BrowserGuardInstall" && result.SessionID == sessionID && browserengineguard.ValidDiagnostic(result.Diagnostic) &&
			(browserengineguard.NoInstallWasAttempted(result, sessionID) || !result.Verified && (result.State == "refused" || result.State == "unknown_filters_retained")) {
			return result, nil
		}
		return result, errBrowserGuardRefused
	}
	return result, nil
}

// BrowserGuardCapability is a fixed read-only authenticated helper query.
// Older/fake helpers need not implement it; absence is never availability.
func (c *Client) BrowserGuardCapability(ctx context.Context) (browserengineguard.Capability, error) {
	var result browserengineguard.Capability
	if err := c.Call(ctx, "BrowserGuardStatus", nil, &result); err != nil {
		return result, err
	}
	if !browserengineguard.ValidCapability(result) {
		return browserengineguard.Capability{}, errBrowserGuardRefused
	}
	return result, nil
}
