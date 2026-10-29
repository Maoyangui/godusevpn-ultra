package privacyhost

import (
	"context"
	"net"

	"github.com/Maoyangui/godusevpn/internal/locationportal"
	prep "github.com/Maoyangui/godusevpn/internal/locationportalprepare"
)

type preparedLocationConfig struct {
	Port              int    `json:"port"`
	CertificateSHA256 string `json:"certificateSHA256"`
	Username          string `json:"username"`
	Password          string `json:"password"`
}

// LocationPortalPreparation keeps credentials in the authenticated helper
// response only. The public report never includes the Config fields.
func (c *Client) LocationPortalPreparation(ctx context.Context, method string) (prep.Report, *locationportal.Config, error) {
	if c == nil {
		return prep.Report{}, nil, net.ErrClosed
	}
	if method != "LocationPortalPreparationReport" && method != "LocationPortalPreparationPrepare" && method != "LocationPortalPreparationRestore" {
		return prep.Report{}, nil, prep.ErrUnsupported
	}
	var out struct {
		Report prep.Report             `json:"report"`
		Config *preparedLocationConfig `json:"config,omitempty"`
	}
	if err := c.Call(ctx, method, nil, &out); err != nil {
		return out.Report, nil, err
	}
	if out.Config != nil {
		v := locationportal.Config{Port: out.Config.Port, CertificateSHA256: out.Config.CertificateSHA256, Username: out.Config.Username, Password: out.Config.Password}
		return out.Report, &v, nil
	}
	return out.Report, nil, nil
}
