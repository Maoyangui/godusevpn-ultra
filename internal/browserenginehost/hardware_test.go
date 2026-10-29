package browserenginehost

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

func hardwareConfig(count int) *browserhost.HardwareConfig {
	return &browserhost.HardwareConfig{Mode: "logical-processors", LogicalProcessors: count}
}

func TestNativeHardwareConfigRefusesUnsupportedEngineOrValues(t *testing.T) {
	for _, engine := range []string{"chrome-for-testing", "firefox"} {
		f := newDriverFixture(t, engine)
		if validateConfig(f.config) != nil {
			t.Fatal("native omission rejected", engine)
		}
		for _, count := range []int{-1, 0, 1, 64, 65} {
			c := f.config
			c.Hardware = hardwareConfig(count)
			accepted := engine == "chrome-for-testing" && count >= 1 && count <= 64
			if (validateConfig(c) == nil) != accepted {
				t.Fatal("wrong hardware validation", engine, count)
			}
		}
		f.config.Hardware = &browserhost.HardwareConfig{Mode: "native", LogicalProcessors: 8}
		if validateConfig(f.config) == nil {
			t.Fatal("explicit native or unknown mode accepted")
		}
	}
}

func TestNativeHardwareObservationRequiresExactIntegerEvidence(t *testing.T) {
	if verifyHardwareConcurrency(`{"hardwareConcurrency":4}`, 4) != nil {
		t.Fatal("native integer rejected")
	}
	for _, raw := range []string{``, `{}`, `null`, `{"hardwareConcurrency":null}`, `{"hardwareConcurrency":8}`, `{"hardwareConcurrency":"4"}`, `{"hardwareConcurrency":4.0}`, `{"hardwareConcurrency":4e0}`, `{"HardwareConcurrency":4}`, `{"hardwareConcurrency":4,"extra":true}`, `{"hardwareConcurrency":4,"hardwareConcurrency":4}`} {
		if verifyHardwareConcurrency(raw, 4) == nil {
			t.Fatal("accepted absent, ambiguous or mismatching evidence", raw)
		}
	}
}

func TestChromeHardwareNativeOmissionDoesNotIssueOverride(t *testing.T) {
	f := newDriverFixture(t, "chrome-for-testing")
	f.start()
	d := newChromeDriver(f.m, f.config, f.launch, nil)
	defer d.Close()
	if err := d.Configure(context.Background()); err != nil || f.count("Emulation.setHardwareConcurrencyOverride") != 0 {
		t.Fatal("native omission changed hardware", err)
	}
}

func TestChromeHardwareConfiguredAndReadBeforeResumeThenVerifiedAcrossFrames(t *testing.T) {
	for _, count := range []int{1, 4, 64} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			f.config.Hardware = hardwareConfig(count)
			f.start()
			d := newChromeDriver(f.m, f.config, f.launch, nil)
			defer d.Close()
			// The driver owns a snapshot of the request, not a mutable caller pointer.
			f.config.Hardware.LogicalProcessors = 63
			if err := d.Configure(context.Background()); err != nil {
				t.Fatal(err)
			}
			set, read, resumed := false, false, false
			for _, command := range f.snapshot() {
				if command.Session != "main-session" {
					continue
				}
				switch command.Method {
				case "Emulation.setHardwareConcurrencyOverride":
					if command.Params["hardwareConcurrency"] != float64(count) || len(command.Params) != 1 {
						t.Fatal("wrong native command")
					}
					set = true
				case "Runtime.evaluate":
					if command.Params["expression"] == protocolHardwareReadback {
						if !set || resumed {
							t.Fatal("native hardware read outside paused configuration")
						}
						read = true
					}
				case "Runtime.runIfWaitingForDebugger":
					if !set || !read {
						t.Fatal("target resumed without hardware evidence")
					}
					resumed = true
				}
			}
			if !resumed || f.count("Fetch.fulfillRequest") != 2 {
				t.Fatal("missing real HTTP bootstrap phases")
			}
		})
	}
}

