package detection

import (
	"strings"
	"time"
)

// Times describe completion of a bounded source read, never runtime enforcement
// or the generation time of an older cached service snapshot. Missing provenance
// remains empty; guidance items do not acquire a fictitious observation time.
func bindSourceTimes(r *Report, sources map[string]string) {
	for i := range r.Findings {
		f := &r.Findings[i]
		source := ""
		switch {
		case strings.HasPrefix(f.ID, "policy-"):
			source = "policies"
		case strings.HasPrefix(f.ID, "network-firewall") || strings.HasPrefix(f.ID, "firewall-rule-"):
			source = "firewall"
		case f.ID == "network-path" || f.ID == "network-ipv6" || f.ID == "network-dns" || f.ID == "local-logs":
			source = "network"
		case f.ID == "timezone" || f.ID == "locale" || f.ID == "location" || f.ID == "camera" || f.ID == "microphone":
			source = "facts"
		}
		f.Source, f.DetectedAt = source, ""
		if value := sources[source]; source != "" && value != "" {
			if stamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
				f.DetectedAt = stamp.UTC().Format(time.RFC3339Nano)
			}
		}
	}
}
