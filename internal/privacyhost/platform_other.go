//go:build !windows || (!amd64 && !arm64)

package privacyhost

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
)

func Start(context.Context, string) (*Client, error)                      { return nil, ErrUnsupported }
func StartMaintenance(context.Context, string) (*Client, error)           { return nil, ErrUnsupported }
func IsElevated() bool                                                    { return false }
func ExecuteElevated(string, any, any) error                              { return ErrUnsupported }
func ExecuteElevatedWithLease(*maintenance.Lease, string, any, any) error { return ErrUnsupported }
func Handle(args []string) (bool, int) {
	if len(args) > 0 && args[0] == helperFlag {
		return true, 1
	}
	return false, 0
}
