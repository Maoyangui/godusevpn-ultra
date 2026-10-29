//go:build !windows

package browserengine

import "os"

func defaultDirectory() (string, error)         { return "", ErrUnsupported }
func acquireStore(string, bool) (func(), error) { return nil, ErrUnsupported }
func verifyBinaryVersion(string, string) error  { return ErrUnsupported }
func verifyRootIdentity(*os.Root, string) error { return ErrUnsupported }
