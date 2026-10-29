package locationadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func observationFixture() (Position, time.Time, time.Time, SystemObservation, ObservationPolicy) {
	now := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	position := Position{Latitude: 12.3456789, Longitude: 123.4567891, AccuracyMeter: 30}
	return position, now.Add(-2 * time.Second), now, SystemObservation{Position: position, Timestamp: now.Add(-time.Second), Source: SourceSatellite, SourceKnown: true, IsRemoteSource: true, RemoteSourceKnown: true}, DefaultObservationPolicy()
}

func TestIndependentLocationObservationMatchDoesNotClaimApplyOwnershipOrLeak(t *testing.T) {
	position, start, now, observation, policy := observationFixture()
	result, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy)
	if err != nil || !result.MatchesRequestedPosition || result.Applied || result.ProviderOwnershipVerified || result.Status != "observation_matches_request" {
		t.Fatalf("false result: %+v %v", result, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	privateJSON, _ := json.Marshal(observation)
	if string(privateJSON) != "{}" {
		t.Fatal("observation automatically serializes private data")
	}
	for _, value := range []string{string(encoded), fmt.Sprintf("%v", observation), fmt.Sprintf("%+v", observation), fmt.Sprintf("%#v", observation)} {
		for _, secret := range []string{"12.3456789", "123.4567891", "2026-10-26", "Satellite"} {
			if strings.Contains(value, secret) {
				t.Fatal("default output disclosed observation")
			}
		}
	}
}

func TestObservationRejectsCacheFutureUnknownSourceAndInexactFix(t *testing.T) {
	position, start, now, base, policy := observationFixture()
	cases := []struct {
		name   string
		mutate func(*SystemObservation)
		want   error
	}{
		{"old-matching-cache", func(o *SystemObservation) { o.Timestamp = start.Add(-time.Nanosecond) }, ErrObservationStale},
		{"cache-over-age", func(o *SystemObservation) { o.Timestamp = now.Add(-policy.MaxAge - time.Nanosecond) }, ErrObservationStale},
		{"future", func(o *SystemObservation) { o.Timestamp = now.Add(policy.FutureClockSkew + time.Nanosecond) }, ErrObservationFuture},
		{"zero-timestamp", func(o *SystemObservation) { o.Timestamp = time.Time{} }, ErrObservationInput},
		{"not-remote", func(o *SystemObservation) { o.IsRemoteSource = false }, ErrObservationSource},
		{"remote-flag-not-observed", func(o *SystemObservation) { o.RemoteSourceKnown = false }, ErrObservationSource},
		{"source-not-observed", func(o *SystemObservation) { o.SourceKnown = false }, ErrObservationSource},
		{"ordinary-ip-fallback", func(o *SystemObservation) { o.Source = SourceIPAddress }, ErrObservationSource},
		{"unknown", func(o *SystemObservation) { o.Source = SourceUnknown }, ErrObservationSource},
		{"manual-default", func(o *SystemObservation) { o.Source = SourceDefault }, ErrObservationSource},
		{"obfuscated", func(o *SystemObservation) { o.Source = SourceObfuscated }, ErrObservationSource},
		{"foreign-enum", func(o *SystemObservation) { o.Source = 37 }, ErrObservationSource},
		{"coarser-than-request", func(o *SystemObservation) { o.Position.AccuracyMeter = 31 }, ErrObservationPrecision},
		{"zero-accuracy", func(o *SystemObservation) { o.Position.AccuracyMeter = 0 }, ErrObservationInput},
		{"latitude-invalid", func(o *SystemObservation) { o.Position.Latitude = 91 }, ErrObservationInput},
		{"longitude-invalid", func(o *SystemObservation) { o.Position.Longitude = math.Inf(1) }, ErrObservationInput},
		{"nan", func(o *SystemObservation) { o.Position.Latitude = math.NaN() }, ErrObservationInput},
		{"wrong-location", func(o *SystemObservation) { o.Position.Longitude += 0.01 }, ErrObservationMismatch},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			observation := base
			test.mutate(&observation)
			result, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy)
			if !errors.Is(err, test.want) || result.MatchesRequestedPosition || result.Applied || result.ProviderOwnershipVerified {
				t.Fatalf("accepted invalid observation: %+v %v", result, err)
			}
		})
	}
	for source := SourceCellular; source <= SourceIPAddress; source++ {
		observation := base
		observation.Source = source
		if _, err := CheckOverrideObservation(position, source, start, now, observation, policy); err != nil {
			t.Fatal("known matching remote source refused", err)
		}
	}
}

