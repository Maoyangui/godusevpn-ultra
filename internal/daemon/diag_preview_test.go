package daemon

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/logx"
	"github.com/Maoyangui/godusevpn/internal/paths"
	"github.com/Maoyangui/godusevpn/internal/settings"
	"github.com/Maoyangui/godusevpn/internal/state"
)

func syntheticDiagnosticDaemon(t *testing.T) *Daemon {
	d := &Daemon{settings: settings.Default(), settingsOK: true, persistedOK: true, machine: state.New(state.Deps{}), server: ipc.NewServer(nil)}
	t.Cleanup(d.clearDiagnosticCache)
	return d
}

func unzipDiagnostic(t *testing.T, data []byte) map[string]string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

func TestDiagnosticMinimalAllowlistDropsMachineNetworkAndFreeText(t *testing.T) {
	s := settings.Default()
	poison := "SYNTHETIC-HOST 192.0.2.77 2001:db8::42 AA-BB-CC-DD-EE-01 C:\\Users\\Fixture\\secret fixture.invalid"
	s.Selected, s.ActiveProfile, s.ProfileURL, s.RemoteDNS, s.LocalDNS = poison, poison, poison, poison, poison
	s.Profiles = []settings.Profile{{ID: poison, Name: poison, URL: poison}}
	s.BypassApps, s.LANSubnets = []string{poison}, []string{poison}
	s.Mode = poison
	snap := state.Snapshot{Status: state.Status(poison), Code: poison, Error: poison, Since: 123456789, Retries: 456789}
	b, err := json.Marshal(minimalDiagnostic(s, snap, true, true, true, true))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SYNTHETIC-HOST", "192.0.2.77", "2001:db8", "AA-BB", "Fixture", "fixture.invalid", "123456789", "456789", "profiles", "dataDir", "serviceLog", "coreLog"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("minimal diagnostic included %q", forbidden)
		}
	}
	if !bytes.Contains(b, []byte("E_UNKNOWN")) || !bytes.Contains(b, []byte("minimal_allowlist")) {
		t.Fatal("fixed status/unknown evidence missing")
	}
}

