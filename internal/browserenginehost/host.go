// Package browserenginehost controls only product-owned Chrome/Firefox sessions.
// It is separate from the WebView2 GUI and never accepts arbitrary executables.
package browserenginehost

import (
	"context"
	"errors"
	"sync"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/browserhost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

var ErrRefused = errors.New("browser_engine_host_refused")
var ErrNotReady = errors.New("browser_engine_host_not_ready")

// NativeLaunchEnabled reports whether the controlled native browser launch
// path is compiled for this platform. Windows amd64/arm64 builds expose the
// path; startAcceptance still performs all runtime checks before navigation.
func NativeLaunchEnabled() bool {
	return nativeAcceptanceBuild || nativeLaunchPlatformEnabled()
}

type Config struct {
	EngineID, Version, ProfileID, Language, Timezone string
	Width, Height                                    int
	ExitCleanup                                      string
	Proxy                                            browserproxy.Config
	Identity                                         browserhost.ProfileIdentity
	Environment                                      *browserhost.EnvironmentConfig
	Hardware                                         *browserhost.HardwareConfig
	Graphics                                         *browserhost.GraphicsConfig
	WebGPU                                           string
}
type Guard interface {
	Install(context.Context, browserengineguard.InstallRequest) (browserengineguard.Result, error)
	Report(context.Context, string) (browserengineguard.Result, error)
	Remove(context.Context, string) (browserengineguard.Result, error)
}
type Status struct {
	SessionID   string                         `json:"sessionId"`
	EngineID    string                         `json:"engineId"`
	Version     string                         `json:"version"`
	State       string                         `json:"state"`
	Ready       bool                           `json:"ready"`
	CanNavigate bool                           `json:"canNavigate"`
	Detail      string                         `json:"detail"`
	Graphics    *GraphicsEvidence              `json:"graphics,omitempty"`
	Diagnostic  *browserengineguard.Diagnostic `json:"diagnostic,omitempty"`
}
type Session struct {
	mu       sync.Mutex
	id       string
	status   Status
	done     chan struct{}
	close    func() error
	navigate func(context.Context, string) error
	once     sync.Once
	closeErr error
}

func (s *Session) ID() string { return s.id }
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.status
	if status.Graphics != nil {
		copy := *status.Graphics
		status.Graphics = &copy
	}
	status.Diagnostic = copyGuardDiagnostic(status.Diagnostic)
	return status
}
func (s *Session) Done() <-chan struct{} { return s.done }
func (s *Session) Close() error {
	s.once.Do(func() {
		if s.close != nil {
			s.closeErr = s.close()
		}
	})
	return s.closeErr
}
func (s *Session) Navigate(ctx context.Context, url string) error {
	if s.navigate == nil {
		return ErrNotReady
	}
	return s.navigate(ctx, url)
}

func Start(ctx context.Context, engines *browserengine.Manager, config Config, guard Guard) (*Session, error) {
	return start(ctx, engines, config, guard)
}

// StartAcceptance is the production native path exposed only to the controlled
// VM acceptance harness. It is not a Wails method or a frontend bypass flag.
// Public Start remains closed until the real engine/traffic matrix is accepted.
func StartAcceptance(ctx context.Context, engines *browserengine.Manager, config Config, guard Guard) (*Session, error) {
	return startAcceptance(ctx, engines, config, guard)
}
