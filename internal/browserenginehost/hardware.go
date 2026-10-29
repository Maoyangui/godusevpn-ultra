package browserenginehost

import (
	"encoding/json"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
)

// This is an engine emulation request for navigator.hardwareConcurrency, not a
// change to the CPU, scheduler, available processing capacity or performance.
func validateNativeHardware(c Config) error {
	if c.Hardware == nil {
		return nil
	}
	if c.EngineID != browserengine.Chrome || c.Hardware.Mode != "logical-processors" || c.Hardware.LogicalProcessors < 1 || c.Hardware.LogicalProcessors > 64 {
		return ErrRefused
	}
	return nil
}

func copyHardwareConfig(c Config) Config {
	if c.Graphics != nil {
		graphics := *c.Graphics
		c.Graphics = &graphics
	}
	if c.Hardware != nil {
		hardware := *c.Hardware
		c.Hardware = &hardware
	}
	return c
}

// Read the native property; never install a JavaScript getter replacement.
const protocolHardwareReadback = `JSON.stringify({hardwareConcurrency:navigator.hardwareConcurrency})`

func verifyHardwareConcurrency(raw string, expected int) error {
	if len(raw) == 0 || len(raw) > 256 || uniqueProtocolJSON([]byte(raw)) != nil {
		return ErrRefused
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil || len(fields) != 1 || fields["hardwareConcurrency"] == nil {
		return ErrRefused
	}
	var actual *int
	if json.Unmarshal(fields["hardwareConcurrency"], &actual) != nil || actual == nil || *actual != expected {
		return ErrRefused
	}
	return nil
}
