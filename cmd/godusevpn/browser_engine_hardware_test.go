package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

func TestEngineControllerHardwareReachesHostWithoutChanges(t *testing.T) {
	c, session, _ := engineFixture(t)
	var observed browserenginehost.Config
	c.deps.start = func(_ context.Context, _ *browserengine.Manager, config browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
		observed = config
		return session, nil
	}
	data := `{"hardware":{"mode":"logical-processors","logicalProcessors":16},` + engineConfigFixture[1:]
	if _, err := c.launch(data, engineProxyFixture); err != nil {
		t.Fatal(err)
	}
	if observed.Hardware == nil || observed.Hardware.Mode != "logical-processors" || observed.Hardware.LogicalProcessors != 16 {
		t.Fatal("hardware request was dropped or changed")
	}
}

func TestEngineControllerRejectsInvalidOrUnsupportedHardware(t *testing.T) {
	bad := []string{`null`, `{}`, `{"mode":"native"}`, `{"mode":"logical-processors","logicalProcessors":0}`, `{"mode":"logical-processors","logicalProcessors":65}`, `{"mode":"logical-processors","logicalProcessors":1.0}`, `{"mode":"logical-processors","logicalProcessors":1e1}`, `{"mode":"logical-processors","logicalProcessors":"16"}`, `{"mode":"logical-processors","logicalProcessors":16,"logicalProcessors":8}`, `{"mode":"logical-processors","LogicalProcessors":16}`, `{"mode":"logical-processors","logicalProcessors":16,"extra":true}`}
	for _, value := range bad {
		if _, err := parseBrowserEngineRequest(`{"hardware":` + value + `,` + engineConfigFixture[1:]); err == nil {
			t.Fatal("invalid hardware accepted")
		}
	}
	valid := `{"hardware":{"mode":"logical-processors","logicalProcessors":16},` + engineConfigFixture[1:]
	if _, err := parseBrowserEngineRequest(valid); err != nil {
		t.Fatal("valid Chrome hardware refused")
	}
	ff := strings.Replace(strings.Replace(valid, `"chrome-for-testing"`, `"firefox"`, 1), `"130.0.6723.58"`, `"158.0.1"`, 1)
	if _, err := parseBrowserEngineRequest(ff); err == nil {
		t.Fatal("unsupported Firefox hardware accepted")
	}
}
