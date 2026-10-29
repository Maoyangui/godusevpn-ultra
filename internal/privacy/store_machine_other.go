//go:build !windows

package privacy

import (
	"errors"
	"os"
)

func validateMachineStoreRead(string) error     { return errors.New("Windows-only machine store") }
func validateMachineDirectoryRead(string) error { return errors.New("Windows-only machine directory") }
func openMachineJournal(string) (*os.File, error) {
	return nil, errors.New("Windows-only machine store")
}
func lockMachineStore(string) (func(), error)  { return nil, errors.New("Windows-only machine store") }
func durableMachineWrite(string, []byte) error { return errors.New("Windows-only machine store") }
