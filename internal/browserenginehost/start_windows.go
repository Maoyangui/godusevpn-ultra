//go:build windows && (amd64 || arm64)

package browserenginehost

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/browserengine"
)

// The native path uses the installed ordinary GUI and its real helper
// authentication path. Every launch still goes through the same ordinary-user
// token, code-lease, Job, helper/WFP and protocol readback checks; there is no
// frontend flag or alternate unverified path.
func start(ctx context.Context, engines *browserengine.Manager, c Config, guard Guard) (*Session, error) {
	return startAcceptance(ctx, engines, c, guard)
}
