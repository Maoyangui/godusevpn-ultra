//go:build !windows

package browserengine

import "os"

func openVersionDeletion(*os.Root, string, string, Receipt) (versionDeletion, error) {
	return nil, ErrUnsupported
}
