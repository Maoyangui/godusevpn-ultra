package main

import (
	"context"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"testing"
)

type fakeLocationPortal struct {
	reads, applies, restores, observes, stops, closes, connects int
	request                                                     locationportal.ApplyRequest
}

func (f *fakeLocationPortal) Connect(context.Context, locationportal.Config) (locationportal.Report, error) {
	f.connects++
	return locationportal.Report{Connected: true}, nil
}
func (f *fakeLocationPortal) Read(context.Context) (locationportal.Report, error) {
	f.reads++
	return locationportal.Report{Status: "fake"}, nil
}
func (f *fakeLocationPortal) Apply(_ context.Context, r locationportal.ApplyRequest) (locationportal.Report, error) {
	f.applies++
	f.request = r
	return locationportal.Report{ConfigurationVerified: true}, nil
}
func (f *fakeLocationPortal) Observe(context.Context) (locationportal.Report, error) {
	f.observes++
	return locationportal.Report{ConsumerVerified: true}, nil
}
func (f *fakeLocationPortal) Restore(context.Context) (locationportal.Report, error) {
	f.restores++
	return locationportal.Report{Status: "restored"}, nil
}
func (f *fakeLocationPortal) Stop()              { f.stops++ }
func (f *fakeLocationPortal) CancelObservation() {}
func (f *fakeLocationPortal) Close()             { f.closes++ }
func TestLocationPortalStrictBridgeAndLifecycle(t *testing.T) {
	f := &fakeLocationPortal{}
	a := &App{locationPortal: f}
	if _, e := a.PrivacyLocationPortalApply(`{"points":[{}],"intervalSeconds":0}`); e == nil || f.applies != 0 {
		t.Fatal("invalid draft reached backend")
	}
	if _, e := a.PrivacyLocationPortalApply(`{"points":[{"latitude":1,"longitude":2}],"intervalSeconds":0}`); e != nil || f.applies != 1 || a.privacyEvidenceEpoch != 2 {
		t.Fatal(e)
	}
	if _, e := a.PrivacyLocationPortalObserve(); e != nil || f.observes != 1 {
		t.Fatal(e)
	}
	if _, e := a.PrivacyLocationPortalRestore(); e != nil || f.restores != 1 || a.privacyEvidenceEpoch != 4 {
		t.Fatal(e)
	}
	if _, e := a.PrivacyLocationPortalStopTrajectory(); e != nil || f.stops != 1 || f.reads != 1 {
		t.Fatal(e)
	}
	a.shutdownLocationPortal()
	a.shutdownLocationPortal()
	if f.closes != 1 {
		t.Fatal("shutdown not idempotent")
	}
	if _, e := a.PrivacyLocationPortalReport(); e == nil {
		t.Fatal("closed manager reopened")
	}
}
func TestLocationPortalInvalidCredentialsNeverCreateManager(t *testing.T) {
	previous := newLocationPortalBackend
	defer func() { newLocationPortalBackend = previous }()
	created := 0
	newLocationPortalBackend = func() locationPortalBackend { created++; return &fakeLocationPortal{} }
	a := &App{}
	if _, e := a.PrivacyLocationPortalConnect(`{"port":58443,"certificateSHA256":"bad","username":"x","password":"y"}`); e == nil || created != 0 {
		t.Fatal("invalid input created controller")
	}
}
