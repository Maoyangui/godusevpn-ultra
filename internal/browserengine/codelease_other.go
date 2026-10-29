//go:build !windows

package browserengine

func pinCodeTree(string, string, string, string, Receipt) (*CodeLease, error) {
	return nil, ErrUnsupported
}
