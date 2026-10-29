package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/detection"
	"github.com/Maoyangui/godusevpn/internal/identity"
	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/settings"
)

type fakePrivacyHelper struct {
	calls                    []string
	failMutation, failReport bool
	closed                   bool
}

func (h *fakePrivacyHelper) Call(_ context.Context, method string, _, _ any) error {
	h.calls = append(h.calls, method)
	if method == "Report" && h.failReport {
		return errors.New("synthetic lost transport")
	}
	if method != "Report" && h.failMutation {
		return errors.New("synthetic mutation result unknown")
	}
	return nil
}
func (h *fakePrivacyHelper) Close() error { h.closed = true; return nil }

func TestPrivacyAuthorizationIsSingleFlightAndExplicitlyRetryable(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	entered, finish := make(chan struct{}), make(chan struct{})
	var launches atomic.Int32
	h := &fakePrivacyHelper{}
	launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) {
		launches.Add(1)
		close(entered)
		<-finish
		return h, nil
	}
	a := &App{ctx: context.Background()}
	done := make(chan error, 1)
	go func() { _, err := a.PrivacyAuthorize(); done <- err }()
	<-entered
	if state := a.PrivacyHelperStatus(); state.State != "authorizing" {
		t.Fatal(state)
	}
	if _, err := a.PrivacyAuthorize(); err == nil || launches.Load() != 1 {
		t.Fatal("duplicate launch was accepted")
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state := a.PrivacyHelperStatus(); state.State != "connected" {
		t.Fatal(state)
	}
	if _, err := a.PrivacyAuthorize(); err != nil || launches.Load() != 1 {
		t.Fatal("connected helper relaunched")
	}
	launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) {
		launches.Add(1)
		return nil, errors.New("synthetic cancellation")
	}
	b := &App{ctx: context.Background()}
	if _, err := b.PrivacyAuthorize(); err == nil || b.PrivacyHelperStatus().State != "cancelled_or_unavailable" {
		t.Fatal("cancelled authorization claimed connected")
	}
	launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) {
		launches.Add(1)
		return &fakePrivacyHelper{}, nil
	}
	if _, err := b.PrivacyAuthorize(); err != nil || b.PrivacyHelperStatus().State != "connected" {
		t.Fatal("explicit retry failed", err)
	}
}

func TestPrivacyFailedWriteIsNeverRetriedAndLostHelperIsCleared(t *testing.T) {
	for _, lost := range []bool{false, true} {
		h := &fakePrivacyHelper{failMutation: true, failReport: lost}
		a := &App{ctx: context.Background(), privacyHost: h, privacyHostState: "connected"}
		if err := a.hostCall("Apply", "fixture", nil); err == nil {
			t.Fatal("write failure hidden")
		}
		if strings.Join(h.calls, ",") != "Apply,Report" {
			t.Fatal("write retried or status probe missing", h.calls)
		}
		if lost && (a.privacyHost != nil || !h.closed || a.PrivacyHelperStatus().State != "lost") {
			t.Fatal("lost helper remains usable")
		}
		if !lost && (a.privacyHost != h || h.closed) {
			t.Fatal("ordinary policy failure closed healthy helper")
		}
	}
}

func TestPrivacyAuthorizationCompletionInvalidatesInterimEvidenceAndClosedUI(t *testing.T) {
	if !identity.Ultra {
		t.Skip("Ultra helper is Windows-only")
	}
	old := launchPrivacyHelper
	t.Cleanup(func() { launchPrivacyHelper = old })
	for _, closed := range []bool{false, true} {
		entered, finish := make(chan struct{}), make(chan struct{})
		h := &fakePrivacyHelper{}
		launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { close(entered); <-finish; return h, nil }
		a := &App{ctx: context.Background()}
		done := make(chan error, 1)
		go func() { _, err := a.PrivacyAuthorize(); done <- err }()
		<-entered
		a.privacyMu.Lock()
		previous := a.privacyEvidenceEpoch
		a.privacySnapshot = fixturePrivacySnapshot().privacySnapshot
		if closed {
			a.privacyHostState = "closed"
			a.privacyHostEpoch++
			a.privacySnapshot = nil
		}
		a.privacyMu.Unlock()
		close(finish)
		err := <-done
		if closed {
			if err == nil || !h.closed || a.privacyHost != nil || a.PrivacyHelperStatus().State != "closed" {
				t.Fatal("closed UI accepted late helper")
			}
		} else if err != nil || a.privacySnapshot != nil || a.privacyEvidenceEpoch <= previous {
			t.Fatal("authorization completion kept interim evidence", err)
		}
	}
	launchPrivacyHelper = func(context.Context, string) (privacyHelper, error) { return nil, syscall.Errno(1223) }
	a := &App{ctx: context.Background()}
	if _, err := a.PrivacyAuthorize(); err == nil || a.PrivacyHelperStatus().State != "cancelled" {
		t.Fatal("known UAC cancellation not distinguished")
	}
}

func fixturePrivacySnapshot() *App {
	return &App{privacySnapshot: &privacyDetectionSnapshot{id: "source-fixture", expires: time.Now().Add(time.Minute), report: detection.Report{GeneratedAt: "fixture-time", Observer: "fixture-observer", Findings: []detection.Finding{{ID: "timezone", Summary: "private-zone"}, {ID: "locale", Summary: "private-locale"}, {ID: "fixture-rule", Summary: "bounded policy evidence"}}, Warnings: []string{"fixture scope"}}}}
}

