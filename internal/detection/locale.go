package detection

// Windows locale names are bounded ASCII identifiers. Reject untrusted strings
// before consulting NLS, and validate the canonical answer again before export.
// The verifier must check an exact known locale, never resolve by fallback.
func checkedLocale(raw string, verify func(string) string) string {
	if !localeIdentifier(raw) {
		return ""
	}
	canonical := verify(raw)
	if !localeIdentifier(canonical) {
		return ""
	}
	return canonical
}

func localeIdentifier(value string) bool {
	if len(value) < 2 || len(value) > 84 {
		return false
	}
	for i, c := range []byte(value) {
		alpha := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if i == 0 && !alpha {
			return false
		}
		if !alpha && !(c >= '0' && c <= '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
