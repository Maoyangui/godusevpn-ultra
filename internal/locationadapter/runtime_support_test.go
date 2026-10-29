package locationadapter

import "testing"

func TestProviderBuildGateUsesDocumentedFloor(t *testing.T) {
	for _, tc := range []struct {
		build uint32
		ok    bool
	}{{0, false}, {19045, false}, {22631, false}, {23503, false}, {23504, true}, {26100, true}} {
		if got := providerBuildSupported(tc.build); got != tc.ok {
			t.Fatalf("build %d supported=%v want %v", tc.build, got, tc.ok)
		}
	}
}
