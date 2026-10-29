//go:build windows && (amd64 || arm64)

package browserenginehost

import "testing"

func TestBiDiGeneratedEndpointIsOnlyExpectedNumericLoopbackPort(t *testing.T) {
	for _, raw := range []string{`{"ws_host":"127.0.0.1","ws_port":49152}`, `{"ws_host":"localhost","ws_port":49152}`} {
		if parseBiDiServer([]byte(raw), 49152) != nil {
			t.Fatal("official loopback endpoint refused")
		}
	}
	for _, raw := range []string{`{"ws_host":"example.invalid","ws_port":49152}`, `{"ws_host":"127.0.0.1","ws_port":49153}`, `{"ws_host":"127.0.0.1","ws_port":49152,"secret":"x"}`, `{"ws_host":"127.0.0.1","ws_host":"localhost","ws_port":49152}`, `{"WS_HOST":"127.0.0.1","ws_port":49152}`, `{"ws_host":"127.0.0.1","ws_port":"49152"}`, `{"ws_host":"[::1]","ws_port":49152}`, `{"ws_host":"127.0.0.1","ws_port":49152} {}`} {
		if parseBiDiServer([]byte(raw), 49152) == nil {
			t.Fatal("untrusted endpoint accepted", raw)
		}
	}
}
