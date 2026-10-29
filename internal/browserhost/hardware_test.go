package browserhost

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestHardwareLogicalProcessorsRoundTripAndPlanRemainUnapplied(t *testing.T) {
	runtime := previewRuntime()
	config := validProfile(t)
	original, err := EncodeProfile(config, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(original), `"hardware"`) {
		t.Fatal("legacy profile gained hardware override")
	}
	decoded, err := DecodeProfile(original, runtime)
	if err != nil || decoded.Hardware != nil {
		t.Fatal("legacy default changed")
	}
	for count := 1; count <= 64; count++ {
		config.Hardware = &HardwareConfig{Mode: "logical-processors", LogicalProcessors: count}
		data, err := EncodeProfile(config, runtime)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeProfile(data, runtime)
		if err != nil || !reflect.DeepEqual(decoded.Hardware, config.Hardware) {
			t.Fatalf("count %d: %v", count, err)
		}
		preview, err := PreviewProfile(config, runtime)
		if err != nil {
			t.Fatal(err)
		}
		plan := preview.HardwareRequirements
		if plan == nil || plan.Method != "Emulation.setHardwareConcurrencyOverride" || plan.LogicalProcessors != count || !plan.BeforeNavigation || !plan.WorkersBlockedBeforeRun || !plan.CloseOnReadbackMismatch || plan.ChangesNativeHardware || preview.Applied || preview.CanLaunch || !preview.NeedsRuntimeReadback || len(plan.Contexts) != 5 {
			t.Fatalf("false hardware claim: %+v", preview)
		}
		config.Hardware.LogicalProcessors = 65
		if preview.Config.Hardware.LogicalProcessors != count || plan.LogicalProcessors != count {
			t.Fatal("preview shares mutable hardware pointer")
		}
	}
}

func TestHardwareRejectsAmbiguousOrUnsupportedJSON(t *testing.T) {
	runtime := previewRuntime()
	config := validProfile(t)
	base, err := EncodeProfile(config, runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`null`, `{}`, `[]`, `"8"`, `false`, `{"mode":"native"}`, `{"mode":"logical-processors"}`, `{"logicalProcessors":8}`, `{"mode":"logical-processors","logicalProcessors":null}`, `{"mode":"logical-processors","logicalProcessors":"8"}`, `{"mode":"logical-processors","logicalProcessors":8.0}`, `{"mode":"logical-processors","logicalProcessors":8e0}`, `{"mode":"logical-processors","logicalProcessors":0}`, `{"mode":"logical-processors","logicalProcessors":-1}`, `{"mode":"logical-processors","logicalProcessors":65}`, `{"mode":"logical-processors","logicalProcessors":2147483648}`, `{"Mode":"logical-processors","logicalProcessors":8}`, `{"mode":"logical-processors","LogicalProcessors":8}`, `{"mode":"logical-processors","logicalProcessors":8,"ram":16}`, `{"mode":"logical-processors","logicalProcessors":8,"logicalProcessors":8}`, `{"mode":"logical-processors","logicalProcessors":8,"LOGICALPROCESSORS":8}`, `{"mode":"logical-processors","logicalProcessors":8,"logical\u0050rocessors":8}`} {
		data := strings.Replace(string(base), `"identity":`, `"hardware":`+raw+`,"identity":`, 1)
		if _, err := DecodeProfile([]byte(data), runtime); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, name := range []string{"Hardware", "HARDWARE", "hArDwArE"} {
		data := strings.Replace(string(base), `"identity":`, `"`+name+`":{"mode":"logical-processors","logicalProcessors":8},"identity":`, 1)
		if _, err := DecodeProfile([]byte(data), runtime); err == nil {
			t.Fatal("case alias accepted")
		}
	}
	for _, hardware := range []*HardwareConfig{{Mode: "native", LogicalProcessors: 8}, {Mode: "logical-processors", LogicalProcessors: 0}, {Mode: "logical-processors", LogicalProcessors: 65}} {
		config.Hardware = hardware
		var field *ConfigError
		if err := ValidateProfile(config, runtime); !errors.As(err, &field) || field.Field != "hardware" {
			t.Fatalf("typed invalid hardware accepted: %v", err)
		}
	}
}

func TestHardwareDoesNotModifyUAEnvironmentOrRandomization(t *testing.T) {
	runtime := previewRuntime()
	config := validProfile(t)
	config.Identity = ProfileIdentity{Mode: "manual", UserAgent: "  User-owned/1.0  "}
	config.Environment = &EnvironmentConfig{Mode: "browser-emulation", OS: "android", DevicePresetID: "pixel-7", OSVersion: "17.2"}
	config.EngineID = "webview2"
	runtime.Engine.ID = "webview2"
	config.Viewport = ViewportConfig{412, 915}
	config.Hardware = &HardwareConfig{Mode: "logical-processors", LogicalProcessors: 3}
	original := config.Hardware
	preview, err := RandomizedProfilePreview(config, runtime, "cpu-does-not-randomize")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Config.Hardware.LogicalProcessors != 3 || preview.Config.Identity.UserAgent != config.Identity.UserAgent || preview.Config.Environment.OSVersion != "17.2" || config.Hardware != original {
		t.Fatal("CPU changed unrelated requested settings")
	}
	data, err := json.Marshal(preview.Config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeProfile(data, runtime); err != nil {
		t.Fatal(err)
	}
	native := validProfile(t)
	if HardwareRequirements(native) != nil {
		t.Fatal("native omission has override plan")
	}
}
