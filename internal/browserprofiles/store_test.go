package browserprofiles

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserhost"
)

// All storage here is synthetic in-memory storage. It models write/delete
// sharing and handle deletion without calling New, NT, SCM or known folders.
type memoryEntry struct {
	data    []byte
	readers int
	writer  bool
}
type memoryStorage struct {
	mu         sync.Mutex
	entries    map[string]*memoryEntry
	exists     bool
	begins     []bool
	removed    []string
	writeLimit int
	failSync   bool
	afterWrite func()
}
type memoryDirectory struct{ fs *memoryStorage }
type memoryFile struct {
	fs            *memoryStorage
	entry         *memoryEntry
	name          string
	pos           int
	write, closed bool
}
type memoryInfo struct{ size int64 }

func (memoryInfo) Name() string       { return "opaque" }
func (i memoryInfo) Size() int64      { return i.size }
func (memoryInfo) Mode() os.FileMode  { return 0600 }
func (memoryInfo) ModTime() time.Time { return time.Time{} }
func (memoryInfo) IsDir() bool        { return false }
func (memoryInfo) Sys() any           { return nil }
func newMemory() *memoryStorage {
	return &memoryStorage{entries: map[string]*memoryEntry{}, writeLimit: -1}
}
func (s *memoryStorage) begin(create bool) (directory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.begins = append(s.begins, create)
	if !s.exists {
		if !create {
			return nil, os.ErrNotExist
		}
		s.exists = true
	}
	return &memoryDirectory{s}, nil
}
func (d *memoryDirectory) close() {}
func (d *memoryDirectory) names() ([]string, error) {
	d.fs.mu.Lock()
	defer d.fs.mu.Unlock()
	var names []string
	for name := range d.fs.entries {
		names = append(names, name)
	}
	return names, nil
}
func (d *memoryDirectory) open(name string, write, create bool) (recordFile, error) {
	s := d.fs
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found := s.entries[name]
	if create {
		if found {
			return nil, ErrConflict
		}
		entry = &memoryEntry{}
		s.entries[name] = entry
	} else if !found {
		return nil, os.ErrNotExist
	}
	if entry.writer || write && entry.readers != 0 {
		return nil, ErrBusy
	}
	if write {
		entry.writer = true
	} else {
		entry.readers++
	}
	return &memoryFile{fs: s, entry: entry, name: name, write: write}, nil
}
func (d *memoryDirectory) remove(file recordFile) error {
	f, ok := file.(*memoryFile)
	if !ok {
		return ErrStore
	}
	d.fs.mu.Lock()
	defer d.fs.mu.Unlock()
	if f.closed || !f.write || d.fs.entries[f.name] != f.entry {
		return ErrStore
	}
	delete(d.fs.entries, f.name)
	d.fs.removed = append(d.fs.removed, f.name)
	return nil
}
func (f *memoryFile) Stat() (os.FileInfo, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.closed {
		return nil, os.ErrClosed
	}
	return memoryInfo{int64(len(f.entry.data))}, nil
}
func (f *memoryFile) Read(p []byte) (int, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	if f.pos >= len(f.entry.data) {
		return 0, io.EOF
	}
	n := copy(p, f.entry.data[f.pos:])
	f.pos += n
	return n, nil
}
func (f *memoryFile) Write(p []byte) (int, error) {
	s := f.fs
	s.mu.Lock()
	if f.closed || !f.write {
		s.mu.Unlock()
		return 0, os.ErrPermission
	}
	n := len(p)
	if s.writeLimit >= 0 && n > s.writeLimit {
		n = s.writeLimit
	}
	end := f.pos + n
	if end > len(f.entry.data) {
		f.entry.data = append(f.entry.data, make([]byte, end-len(f.entry.data))...)
	}
	copy(f.entry.data[f.pos:], p[:n])
	f.pos = end
	hook := s.afterWrite
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	return n, nil
}
func (f *memoryFile) Seek(offset int64, whence int) (int64, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.closed {
		return 0, os.ErrClosed
	}
	next := offset
	switch whence {
	case io.SeekCurrent:
		next += int64(f.pos)
	case io.SeekEnd:
		next += int64(len(f.entry.data))
	case io.SeekStart:
	default:
		return 0, ErrStore
	}
	if next < 0 || next > maxJournal {
		return 0, ErrStore
	}
	f.pos = int(next)
	return next, nil
}
func (f *memoryFile) Sync() error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.closed || f.fs.failSync {
		return ErrStore
	}
	return nil
}
func (f *memoryFile) Close() error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if !f.closed {
		f.closed = true
		if f.write {
			f.entry.writer = false
		} else {
			f.entry.readers--
		}
	}
	return nil
}

