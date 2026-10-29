//go:build windows

package main

import (
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

// Only pointer/interface conversion is exercised. No StartRelay, network,
// entropy source, native call or product/browser process is invoked.
func TestBrowserProfileRelayConcreteNilIsNormalizedBeforeCleanup(t *testing.T) {
	failure := errors.New("synthetic relay unavailable")
	partial := &browserproxy.Relay{} // Deliberately not started: conversion must not touch/close it.
	for _, test := range []struct {
		name  string
		relay *browserproxy.Relay
		err   error
	}{
		{"nil-with-error", nil, failure},
		{"nil-without-error", nil, nil},
		{"partial-with-error", partial, failure},
		{"owner-without-error", partial, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, err := normalizeBrowserProfileRelay(test.relay, test.err)
			if err != test.err {
				t.Fatal("producer changed the original failure")
			}
			if test.relay == nil {
				if owner != nil {
					t.Fatal("nil concrete Relay became a live interface")
				}
				return
			}
			if owner != partial {
				t.Fatal("partial owner was lost before caller cleanup")
			}
		})
	}
}
