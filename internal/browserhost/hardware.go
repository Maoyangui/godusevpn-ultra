package browserhost

import (
	"bytes"
	"encoding/json"
)

// HardwareConfig controls one website-visible value through the engine's
// official emulation protocol. It neither replaces a CPU nor changes available
// processing capacity. Omission keeps the native value; explicit null is invalid.
type HardwareConfig struct {
	Mode              string `json:"mode"`
	LogicalProcessors int    `json:"logicalProcessors"`
}

func (h *HardwareConfig) UnmarshalJSON(data []byte) error {
	if len(data) > 256 || rejectDuplicateKeys(data) != nil {
		return invalid("hardware", "invalid_or_duplicate_fields")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 2 || fields["mode"] == nil || fields["logicalProcessors"] == nil {
		return invalid("hardware", "exact_object_required")
	}
	var value struct {
		Mode              string `json:"mode"`
		LogicalProcessors int    `json:"logicalProcessors"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || value.Mode != "logical-processors" || value.LogicalProcessors < 1 || value.LogicalProcessors > 64 {
		return invalid("hardware", "logical_processors_integer_1_to_64_required")
	}
	*h = HardwareConfig{Mode: value.Mode, LogicalProcessors: value.LogicalProcessors}
	return nil
}

func validateHardware(config ProfileConfig) error {
	if config.Hardware == nil {
		return nil
	}
	if config.Hardware.Mode != "logical-processors" || config.Hardware.LogicalProcessors < 1 || config.Hardware.LogicalProcessors > 64 {
		return invalid("hardware", "logical_processors_integer_1_to_64_required")
	}
	return nil
}

type HardwareEnforcementRequirements struct {
	BeforeNavigation        bool     `json:"beforeNavigation"`
	Method                  string   `json:"method"`
	LogicalProcessors       int      `json:"logicalProcessors"`
	Contexts                []string `json:"contexts"`
	WorkersBlockedBeforeRun bool     `json:"workersBlockedBeforeRun"`
	CloseOnReadbackMismatch bool     `json:"closeOnReadbackMismatch"`
	ChangesNativeHardware   bool     `json:"changesNativeHardware"`
}

// This is a proposed host requirement, never an applied/read-back receipt.
func HardwareRequirements(config ProfileConfig) *HardwareEnforcementRequirements {
	if config.Hardware == nil {
		return nil
	}
	return &HardwareEnforcementRequirements{
		BeforeNavigation: true, Method: "Emulation.setHardwareConcurrencyOverride", LogicalProcessors: config.Hardware.LogicalProcessors,
		Contexts:                []string{"main-frame", "same-origin-frame", "srcdoc-frame", "aboutblank-frame", "paused-oopif"},
		WorkersBlockedBeforeRun: true, CloseOnReadbackMismatch: true,
	}
}