type countingEntropy byte

func (c *countingEntropy) Read(p []byte) (int, error) {
	for i := range p {
		*c++
		p[i] = byte(*c)
	}
	return len(p), nil
}
func managerFixture() (*Manager, *memoryStorage) {
	fs := newMemory()
	m := newManager(fs)
	var entropy countingEntropy
	m.entropy = &entropy
	m.now = func() time.Time { return time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC) }
	return m, fs
}
func configFixture(t *testing.T) json.RawMessage {
	t.Helper()
	runtime := browserhost.RuntimeContext{Engine: browserhost.Engine{ID: "webview2", Available: true}}
	c, e := browserhost.DefaultProfile(runtime, "draft", browserhost.NetworkBinding{Mode: "tunnel", ReferenceID: "session-only"})
	if e != nil {
		t.Fatal(e)
	}
	c.Identity.Mode = "generated"
	b, _ := json.Marshal(c)
	var object map[string]json.RawMessage
	_ = json.Unmarshal(b, &object)
	delete(object, "binding")
	b, _ = json.Marshal(object)
	return b
}
func requestFixture(t *testing.T, name string, record *Record) []byte {
	t.Helper()
	request := SaveRequest{SchemaVersion: 1, Name: name, Config: configFixture(t), Engine: EngineSelection{EngineID: "webview2"}}
	if record != nil {
		request.ID = record.ID
		request.ExpectedRevision = record.Revision
		request.Config = record.Config
		request.Engine = record.Engine
	}
	b, e := json.Marshal(request)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func mustSave(t *testing.T, m *Manager, name string, record *Record) Record {
	t.Helper()
	r, e := m.Save(context.Background(), requestFixture(t, name, record))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func mutateJSON(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		t.Fatal("fixture JSON")
	}
	mutate(value)
	b, e := json.Marshal(value)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestCRUDRenameCASAndNoRuntimeBinding(t *testing.T) {
	m, fs := managerFixture()
	ctx := context.Background()
	list, e := m.List(ctx)
	if e != nil || len(list.Records) != 0 || fs.exists {
		t.Fatalf("list created store: %+v %v", list, e)
	}
	created := mustSave(t, m, "工作环境 · 上海", nil)
	if !idPattern.MatchString(created.ID) || !revisionPattern.MatchString(created.Revision) || created.Applied || !created.RequiresLaunchValidation || created.RecoveryNeeded {
		t.Fatalf("unsafe receipt: %+v", created)
	}
	if bytes.Contains(created.Config, []byte(`"binding"`)) || bytes.Contains(created.Config, []byte("session-only")) {
		t.Fatal("binding persisted")
	}
	var config browserhost.ProfileConfig
	_ = json.Unmarshal(created.Config, &config)
	if config.ProfileID != created.ID {
		t.Fatal("record ID not authoritative")
	}
	rawBefore := append([]byte{}, fs.entries[created.ID+extension].data...)
	loaded, e := m.Load(ctx, created.ID)
	if e != nil || loaded.Revision != created.Revision || loaded.Name != created.Name {
		t.Fatalf("load: %v", e)
	}
	renamed := mustSave(t, m, "重命名 / 设备 A", &created)
	if renamed.ID != created.ID || renamed.Revision == created.Revision || !bytes.Equal(fs.entries[created.ID+extension].data[:len(rawBefore)], rawBefore) {
		t.Fatal("rename overwrote previous commit")
	}
	if _, e = m.Save(ctx, requestFixture(t, "stale", &created)); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale save: %v", e)
	}
	if _, e = m.Delete(ctx, created.ID, created.Revision); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale delete: %v", e)
	}
	list, e = m.List(ctx)
	if e != nil || len(list.Records) != 1 || list.Records[0].Name != renamed.Name || list.Records[0].Revision != renamed.Revision || list.Applied {
		t.Fatal("summary mismatch")
	}
	second := mustSave(t, m, "另一独立记录", nil)
	deleted, e := m.Delete(ctx, renamed.ID, renamed.Revision)
	if e != nil || !deleted.Deleted || deleted.ID != renamed.ID || len(fs.removed) != 1 || fs.removed[0] != renamed.ID+extension {
		t.Fatalf("delete: %+v %v", deleted, e)
	}
	if _, e = m.Load(ctx, renamed.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	if _, e = m.Load(ctx, second.ID); e != nil {
		t.Fatal("delete affected another record")
	}
}

func TestSchemaV2MetaAndExtrasRoundTrip(t *testing.T) {
	m, _ := managerFixture()
	request := SaveRequest{
		SchemaVersion: 2,
		Name:          "v2 环境",
		Meta:          ProfileMeta{EnvironmentName: "工作环境", Group: "社媒", Remark: "只保存配置，不保存 Cookie 原文", Tags: []string{"工作", "稳定"}},
		Extras: ProfileExtras{
			Storage:     json.RawMessage(`{"cookieFormat":"json","merge":true,"cookieCount":2,"cookieSessionOnly":true}`),
			Tabs:        json.RawMessage(`{"urls":["https://example.com"],"startupMode":"urls"}`),
			Proxy:       json.RawMessage(`{"mode":"dynamic","type":"socks5","provider":"IP2Location","accountRef":"acct-1"}`),
			Fingerprint: json.RawMessage(`{"webrtc":"replace","timezone":"ip","geo":"forbidden","language":"custom","uiLanguage":"by-language","resolution":"ua","fonts":"default","noise":{"canvas":true},"webglMetadata":"custom","webglVendor":"Google Inc. (AMD)","webglRenderer":"ANGLE (AMD)","webgpu":"disabled","seed":"001122"}`),
			Advanced:    json.RawMessage(`{"cpu":"custom","ram":"real","deviceName":"real","mac":"real","dnt":"on","portScan":"enabled","hardwareAcceleration":"default","tls":"default","teamApps":"global","dataSync":"global","browserSettings":"global","randomFingerprint":true,"allowedPorts":[80,443]}`),
			Readback:    json.RawMessage(`{"state":"unknown","scope":"browser-profile","runtimeVerified":false,"warnings":["启动后读回"]}`),
		},
		Config: configFixture(t), Engine: EngineSelection{EngineID: "webview2"},
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := m.Save(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SchemaVersion != 2 || saved.Meta.EnvironmentName != "工作环境" || string(saved.Extras.Tabs) == "" {
		t.Fatalf("v2 metadata missing: %#v", saved)
	}
	loaded, err := m.Load(context.Background(), saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Meta.Group != "社媒" || !bytes.Contains(loaded.Extras.Fingerprint, []byte(`"webgpu":"disabled"`)) {
		t.Fatalf("v2 extras missing: %#v", loaded)
	}
	list, err := m.List(context.Background())
	if err != nil || len(list.Records) != 1 || list.Records[0].Meta.EnvironmentName != "工作环境" {
		t.Fatalf("v2 list missing: %#v %v", list, err)
	}
	bad := mutateJSON(t, raw, func(value map[string]any) {
		value["extras"].(map[string]any)["tabs"].(map[string]any)["unexpected"] = true
	})
	if _, err = m.Save(context.Background(), bad); !errors.Is(err, ErrInput) {
		t.Fatalf("unknown v2 extra accepted: %v", err)
	}
}
func TestRejectSecretsUnknownTrustAndUnusableDraftBeforeIO(t *testing.T) {
	base := requestFixture(t, "默认", nil)
	cases := map[string]func(map[string]any){
		"top binding":              func(v map[string]any) { v["binding"] = map[string]any{"mode": "tunnel"} },
		"subscription":             func(v map[string]any) { v["subscription"] = "do-not-store" },
		"proxy password":           func(v map[string]any) { v["proxyPassword"] = "do-not-store" },
		"service PID":              func(v map[string]any) { v["servicePid"] = 10 },
		"port":                     func(v map[string]any) { v["proxyPort"] = 8080 },
		"config binding even null": func(v map[string]any) { v["config"].(map[string]any)["binding"] = nil },
		"config service PID":       func(v map[string]any) { v["config"].(map[string]any)["servicePid"] = 1 },
		"config arbitrary args":    func(v map[string]any) { v["config"].(map[string]any)["arguments"] = []string{"--no-sandbox"} },
		"identity client hints": func(v map[string]any) {
			v["config"].(map[string]any)["identity"] = map[string]any{"mode": "generated", "clientHints": nil}
		},
		"observed trust": func(v map[string]any) {
			v["config"].(map[string]any)["identity"] = map[string]any{"mode": "observed-exact"}
		},
		"manual CRLF": func(v map[string]any) {
			v["config"].(map[string]any)["identity"] = map[string]any{"mode": "manual", "userAgent": "agent\r\nSecret: yes"}
		},
		"manual blank": func(v map[string]any) {
			v["config"].(map[string]any)["identity"] = map[string]any{"mode": "manual", "userAgent": "   "}
		},
		"unsupported canvas": func(v map[string]any) { v["config"].(map[string]any)["canvas"] = "spoof" },
		"unsupported permission": func(v map[string]any) {
			v["config"].(map[string]any)["permissions"].(map[string]any)["camera"] = "allow"
		},
		"unsupported viewport": func(v map[string]any) { v["config"].(map[string]any)["viewport"].(map[string]any)["width"] = 1 },
		"engine path": func(v map[string]any) {
			v["engine"] = map[string]any{"engineId": "webview2", "version": "", "path": "C:\\evil.exe"}
		},
		"unknown engine":                 func(v map[string]any) { v["engine"] = map[string]any{"engineId": "arbitrary", "version": "1.2.3.4"} },
		"runtime version not selectable": func(v map[string]any) { v["engine"] = map[string]any{"engineId": "webview2", "version": "143.0.1.2"} },
		"bad chrome version": func(v map[string]any) {
			v["engine"] = map[string]any{"engineId": "chrome-for-testing", "version": "latest"}
		},
		"empty name": func(v map[string]any) { v["name"] = "" }, "control name": func(v map[string]any) { v["name"] = "设备\x00" }, "bidi name": func(v map[string]any) { v["name"] = "设备\u202e" }, "long name": func(v map[string]any) { v["name"] = strings.Repeat("中", 81) },
		"revision without id":    func(v map[string]any) { v["expectedRevision"] = strings.Repeat("a", 64) },
		"id without revision":    func(v map[string]any) { v["id"] = "p-" + strings.Repeat("a", 32) },
		"null identity pair":     func(v map[string]any) { v["id"] = nil; v["expectedRevision"] = nil },
		"empty identity pair":    func(v map[string]any) { v["id"] = ""; v["expectedRevision"] = "" },
		"null engine version":    func(v map[string]any) { v["engine"] = map[string]any{"engineId": "webview2", "version": nil} },
		"missing engine version": func(v map[string]any) { v["engine"] = map[string]any{"engineId": "webview2"} },
		"path id":                func(v map[string]any) { v["id"] = "../other"; v["expectedRevision"] = strings.Repeat("a", 64) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m, fs := managerFixture()
			_, e := m.Save(context.Background(), mutateJSON(t, base, mutate))
			if !errors.Is(e, ErrInput) || len(fs.begins) != 0 {
				t.Fatalf("not refused before I/O: %v", e)
			}
		})
	}
	malformed := []string{`null`, `[]`, `{}`, string(base) + ` {}`, strings.Replace(string(base), `"name":"默认"`, `"name":"默认","Name":"别名"`, 1), strings.Replace(string(base), `"mode":"generated"`, `"mode":"generated","mode":"native"`, 1), strings.Repeat(" ", 64<<10) + string(base)}
	for i, data := range malformed {
		m, fs := managerFixture()
		if _, e := m.Save(context.Background(), []byte(data)); !errors.Is(e, ErrInput) || len(fs.begins) != 0 {
			t.Fatalf("malformed %d: %v", i, e)
		}
	}
}
func TestValidIdentityEnvironmentAndEngineSelections(t *testing.T) {
	for _, mode := range []string{"native", "generated", "manual"} {
		for _, engine := range []EngineSelection{{"webview2", ""}, {"chrome-for-testing", "143.0.7499.192"}, {"firefox", "145.0.1"}, {"firefox", "140.5.0esr"}} {
			t.Run(mode+engine.EngineID+engine.Version, func(t *testing.T) {
				m, _ := managerFixture()
				data := mutateJSON(t, requestFixture(t, "手机 Android", nil), func(v map[string]any) {
					v["engine"] = map[string]any{"engineId": engine.EngineID, "version": engine.Version}
					c := v["config"].(map[string]any)
					identity := map[string]any{"mode": mode}
					if mode == "manual" {
						identity["userAgent"] = "  User-picked/1.0 Windows NT 10.0  "
					}
					c["identity"] = identity
					c["environment"] = map[string]any{"mode": "browser-emulation", "os": "android", "devicePresetId": "pixel-7", "osVersion": "17.2"}
					c["viewport"] = map[string]any{"width": 412, "height": 915}
				})
				r, e := m.Save(context.Background(), data)
				if e != nil {
					t.Fatal(e)
				}
				if mode == "manual" && !bytes.Contains(r.Config, []byte("  User-picked/1.0 Windows NT 10.0  ")) {
					t.Fatal("manual UA changed")
				}
				if r.Engine != engine {
					t.Fatal("selection changed")
				}
			})
		}
	}
}
func TestEveryTornCommitRetainsPriorRecordAndRequiresExplicitRecovery(t *testing.T) {
	m, fs := managerFixture()
	old := mustSave(t, m, "原记录", nil)
	before := append([]byte{}, fs.entries[old.ID+extension].data...)
	newRecord := mustSave(t, m, "新记录", &old)
	full := append([]byte{}, fs.entries[old.ID+extension].data...)
	frame := full[len(before):]
	for cut := 1; cut < len(frame); cut++ {
		fs.entries[old.ID+extension].data = append(append([]byte{}, before...), frame[:cut]...)
		loaded, e := m.Load(context.Background(), old.ID)
		if e != nil || loaded.Name != old.Name || !loaded.RecoveryNeeded || loaded.Revision == old.Revision || loaded.Revision == newRecord.Revision {
			t.Fatalf("cut %d: %+v %v", cut, loaded, e)
		}
		if _, e = m.Save(context.Background(), requestFixture(t, "overwrite", &loaded)); !errors.Is(e, ErrStore) {
			t.Fatalf("recovery overwrite %d: %v", cut, e)
		}
	}
	loaded, e := m.Load(context.Background(), old.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Delete(context.Background(), old.ID, loaded.Revision); e != nil {
		t.Fatal("exact partial record cannot be explicitly deleted")
	}
}
func TestFailedCreateIsDeletedByItsOwnHandleAndPartialUpdateNeverOverwrites(t *testing.T) {
	for _, limit := range []int{0, 1, len(journalMagic) - 1, len(journalMagic), 80} {
		t.Run("short-write-"+string(rune('a'+limit)), func(t *testing.T) {
			m, fs := managerFixture()
			fs.writeLimit = limit
			if _, e := m.Save(context.Background(), requestFixture(t, "fresh", nil)); !errors.Is(e, ErrStore) || len(fs.entries) != 0 || len(fs.removed) != 1 {
				t.Fatalf("failed fresh persisted: %v %+v", e, fs)
			}
		})
	}
	m, fs := managerFixture()
	fs.failSync = true
	if _, e := m.Save(context.Background(), requestFixture(t, "fresh", nil)); !errors.Is(e, ErrStore) || len(fs.entries) != 0 {
		t.Fatal("sync failure leaked new record")
	}
	m, fs = managerFixture()
	old := mustSave(t, m, "original", nil)
	before := append([]byte{}, fs.entries[old.ID+extension].data...)
	fs.writeLimit = 7
	if _, e := m.Save(context.Background(), requestFixture(t, "new", &old)); !errors.Is(e, ErrStore) {
		t.Fatal(e)
	}
	after := fs.entries[old.ID+extension].data
	if !bytes.HasPrefix(after, before) {
		t.Fatal("existing commit overwritten")
	}
	loaded, e := m.Load(context.Background(), old.ID)
	if e != nil || loaded.Name != old.Name || !loaded.RecoveryNeeded {
		t.Fatal("prior commit not recoverable")
	}
}
func TestCancellationCloseAndForeignRevision(t *testing.T) {
	m, fs := managerFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := m.Save(ctx, requestFixture(t, "cancelled", nil)); !errors.Is(e, context.Canceled) || len(fs.begins) != 0 {
		t.Fatal("cancel touched storage")
	}
	if _, e := m.Load(context.Background(), "../../outside"); !errors.Is(e, ErrInput) || len(fs.begins) != 0 {
		t.Fatal("path accepted")
	}
	if _, e := m.Delete(context.Background(), "bad", strings.Repeat("a", 64)); !errors.Is(e, ErrInput) {
		t.Fatal(e)
	}
	old := mustSave(t, m, "first", nil)
	second := newManager(fs)
	var entropy countingEntropy = 90
	second.entropy = &entropy
	changed := mustSave(t, second, "other window", &old)
	if _, e := m.Save(context.Background(), requestFixture(t, "stale", &old)); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	dir, e := fs.begin(false)
	if e != nil {
		t.Fatal(e)
	}
	held, e := dir.open(old.ID+extension, false, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = m.Save(context.Background(), requestFixture(t, "held", &changed)); !errors.Is(e, ErrBusy) {
		t.Fatalf("writer shared another read: %v", e)
	}
	_ = held.Close()
	ctx, cancel = context.WithCancel(context.Background())
	fs.afterWrite = cancel
	if _, e = m.Save(ctx, requestFixture(t, "cancel during write", &changed)); !errors.Is(e, context.Canceled) {
		t.Fatalf("cancel lost: %v", e)
	}
	fs.afterWrite = nil
	loaded, e := m.Load(context.Background(), old.ID)
	if e != nil || loaded.Name != "cancel during write" {
		t.Fatal("durable commit outcome cannot be reloaded")
	}
	m.Close()
	m.Close()
	if _, e = m.List(context.Background()); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	if _, e = m.Save(context.Background(), requestFixture(t, "closed", nil)); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
}
func TestCorruptionUnknownNamesAndBounds(t *testing.T) {
	m, fs := managerFixture()
	old := mustSave(t, m, "old", nil)
	original := append([]byte{}, fs.entries[old.ID+extension].data...)
	for _, offset := range []int{0, len(journalMagic), len(journalMagic) + 4, len(original) - 1, len(original) - len(commitMagic) - 5} {
		mutated := append([]byte{}, original...)
		mutated[offset] ^= 0x7f
		fs.entries[old.ID+extension].data = mutated
		if _, e := m.Load(context.Background(), old.ID); !errors.Is(e, ErrStore) {
			t.Fatalf("corruption %d accepted: %v", offset, e)
		}
	}
	fs.entries[old.ID+extension].data = original
	fs.entries["outside.txt"] = &memoryEntry{data: []byte("do not touch")}
	if _, e := m.List(context.Background()); !errors.Is(e, ErrStore) {
		t.Fatal("unknown file ignored")
	}
	if _, e := m.Save(context.Background(), requestFixture(t, "new", nil)); !errors.Is(e, ErrStore) {
		t.Fatal("unknown directory accepted")
	}
	if _, ok := fs.entries["outside.txt"]; !ok {
		t.Fatal("foreign file deleted")
	}
	delete(fs.entries, "outside.txt")
	fs.entries[old.ID+extension].data = make([]byte, maxJournal+1)
	if _, e := m.Load(context.Background(), old.ID); !errors.Is(e, ErrStore) {
		t.Fatal("oversized read accepted")
	}
	if _, e := validNames([]string{"P-" + strings.Repeat("a", 32) + extension}); !errors.Is(e, ErrStore) {
		t.Fatal("case alias accepted")
	}
	if _, e := validNames(make([]string, maxRecords+1)); !errors.Is(e, ErrFull) {
		t.Fatal("unbounded directory accepted")
	}
	fs.entries[old.ID+extension].data = append([]byte{}, journalMagic...)
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], maxFrame+1)
	fs.entries[old.ID+extension].data = append(fs.entries[old.ID+extension].data, size[:]...)
	if _, e := m.Load(context.Background(), old.ID); !errors.Is(e, ErrStore) {
		t.Fatal("oversized frame accepted")
	}
}
func TestFullStoreAndEntropyFailureDoNotOverwrite(t *testing.T) {
	m, fs := managerFixture()
	m.entropy = bytes.NewReader(nil)
	if _, e := m.Save(context.Background(), requestFixture(t, "no entropy", nil)); !errors.Is(e, ErrStore) || len(fs.begins) != 0 {
		t.Fatal("missing entropy caused I/O")
	}
	m, fs = managerFixture()
	fs.exists = true
	for i := 0; i < maxRecords; i++ {
		id := "p-" + strings.Repeat("0", 30) + "00"
		tail := []byte("0123456789abcdef")
		id = id[:32] + string(tail[i/16]) + string(tail[i%16])
		fs.entries[id+extension] = &memoryEntry{}
	}
	if _, e := m.Save(context.Background(), requestFixture(t, "full", nil)); !errors.Is(e, ErrFull) || len(fs.entries) != maxRecords {
		t.Fatalf("full store: %v %d", e, len(fs.entries))
	}
}

func TestConcurrentManagersCannotBothCommitTheSameRevision(t *testing.T) {
	m, fs := managerFixture()
	original := mustSave(t, m, "同时编辑", nil)
	other := newManager(fs)
	var entropy countingEntropy = 90
	other.entropy = &entropy
	start := make(chan struct{})
	type outcome struct {
		record Record
		err    error
	}
	results := make(chan outcome, 2)
	for index, manager := range []*Manager{m, other} {
		request := requestFixture(t, []string{"窗口一", "窗口二"}[index], &original)
		go func(manager *Manager, data []byte) {
			<-start
			record, err := manager.Save(context.Background(), data)
			results <- outcome{record, err}
		}(manager, request)
	}
	close(start)
	winners := 0
	var winner Record
	for range 2 {
		result := <-results
		if result.err == nil {
			winners++
			winner = result.record
		} else if !errors.Is(result.err, ErrBusy) && !errors.Is(result.err, ErrConflict) {
			t.Fatal(result.err)
		}
	}
	if winners != 1 {
		t.Fatalf("same revision committed %d times", winners)
	}
	loaded, err := m.Load(context.Background(), original.ID)
	if err != nil || loaded.Revision != winner.Revision || loaded.Name != winner.Name {
		t.Fatal("winning commit changed")
	}
}

func TestJournalFrameUsesActualTemporaryFileIO(t *testing.T) {
	// Only a new Go-test-owned TEMP file is opened; production New/NT paths are
	// never invoked. Exercise the same read/seek/sync framing against os.File.
	id := "p-" + strings.Repeat("a", 32)
	config, err := canonicalDraft(configFixture(t), id)
	if err != nil {
		t.Fatal(err)
	}
	entry := commit{SchemaVersion: 1, ID: id, Name: "临时夹具", Config: config, Engine: EngineSelection{EngineID: "webview2"}, UpdatedAt: time.Now().UTC(), Nonce: strings.Repeat("b", 32)}
	frame, err := encodeCommit(entry)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(t.TempDir(), id+extension), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.Write(append(append([]byte{}, journalMagic...), frame...)); err != nil {
		t.Fatal(err)
	}
	if err = file.Sync(); err != nil {
		t.Fatal(err)
	}
	loaded, err := readJournal(file, id)
	if err != nil || loaded.record.Name != entry.Name || loaded.record.RecoveryNeeded || loaded.record.Applied {
		t.Fatalf("temporary file framing: %+v %v", loaded, err)
	}
}