func TestPrivacyExportUsesBackendSelectionAndExactPreviewBytes(t *testing.T) {
	a := fixturePrivacySnapshot()
	p, err := a.PrivacyPreviewDetectionExport(PrivacyExportSelection{SnapshotID: "source-fixture", Scope: "filtered", FindingIDs: []string{"timezone", "locale", "fixture-rule"}})
	if err != nil || p.Count != 1 || strings.Contains(p.Content, "private-zone") || strings.Contains(p.Content, "private-locale") || strings.Contains(p.Content, "fixture-observer") {
		t.Fatal("unselected fields exported", p, err)
	}
	if _, err := a.PrivacyPreviewDetectionExport(PrivacyExportSelection{SnapshotID: "source-fixture", Scope: "filtered", FindingIDs: []string{"foreign-id"}}); err == nil {
		t.Fatal("frontend supplied a foreign finding")
	}
	oldChoose, oldSave := choosePrivacyExportDestination, savePrivacyExport
	t.Cleanup(func() { choosePrivacyExportDestination, savePrivacyExport = oldChoose, oldSave })
	choosePrivacyExportDestination = func(context.Context) (string, error) {
		a.privacySnapshot.report.Findings[2].Summary = "later evidence"
		return "synthetic-output", nil
	}
	var saved []byte
	savePrivacyExport = func(_ string, b []byte) error { saved = append([]byte{}, b...); return nil }
	if _, err := a.PrivacyExportDetection(p.PreviewID); err != nil || string(saved) != p.Content {
		t.Fatal("saved bytes differ from reviewed preview", err)
	}
	if _, err := a.PrivacyExportDetection(p.PreviewID); err == nil {
		t.Fatal("consumed preview token reused")
	}
}

func TestPrivacyExportCancellationExpirationAndInvalidationDoNotWrite(t *testing.T) {
	oldChoose, oldSave := choosePrivacyExportDestination, savePrivacyExport
	t.Cleanup(func() { choosePrivacyExportDestination, savePrivacyExport = oldChoose, oldSave })
	writes := 0
	savePrivacyExport = func(string, []byte) error { writes++; return nil }
	for _, mode := range []string{"cancel", "expired", "invalidated"} {
		a := fixturePrivacySnapshot()
		p, err := a.PrivacyPreviewDetectionExport(PrivacyExportSelection{SnapshotID: "source-fixture", Scope: "all"})
		if err != nil {
			t.Fatal(err)
		}
		choosePrivacyExportDestination = func(context.Context) (string, error) {
			if mode == "cancel" {
				return "", nil
			}
			if mode == "invalidated" {
				a.invalidatePrivacyEvidence()
			}
			return "synthetic-output", nil
		}
		if mode == "expired" {
			a.privacyExport.expires = time.Now().Add(-time.Second)
		}
		_, err = a.PrivacyExportDetection(p.PreviewID)
		if mode != "cancel" && err == nil {
			t.Fatal("stale preview accepted", mode)
		}
		if writes != 0 {
			t.Fatal("cancelled/stale export wrote output")
		}
	}
}

func TestPrivacyNetworkReadDoesNotReusePriorServiceSnapshot(t *testing.T) {
	old := privacyServiceState
	t.Cleanup(func() { privacyServiceState = old })
	failed := false
	privacyServiceState = func(ctx context.Context) (ipc.StateView, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded service read")
		}
		if failed {
			return ipc.StateView{}, errors.New("synthetic unavailable")
		}
		return ipc.StateView{Mode: "global", Guard: "on"}, nil
	}
	n, at := readPrivacyNetwork(context.Background())
	if !n.Available || n.Mode != "global" || at == "" {
		t.Fatal("fresh authenticated read not represented", n, at)
	}
	failed = true
	n, at = readPrivacyNetwork(context.Background())
	if n.Available || n.Mode != "" || n.Guard != "" || at != "" {
		t.Fatal("failed read reused old service evidence", n, at)
	}
}

func TestPrivacyNetworkSummaryDoesNotCopyDNSAddressesOrUnknownSettings(t *testing.T) {
	old := privacyServiceState
	t.Cleanup(func() { privacyServiceState = old })
	v := ipc.StateView{Settings: settings.Settings{Schema: 1, LocalDNS: "https://resolver.fixture.invalid/query", RemoteDNS: "https://remote.fixture.invalid/query", LogLevel: "warn", LogDays: 0}}
	privacyServiceState = func(context.Context) (ipc.StateView, error) { return v, nil }
	n, _ := readPrivacyNetwork(context.Background())
	if !n.SettingsAvailable || n.LocalDNSMode != "doh" || n.RemoteDNSMode != "doh" || n.LogLevel != "warn" || n.LogDays == nil || *n.LogDays != 0 {
		t.Fatal("safe settings summary missing", n)
	}
	v.Settings.Schema = 0
	n, _ = readPrivacyNetwork(context.Background())
	if n.SettingsAvailable || n.LocalDNSMode != "" || n.RemoteDNSMode != "" || n.LogLevel != "" || n.LogDays != nil {
		t.Fatal("missing settings schema became usable configuration", n)
	}
}

func TestPrivacyExportProductionSaveIsPrivateLocalAndNoOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewed.json")
	content := []byte("{\"synthetic\":true}")
	if err := savePrivacyExport(path, content); err != nil {
		t.Fatal("production local export save failed", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != string(content) {
		t.Fatal("production save changed preview", err)
	}
	if err := savePrivacyExport(path, []byte("replace")); err == nil {
		t.Fatal("production export overwrote existing file")
	}
	if b, _ := os.ReadFile(path); string(b) != string(content) {
		t.Fatal("existing report content changed")
	}
}
