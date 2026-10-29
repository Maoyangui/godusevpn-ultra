package main

import (
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

func TestEngineSummaryLaunchAffordanceMatchesVerifiedAcceptanceBuild(t *testing.T) {
	item := browserengine.Installed{
		Receipt: browserengine.Receipt{
			Release:     browserengine.Release{EngineID: browserengine.Chrome, Version: "156.0.8078.47", Source: "test"},
			Files:       []browserengine.TreeFile{{Name: "chrome.exe", Size: 1, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
			InstalledAt: time.Now().UTC(),
		},
		TreeVerified: true, VersionVerified: true,
	}
	got := engineSummary(item)
	want := browserenginehost.NativeLaunchEnabled()
	if got.CanLaunch != want {
		t.Fatalf("CanLaunch=%v, want %v for this platform build", got.CanLaunch, want)
	}
	if got.EngineID != browserengine.Chrome || got.Version != item.Receipt.Release.Version {
		t.Fatalf("summary identity was not preserved: %+v", got)
	}
}
