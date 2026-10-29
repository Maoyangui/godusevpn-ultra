//go:build !windows || (!amd64 && !arm64)

package browserenginehost

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/browserengine"
)

func start(context.Context, *browserengine.Manager, Config, Guard) (*Session, error) {
	return nil, ErrRefused
}
func startAcceptance(context.Context, *browserengine.Manager, Config, Guard) (*Session, error) {
	return nil, ErrRefused
}