func TestObservationPolicyLimitsAndFreshnessBoundary(t *testing.T) {
	position, start, now, observation, base := observationFixture()
	for _, mutate := range []func(*ObservationPolicy){func(p *ObservationPolicy) { p.MaxAge = 0 }, func(p *ObservationPolicy) { p.MaxAge = time.Minute + 1 }, func(p *ObservationPolicy) { p.FutureClockSkew = -1 }, func(p *ObservationPolicy) { p.FutureClockSkew = 2*time.Second + 1 }, func(p *ObservationPolicy) { p.MaxDistanceMeter = 0 }, func(p *ObservationPolicy) { p.MaxDistanceMeter = 1001 }, func(p *ObservationPolicy) { p.MaxDistanceMeter = math.NaN() }, func(p *ObservationPolicy) { p.MaxReportedAccuracyMeter = 10001 }} {
		policy := base
		mutate(&policy)
		if _, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy); !errors.Is(err, ErrObservationInput) {
			t.Fatal("unsafe policy accepted", err)
		}
	}
	observation.Timestamp = now.Add(-base.MaxAge)
	if _, err := CheckOverrideObservation(position, SourceSatellite, observation.Timestamp, now, observation, base); err != nil {
		t.Fatal("max-age inclusive boundary", err)
	}
	observation.Timestamp = now.Add(base.FutureClockSkew)
	if _, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, base); err != nil {
		t.Fatal("allowed clock-skew inclusive boundary", err)
	}
	if _, err := CheckOverrideObservation(position, SourceSatellite, now.Add(time.Second), now, observation, base); !errors.Is(err, ErrObservationInput) {
		t.Fatal("future apply timestamp accepted")
	}
	observation.Timestamp = now
	policy := base
	policy.MaxReportedAccuracyMeter = 20
	if _, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy); !errors.Is(err, ErrObservationPrecision) {
		t.Fatal("policy quality limit ignored")
	}
}

func TestGeolocationDistanceWrapPolesAndIndependentAccuracy(t *testing.T) {
	_, start, now, observation, policy := observationFixture()
	for _, position := range []Position{{Latitude: 0, Longitude: 179.99999, AccuracyMeter: 30}, {Latitude: 90, Longitude: -180, AccuracyMeter: 30}, {Latitude: -90, Longitude: 180, AccuracyMeter: 30}} {
		observation.Position = position
		observation.Position.Longitude = -position.Longitude
		if _, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy); err != nil {
			t.Fatal("wrapped nearby point rejected", err)
		}
	}
	position := Position{Latitude: 0, Longitude: 0, AccuracyMeter: 10000}
	observation.Position = position
	observation.Position.Longitude = 0.001
	policy.MaxReportedAccuracyMeter = 10000
	if _, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy); !errors.Is(err, ErrObservationMismatch) {
		t.Fatal("coarse accuracy widened match tolerance")
	}
	observation.Position.Longitude = 180
	if distance := surfaceDistanceUpperBound(position, observation.Position); math.IsNaN(distance) || distance < 20000000 {
		t.Fatal("antipodal distance invalid")
	}
	for _, delta := range []float64{-0.001, 0.001} {
		observation.Position = position
		observation.Position.Longitude = (policy.MaxDistanceMeter + delta) / 6400000 * 180 / math.Pi
		_, err := CheckOverrideObservation(position, SourceSatellite, start, now, observation, policy)
		if (err == nil) != (delta < 0) {
			t.Fatal("distance threshold mismatch", err)
		}
	}
}
