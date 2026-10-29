package detection

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func registryString(v string) []byte {
	units := append(utf16.Encode([]rune(v)), 0)
	b := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[i*2:], u)
	}
	return b
}
func TestBoundedRegistryFactsAcceptOnlySmallStrings(t *testing.T) {
	for _, v := range []string{"zh-CN", "Allow", "Deny"} {
		if got := boundedFactString(registryString(v), 1); got != v {
			t.Fatal(got, v)
		}
	}
	for _, sample := range []struct {
		data []byte
		typ  uint32
	}{
		{registryString(strings.Repeat("x", 97)), 1},
		{make([]byte, 1<<20), 1},
		{[]byte{1, 0, 0}, 1},
		{[]byte{'x', 0}, 1},
		{registryString("Allow"), 4},
	} {
		if got := boundedFactString(sample.data, sample.typ); got != "" {
			t.Fatal("unbounded/malformed fact accepted", got)
		}
	}
}
