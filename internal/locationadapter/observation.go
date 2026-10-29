package locationadapter

import (
	"errors"
	"math"
	"time"
)

// PositionSource uses the documented Windows.Devices.Geolocation values. A
// reader must obtain these from the actual Geocoordinate, not infer them from
// an IP lookup, requested coordinate, cached config or provider's own receipt.
type PositionSource int

const (
	SourceCellular PositionSource = iota
	SourceSatellite
	SourceWiFi
	SourceIPAddress
	SourceUnknown
	SourceDefault
	SourceObfuscated
)

// SystemObservation contains private data from an independent Geolocator
// observation. It intentionally has no JSON representation or useful default
// formatting. It is distinct from Snapshot, whose stable Raw ownership data is
// used for configuration transactions. This type does not authenticate a reader.
type SystemObservation struct {
	Position          Position       `json:"-"`
	Timestamp         time.Time      `json:"-"`
	Source            PositionSource `json:"-"`
	SourceKnown       bool           `json:"-"`
	IsRemoteSource    bool           `json:"-"`
	RemoteSourceKnown bool           `json:"-"`
}

func (SystemObservation) String() string   { return "<private-system-location-observation>" }
func (SystemObservation) GoString() string { return "<private-system-location-observation>" }

// ObservationPolicy bounds acceptable evidence. A host selects this policy;
// it must not accept looser limits or "verified" flags from a frontend. Distance
// and quality limits are independent: a very coarse fix cannot widen the
// accepted distance to make a mismatching coordinate appear successful.
type ObservationPolicy struct {
	MaxAge                   time.Duration
	FutureClockSkew          time.Duration
	MaxReportedAccuracyMeter float64
	MaxDistanceMeter         float64
}

func DefaultObservationPolicy() ObservationPolicy {
	return ObservationPolicy{MaxAge: 10 * time.Second, FutureClockSkew: time.Second, MaxReportedAccuracyMeter: 100, MaxDistanceMeter: 5}
}

var (
	ErrObservationInput     = errors.New("location_observation_input_refused")
	ErrObservationStale     = errors.New("location_observation_not_fresh")
	ErrObservationFuture    = errors.New("location_observation_future_timestamp")
	ErrObservationSource    = errors.New("location_observation_source_unverified")
	ErrObservationPrecision = errors.New("location_observation_accuracy_insufficient")
	ErrObservationMismatch  = errors.New("location_observation_position_mismatch")
)

// ObservationCheck is safe for a minimal report: no coordinates, distance,
// source, accuracy or timestamp is copied into it. A match never establishes
// provider ownership, successful configuration mutation or global GPS coverage.
type ObservationCheck struct {
	Status                    string `json:"status"`
	Scope                     string `json:"scope"`
	MatchesRequestedPosition  bool   `json:"matchesRequestedPosition"`
	ProviderOwnershipVerified bool   `json:"providerOwnershipVerified"`
	Applied                   bool   `json:"applied"`
}

func finitePositive(value, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0 && value <= maximum
}

// CheckOverrideObservation validates evidence only. It does not request access,
// read a device, set an override, clear a session, or authorize AtomicBackend.
// notBefore is the host's timestamp for this specific successful Set/update;
// a prior matching cached fix must not verify a later override operation.
func CheckOverrideObservation(requested Position, source PositionSource, notBefore, now time.Time, observed SystemObservation, policy ObservationPolicy) (ObservationCheck, error) {
	result := ObservationCheck{Status: "observation_unverified", Scope: "windows-location-service/2d-request-match"}
	if requested.valid() != nil || observed.Position.valid() != nil || !finitePositive(requested.AccuracyMeter, 10000) || !finitePositive(observed.Position.AccuracyMeter, 10000) ||
		!finitePositive(policy.MaxReportedAccuracyMeter, 10000) || !finitePositive(policy.MaxDistanceMeter, 1000) ||
		policy.MaxAge <= 0 || policy.MaxAge > time.Minute || policy.FutureClockSkew < 0 || policy.FutureClockSkew > 2*time.Second ||
		now.IsZero() || notBefore.IsZero() || observed.Timestamp.IsZero() || notBefore.After(now) {
		return result, ErrObservationInput
	}
	if observed.Timestamp.Before(notBefore) || now.Sub(observed.Timestamp) > policy.MaxAge {
		return result, ErrObservationStale
	}
	if observed.Timestamp.After(now.Add(policy.FutureClockSkew)) {
		return result, ErrObservationFuture
	}
	// Unknown, manually configured default and deliberately obfuscated fixes do
	// not demonstrate this requested remote override. The real provider must
	// separately prove session ownership; IsRemoteSource alone is insufficient.
	if source < SourceCellular || source > SourceIPAddress || !observed.SourceKnown || observed.Source != source || !observed.RemoteSourceKnown || !observed.IsRemoteSource {
		return result, ErrObservationSource
	}
	if observed.Position.AccuracyMeter > requested.AccuracyMeter || observed.Position.AccuracyMeter > policy.MaxReportedAccuracyMeter {
		return result, ErrObservationPrecision
	}
	if surfaceDistanceUpperBound(requested, observed.Position) > policy.MaxDistanceMeter {
		return result, ErrObservationMismatch
	}
	result.Status = "observation_matches_request"
	result.MatchesRequestedPosition = true
	return result, nil
}

// A deliberately conservative spherical distance, not a precision geodesic or
// 3D/altitude match. 6,400,000 m is above the WGS84 maximum curvature radius; it
// avoids accepting extra distance through a mean-Earth-radius underestimate.
// Haversine handles the antimeridian/poles without raw longitude subtraction
// turning a nearby wrapped coordinate into a nearly full-world difference.
func surfaceDistanceUpperBound(a, b Position) float64 {
	const radians = math.Pi / 180
	latA, latB := a.Latitude*radians, b.Latitude*radians
	dLat, dLon := (b.Latitude-a.Latitude)*radians, (b.Longitude-a.Longitude)*radians
	sinLat, sinLon := math.Sin(dLat/2), math.Sin(dLon/2)
	h := sinLat*sinLat + math.Cos(latA)*math.Cos(latB)*sinLon*sinLon
	h = math.Max(0, math.Min(1, h))
	return 2 * 6400000 * math.Atan2(math.Sqrt(h), math.Sqrt(1-h))
}
