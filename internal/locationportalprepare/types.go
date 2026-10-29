// Package locationportalprepare owns explicit preparation of the fixed local
// Windows Device Portal. It does not set coordinates or equate portal readiness
// with an independently observed Windows location.
package locationportalprepare

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"github.com/Maoyangui/godusevpn/internal/locationportalgate"
)

var (
	ErrUnsupported    = errors.New("LOCATION_PORTAL_PREPARATION_UNSUPPORTED")
	ErrBusy           = errors.New("LOCATION_PORTAL_PREPARATION_BUSY")
	ErrConflict       = errors.New("LOCATION_PORTAL_PREPARATION_EXTERNAL_CONFLICT")
	ErrStore          = errors.New("LOCATION_PORTAL_PREPARATION_BACKUP_UNCONFIRMED")
	ErrReadback       = errors.New("LOCATION_PORTAL_PREPARATION_READBACK_UNCONFIRMED")
	ErrClosed         = errors.New("LOCATION_PORTAL_PREPARATION_CLOSED")
	ErrOverrideActive = errors.New("LOCATION_PORTAL_PREPARATION_RESTORE_LOCATION_FIRST")
)

const HTTPPort = 58080
const HTTPSPort = 58443
const CapabilityName = "Tools.DeveloperMode.Core~~~~0.0.1.0"

// Report deliberately has no credentials, owner identity, certificate DER,
// endpoint URL, filesystem path, native output or arbitrary error text.
type Report struct {
	Status                 string   `json:"status"`
	Stage                  string   `json:"stage"`
	Ready                  bool     `json:"ready"`
	Busy                   bool     `json:"busy"`
	CanPrepare             bool     `json:"canPrepare"`
	CanRestore             bool     `json:"canRestore"`
	HasBackup              bool     `json:"hasBackup"`
	RebootRequired         bool     `json:"rebootRequired"`
	DeveloperModeEnabled   bool     `json:"developerModeEnabled"`
	CapabilityInstalled    bool     `json:"capabilityInstalled"`
	ServiceRunning         bool     `json:"serviceRunning"`
	AuthenticationRequired bool     `json:"authenticationRequired"`
	LoopbackGateVerified   bool     `json:"loopbackGateVerified"`
	CertificateVerified    bool     `json:"certificateVerified"`
	HTTPPort               int      `json:"httpPort"`
	HTTPSPort              int      `json:"httpsPort"`
	Detail                 string   `json:"detail"`
	NativeCode             uint32   `json:"nativeCode"`
	OperationID            string   `json:"operationId,omitempty"`
	Warnings               []string `json:"warnings"`
}

type gate interface {
	Ensure(context.Context) (locationportalgate.GateReport, error)
	Inspect(context.Context) (locationportalgate.GateReport, error)
	Remove(context.Context) (locationportalgate.GateReport, error)
}

// State contains only fixed known settings. No arbitrary registry path or
// service name is accepted by any exported method or helper request.
type registryValue struct {
	Present bool   `json:"present"`
	Value   uint32 `json:"value"`
}
type serviceState struct {
	Present   bool   `json:"present"`
	StartType uint32 `json:"startType"`
	State     uint32 `json:"state"`
}
type systemState struct {
	Capability         uint32        `json:"capability"`
	DeveloperMode      registryValue `json:"developerMode"`
	PortalKeyPresent   bool          `json:"portalKeyPresent"`
	PortalConfigSHA256 string        `json:"portalConfigSHA256"`
	DynamicPorts       registryValue `json:"dynamicPorts"`
	HTTPPort           registryValue `json:"httpPort"`
	HTTPSPort          registryValue `json:"httpsPort"`
	Authorizer         registryValue `json:"authorizer"`
	Service            serviceState  `json:"service"`
	SSHBroker          serviceState  `json:"sshBroker"`
	SSHProxy           serviceState  `json:"sshProxy"`
}

type credentials struct{ username, password string }

func (credentials) String() string   { return "<private-portal-preparation-credentials>" }
func (credentials) GoString() string { return "<private-portal-preparation-credentials>" }

type backend interface {
	snapshot(context.Context) (systemState, error)
	addCapability(context.Context) (bool, error)
	removeCapability(context.Context) (bool, error)
	setDeveloperMode(context.Context, registryValue) error
	configure(context.Context, credentials) error
	start(context.Context) error
	stopDisabled(context.Context) error
	secureNewSSH(context.Context, systemState) error
	restoreSettings(context.Context, systemState) error
	certificate(context.Context) (string, error)
	verifyConnection(context.Context, locationportal.Config) error
	overrideDisabled(context.Context, locationportal.Config) (bool, error)
	seal(context.Context, locationportal.Config, string, string, string) ([]byte, error)
	unseal(context.Context, []byte, string, string, string) (locationportal.Config, error)
}

type journal struct {
	Version           int         `json:"version"`
	Owner             string      `json:"owner"`
	TransactionID     string      `json:"transactionId"`
	Stage             string      `json:"stage"`
	Pending           string      `json:"pending,omitempty"`
	Before            systemState `json:"before"`
	Expected          systemState `json:"expected"`
	CertificateSHA256 string      `json:"certificateSHA256,omitempty"`
	CredentialBox     []byte      `json:"credentialBox,omitempty"`
	PortalWasStarted  bool        `json:"portalWasStarted"`
	RestoreAuthorized bool        `json:"restoreAuthorized"`
}
type store interface {
	load() (*journal, error)
	save(*journal) error
	close() error
}
type openStore func(bool) (store, error)

func validPin(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32
}
