package browserprofiles

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Saved records still carry only an unapplied proposal. New CPU metadata is
// round-tripped without a service binding, observed hardware or launch trust.
func TestSavedDraftHardwareCompatibility(t *testing.T) {
	m, _ := managerFixture()
	request := mutateJSON(t, requestFixture(t, "CPU 可编辑草稿", nil), func(value map[string]any) {
		value["config"].(map[string]any)["hardware"] = map[string]any{"mode": "logical-processors", "logicalProcessors": 3}
	})
	saved, err := m.Save(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := m.Load(context.Background(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if json.Unmarshal(loaded.Config, &config) != nil || config["hardware"].(map[string]any)["logicalProcessors"] != float64(3) || config["binding"] != nil || loaded.Applied || !loaded.RequiresLaunchValidation {
		t.Fatal("hardware proposal changed or gained trust")
	}
	for _, hardware := range []any{nil, map[string]any{}, map[string]any{"mode": "logical-processors", "logicalProcessors": 65}, map[string]any{"mode": "logical-processors", "logicalProcessors": 8, "ram": 16}} {
		invalid := mutateJSON(t, request, func(value map[string]any) { value["config"].(map[string]any)["hardware"] = hardware })
		if _, err = m.Save(context.Background(), invalid); !errors.Is(err, ErrInput) {
			t.Fatal("unsupported saved hardware accepted")
		}
	}
}
