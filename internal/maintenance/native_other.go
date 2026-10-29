//go:build !windows

package maintenance

func Acquire() (*Lease, error) { return nil, ErrUnavailable }
func Ready() (bool, error)     { return false, ErrUnavailable }
func Prepare() error           { return ErrUnavailable }
