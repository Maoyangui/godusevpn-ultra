package daemon

import (
	"strings"

	"github.com/Maoyangui/godusevpn/internal/settings"
)

const trialProfileKind = "builtin-trial"

// isTrialProfile also recognizes records written by clients before Kind was
// introduced. Those records are exactly the rows the rotating trial URL must
// replace, instead of leaving a stale link beside the new one.
func isTrialProfile(p settings.Profile) bool {
	if p.Kind == trialProfileKind {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(p.Name)) {
	case "免费试用", "free trial", "free trial nodes":
		return true
	default:
		return false
	}
}

func trialProfileIndexes(profiles []settings.Profile) []int {
	out := make([]int, 0, 1)
	for i, p := range profiles {
		if isTrialProfile(p) {
			out = append(out, i)
		}
	}
	return out
}

// upsertTrialProfileSettings updates the single product-owned trial row while
// preserving every user-managed subscription. Older clients used to append a
// row on each claim, so this also removes duplicate legacy rows and returns
// their IDs for cache cleanup by the caller.
func upsertTrialProfileSettings(s settings.Settings, name, url string) (settings.Settings, string, []string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "免费试用"
	}
	matches := trialProfileIndexes(s.Profiles)
	if len(matches) == 0 {
		sp := settings.Profile{ID: settings.NewID(), Name: name, URL: url, Kind: trialProfileKind}
		s.Profiles = append(s.Profiles, sp)
		if len(s.Profiles) == 1 {
			s.ActiveProfile = sp.ID
		}
		return s, sp.ID, nil
	}

	primary := matches[0]
	for _, i := range matches {
		if s.Profiles[i].ID == s.ActiveProfile {
			primary = i
			break
		}
	}
	primaryID := s.Profiles[primary].ID
	removedIDs := make([]string, 0, len(matches)-1)
	kept := make([]settings.Profile, 0, len(s.Profiles)-len(matches)+1)
	for _, sp := range s.Profiles {
		if isTrialProfile(sp) && sp.ID != primaryID {
			removedIDs = append(removedIDs, sp.ID)
			continue
		}
		kept = append(kept, sp)
	}
	s.Profiles = kept
	for i := range s.Profiles {
		if s.Profiles[i].ID == primaryID {
			s.Profiles[i].Name = name
			s.Profiles[i].URL = url
			s.Profiles[i].Kind = trialProfileKind
			break
		}
	}
	return s, primaryID, removedIDs
}
