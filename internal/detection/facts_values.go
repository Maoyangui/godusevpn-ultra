package detection

import (
	"encoding/binary"
	"unicode/utf16"
)

// Size is checked before registry allocation by the Windows reader as well as
// here. A malformed local setting must not make a read-only report allocate an
// arbitrarily large registry value before deciding it is outside the scope.
const maxFactStringBytes = 194

func boundedFactString(data []byte, typ uint32) string {
	if (typ != 1 && typ != 2) || len(data) < 2 || len(data) > maxFactStringBytes || len(data)%2 != 0 {
		return ""
	}
	n := len(data)
	if data[n-1] != 0 || data[n-2] != 0 {
		return ""
	}
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		u := binary.LittleEndian.Uint16(data[i:])
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	v := string(utf16.Decode(units))
	if len(v) > 96 {
		return ""
	}
	return v
}