func TestChromeHardwareFailureNeverReady(t *testing.T) {
	for _, fault := range []string{"command", "paused-missing", "paused-wrong", "paused-exception", "main", "same-origin", "opaque-origin", "frame-missing", "http-ua"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			f.config.Hardware = hardwareConfig(4)
			if fault == "command" {
				f.fault = "Emulation.setHardwareConcurrencyOverride"
			} else if fault == "http-ua" {
				f.fault = "http-ua"
			}
			f.mutate = func(command driverCommand, result map[string]any) {
				if command.Method != "Runtime.evaluate" {
					return
				}
				if command.Params["expression"] == protocolHardwareReadback {
					switch fault {
					case "paused-missing":
						result["result"] = map[string]any{"type": "string", "value": `{}`}
					case "paused-wrong":
						result["result"] = map[string]any{"type": "string", "value": `{"hardwareConcurrency":8}`}
					case "paused-exception":
						result["exceptionDetails"] = map[string]any{"text": "no execution context"}
					}
				}
				if command.Params["expression"] != protocolReadback {
					return
				}
				var observation map[string]any
				json.Unmarshal([]byte(f.observation()), &observation)
				target := observation["top"].(map[string]any)
				if fault != "main" {
					for _, frame := range observation["frames"].([]any) {
						value := frame.(map[string]any)
						if value["kind"] == fault || fault == "frame-missing" {
							target = value
							break
						}
					}
				}
				if fault == "main" || fault == "same-origin" || fault == "opaque-origin" {
					target["hardwareConcurrency"] = 8
				} else if fault == "frame-missing" {
					delete(target, "hardwareConcurrency")
				}
				encoded, _ := json.Marshal(observation)
				result["result"] = map[string]any{"type": "string", "value": string(encoded)}
			}
			f.start()
			d := newChromeDriver(f.m, f.config, f.launch, nil)
			defer d.Close()
			if d.Configure(context.Background()) == nil || d.Navigate(context.Background(), "https://example.invalid") == nil {
				t.Fatal("failed hardware evidence reached readiness")
			}
			if (fault == "command" || fault == "paused-missing" || fault == "paused-wrong" || fault == "paused-exception") && f.count("Runtime.runIfWaitingForDebugger") != 0 {
				t.Fatal("failed paused target resumed")
			}
		})
	}
}

func TestChromeHardwareLaterOOPIFRequiresOwnNativeReadbackBeforeResume(t *testing.T) {
	for _, fault := range []string{"none", "command", "wrong", "exception"} {
		t.Run(fault, func(t *testing.T) {
			f := newDriverFixture(t, "chrome-for-testing")
			f.config.Hardware = hardwareConfig(4)
			if fault == "command" {
				f.fault, f.faultSession = "Emulation.setHardwareConcurrencyOverride", "child-session"
			}
			f.mutate = func(command driverCommand, result map[string]any) {
				if command.Session == "child-session" && command.Method == "Runtime.evaluate" && command.Params["expression"] == protocolHardwareReadback {
					if fault == "wrong" {
						result["result"] = map[string]any{"type": "string", "value": `{"hardwareConcurrency":8}`}
					} else if fault == "exception" {
						result["exceptionDetails"] = map[string]any{"text": "no execution context"}
					}
				}
			}
			f.start()
			aborted := make(chan struct{})
			d := newChromeDriver(f.m, f.config, f.launch, func() { close(aborted) })
			defer d.Close()
			if err := d.Configure(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.event("Target.attachedToTarget", map[string]any{"sessionId": "child-session", "waitingForDebugger": true, "targetInfo": map[string]any{"targetId": "child-target", "type": "iframe", "browserContextId": "owned-context"}}, "")
			if fault != "none" {
				awaitSignal(t, aborted)
			} else {
				deadline := time.After(time.Second)
				tick := time.NewTicker(time.Millisecond)
				defer tick.Stop()
			wait:
				for {
					for _, command := range f.snapshot() {
						if command.Session == "child-session" && command.Method == "Runtime.runIfWaitingForDebugger" {
							break wait
						}
					}
					select {
					case <-deadline:
						t.Fatal("verified child never resumed")
					case <-tick.C:
					}
				}
			}
			set, read, resumed := false, false, false
			for _, command := range f.snapshot() {
				if command.Session != "child-session" {
					continue
				}
				if command.Method == "Emulation.setHardwareConcurrencyOverride" {
					set = command.Params["hardwareConcurrency"] == float64(4)
				}
				if command.Method == "Runtime.evaluate" && command.Params["expression"] == protocolHardwareReadback {
					if !set {
						t.Fatal("child property read before native command")
					}
					read = true
				}
				if command.Method == "Runtime.runIfWaitingForDebugger" {
					if !set || !read || fault != "none" {
						t.Fatal("unverified child resumed")
					}
					resumed = true
				}
			}
			if resumed != (fault == "none") {
				t.Fatal("wrong child resume state")
			}
		})
	}
}

func TestFirefoxCustomHardwareRefusedBeforeAnyProtocolMutation(t *testing.T) {
	f := newDriverFixture(t, "firefox")
	f.config.Hardware = hardwareConfig(4)
	f.start()
	d := newFirefoxDriver(f.m, f.config, f.launch, nil)
	defer d.Close()
	if d.Configure(context.Background()) == nil || len(f.snapshot()) != 0 {
		t.Fatal("Firefox custom hardware reached protocol mutation")
	}
}
