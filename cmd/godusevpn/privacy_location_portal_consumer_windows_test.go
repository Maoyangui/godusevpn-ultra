//go:build windows

package main

import (
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"strings"
	"testing"
)

func TestLocationPortalConsumerBoundedAndStrict(t *testing.T) {
	good := `{"schemaVersion":1,"status":"observed","position":{"latitude":1.0,"longitude":2},"timestamp":"2026-10-27T01:00:00.1234567Z","accuracyMetres":10,"positionSource":"default"}` + "\n"
	if _, e := decodeLocationConsumer([]byte(good)); e != nil {
		t.Fatal("valid official serializer form refused", e)
	}
	for _, raw := range []string{strings.TrimSuffix(good, "\n"), strings.Replace(good, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1), strings.Replace(good, `"positionSource":"default"`, `"PositionSource":"default"`, 1), strings.Replace(good, `"latitude":1.0,"longitude":2`, `"longitude":2`, 1), good + good, strings.Repeat("x", 4097) + "\n"} {
		if _, e := decodeLocationConsumer([]byte(raw)); e != locationportal.ErrReadback {
			t.Fatal("untrusted consumer output accepted")
		}
	}
	w := &locationBoundedOutput{limit: 4}
	if n, e := w.Write([]byte("1234")); n != 4 || e != nil {
		t.Fatal(e)
	}
	if n, e := w.Write([]byte("x")); n != 0 || e == nil || !w.failed || w.String() != "1234" {
		t.Fatal("unbounded output")
	}
}
