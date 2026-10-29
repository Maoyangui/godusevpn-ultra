//go:build !windows

package main

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
)

func runLocationConsumer(context.Context, locationportal.ObserveRequest) (locationportal.Observation, error) {
	return locationportal.Observation{}, locationportal.ErrUnsupported
}