func TestDiagnosticDefaultNeverInvokesSensitiveCollectorsAndPreviewMatchesArchive(t *testing.T) {
	panicSource := func(func(string, string), func(string)) { t.Fatal("default diagnostic touched a sensitive source") }
	entry, err := buildDiagnostic(ipc.DiagnosticOptions{}, map[string]bool{"synthetic": true}, diagnosticCollectors{panicSource, panicSource, panicSource, panicSource}, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	files := unzipDiagnostic(t, entry.archive.Data)
	if len(files) != 2 || files["summary.json"] == "" || files["scope.json"] == "" {
		t.Fatal("minimal archive contains unexpected files")
	}
	for _, f := range entry.preview.Files {
		if f.Content != files[f.Name] || f.Bytes != len(f.Content) {
			t.Fatal("preview differs from actual archive")
		}
	}
	sum := sha256.Sum256(entry.archive.Data)
	if entry.preview.SHA256 != hex.EncodeToString(sum[:]) || entry.archive.SHA256 != entry.preview.SHA256 {
		t.Fatal("preview hash does not describe exact zip bytes")
	}
}

func TestDiagnosticRequiresExplicitSelectionAndAcknowledgment(t *testing.T) {
	for _, raw := range []string{`{"network":true}`, `{"logs":true}`, `{"config":true}`, `{"crashes":true}`, `{"unknown":true}`, `{"network":"yes"}`, `{} {}`} {
		if _, err := diagnosticOptions(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted unconfirmed or malformed request %s", raw)
		}
	}
	called := 0
	collect := func(add func(string, string), warn func(string)) {
		called++
		add("network.txt", "synthetic network fixture")
	}
	if _, err := buildDiagnostic(ipc.DiagnosticOptions{Network: true}, nil, diagnosticCollectors{network: collect}, time.Now()); err == nil || called != 0 {
		t.Fatal("missing acknowledgement reached collection")
	}
	opts, err := diagnosticOptions(json.RawMessage(`{"logs":true,"acknowledgeSensitive":true}`))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := buildDiagnostic(opts, true, diagnosticCollectors{logs: collect, network: func(func(string, string), func(string)) { t.Fatal("unselected network source") }}, time.Now())
	if err != nil || called != 1 || len(entry.preview.Warnings) < 2 {
		t.Fatal("explicit source selection/evidence failed", err)
	}
}

func TestDiagnosticBoundsTextArchiveAndCommandOutputWithoutRunningCommand(t *testing.T) {
	large := strings.Repeat("合成\xff", ipc.DiagnosticMaxFileBytes)
	collect := func(add func(string, string), warn func(string)) {
		for i := 0; i < 20; i++ {
			add("synthetic.txt", large)
		}
	}
	entry, err := buildDiagnostic(ipc.DiagnosticOptions{Logs: true, AcknowledgeSensitive: true}, true, diagnosticCollectors{logs: collect}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, file := range entry.preview.Files {
		if len(file.Content) > ipc.DiagnosticMaxFileBytes || !utf8.ValidString(file.Content) {
			t.Fatal("unbounded/invalid text")
		}
		total += len(file.Content)
	}
	if total > ipc.DiagnosticMaxTextBytes || len(entry.archive.Data) > ipc.DiagnosticMaxZIPBytes {
		t.Fatal("aggregate bounds exceeded")
	}
	var bounded limitedDiagnosticBuffer
	if _, err := bounded.Write(make([]byte, ipc.DiagnosticMaxZIPBytes+1)); err == nil || bounded.Len() != 0 {
		t.Fatal("zip writer does not enforce bound")
	}
	var command diagnosticCommandBuffer
	p := make([]byte, ipc.DiagnosticMaxFileBytes+1)
	if n, err := command.Write(p); err != nil || n != len(p) || !command.truncated || len(command.data) != ipc.DiagnosticMaxFileBytes {
		t.Fatal("command output not safely bounded")
	}
}

func TestDiagnosticCacheIsBoundedImmutableAndNeverRecollectsExpiredExport(t *testing.T) {
	d := syntheticDiagnosticDaemon(t)
	p, err := d.previewDiag(ipc.DiagnosticOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p.Files[0].Content = "caller mutation"
	a, err := d.exportPreview(p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a.Data[0] ^= 1
	again, err := d.exportPreview(p.ID, time.Now())
	if err != nil || bytes.Equal(a.Data, again.Data) || strings.Contains(unzipDiagnostic(t, again.Data)["summary.json"], "caller mutation") {
		t.Fatal("caller could mutate cached snapshot", err)
	}
	if _, err := d.exportPreview(p.ID, time.Now().Add(diagnosticTTL+time.Second)); err == nil || len(d.diagCache) != 0 {
		t.Fatal("expired export silently recollected")
	}
	first, _ := d.previewDiag(ipc.DiagnosticOptions{})
	_, _ = d.previewDiag(ipc.DiagnosticOptions{})
	_, _ = d.previewDiag(ipc.DiagnosticOptions{})
	if len(d.diagCache) != 2 {
		t.Fatal("preview cache grew past two entries")
	}
	if _, err := d.exportPreview(first.ID, time.Now()); err == nil {
		t.Fatal("evicted preview exported a different/new capture")
	}
	d.expireDiagnosticCache(time.Now().Add(diagnosticTTL + time.Second))
	if len(d.diagCache) != 0 || d.diagTimer != nil {
		t.Fatal("scheduled expiry did not release cached content")
	}
	d.clearDiagnosticCache()
	if _, err := d.previewDiag(ipc.DiagnosticOptions{}); err == nil {
		t.Fatal("closed service recollected diagnostics")
	}
}

func TestDiagnosticIPCDefaultsNeedNoLogCoreOrNetworkObjects(t *testing.T) {
	d := syntheticDiagnosticDaemon(t) // no core, log writer or real network backend
	d.registerHandlers()
	for _, method := range []string{ipc.MDiagnose, ipc.MPreviewDiag, ipc.MExportDiag} {
		if _, err := d.Dispatch(method, nil); err != nil {
			t.Fatalf("minimal %s accessed absent sensitive dependency: %v", method, err)
		}
	}
	if _, err := d.Dispatch(ipc.MPreviewDiag, json.RawMessage(`{"network":true}`)); err == nil {
		t.Fatal("IPC collected unconfirmed detail")
	}
	if _, err := d.Dispatch(ipc.MExportDiag, json.RawMessage(`{"previewId":"expired-fixture"}`)); err == nil {
		t.Fatal("invalid preview id caused a new capture")
	}
}

func TestDiagnosticDetailedProductionReadersUseOnlySyntheticTemporaryFiles(t *testing.T) {
	t.Setenv(paths.DataEnvironment, t.TempDir())
	d := syntheticDiagnosticDaemon(t)
	if err := os.MkdirAll(paths.Logs(), 0700); err != nil {
		t.Fatal(err)
	}
	d.log, d.coreLog = logx.New(filepath.Join(paths.Logs(), "service.log"), 1024, 1), logx.New(filepath.Join(paths.Logs(), "core.log"), 1024, 1)
	for _, file := range []struct{ path, text string }{
		{d.log.Path(), "SYNTHETIC-HOST 192.0.2.77 password=fixture-secret"},
		{d.coreLog.Path(), "synthetic core error"},
		{paths.Config(), `{"server":"192.0.2.77","password":"fixture-secret"}`},
		{filepath.Join(paths.Logs(), "crash.log"), "== fixture start\npanic: synthetic crash reason\nfixture stack"},
	} {
		if err := os.WriteFile(file.path, []byte(file.text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := d.previewDiag(ipc.DiagnosticOptions{Logs: true, Config: true, Crashes: true, AcknowledgeSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	a, err := d.exportPreview(p.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	files := unzipDiagnostic(t, a.Data)
	if !strings.Contains(files["service.log"], "SYNTHETIC-HOST") || !strings.Contains(files["crash.log"], "synthetic crash reason") || files["config.redacted.json"] == "" || files["state.redacted.json"] == "" {
		t.Fatal("opted-in troubleshooting evidence was lost")
	}
	for _, text := range files {
		if strings.Contains(text, "fixture-secret") {
			t.Fatal("common credential was retained")
		}
	}
	if err := os.WriteFile(d.log.Path(), []byte("NEW-SYNTHETIC-RECORD"), 0600); err != nil {
		t.Fatal(err)
	}
	stable, err := d.exportPreview(p.ID, time.Now())
	if err != nil || !bytes.Equal(stable.Data, a.Data) {
		t.Fatal("export reread changing files after preview", err)
	}
	if entries, err := os.ReadDir(paths.DataDir()); err != nil || len(entries) != 2 { // config + logs, no service ZIP/diag directory
		t.Fatal("diagnostic export unexpectedly created a service artifact", err)
	}
}
