package privacyhost

import (
	"encoding/json"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"strings"
	"testing"
)

func TestPreparedLocationConfigUsesPrivateWireWhilePublicConfigRedacts(t *testing.T) {
	c := locationportal.Config{Port: 58443, CertificateSHA256: strings.Repeat("a", 64), Username: "private-user", Password: "private-password"}
	public, _ := json.Marshal(c)
	if strings.Contains(string(public), "private-user") || strings.Contains(string(public), "private-password") || strings.Contains(string(public), c.CertificateSHA256) {
		t.Fatal("public config leaked credentials/pin")
	}
	wire, _ := json.Marshal(preparedLocationConfig{Port: c.Port, CertificateSHA256: c.CertificateSHA256, Username: c.Username, Password: c.Password})
	for _, needle := range []string{"private-user", "private-password", c.CertificateSHA256} {
		if !strings.Contains(string(wire), needle) {
			t.Fatal("private helper wire lost field")
		}
	}
	var decoded preparedLocationConfig
	if err := json.Unmarshal(wire, &decoded); err != nil || decoded.Password != c.Password || decoded.CertificateSHA256 != c.CertificateSHA256 {
		t.Fatal("private wire roundtrip failed", err)
	}
}
