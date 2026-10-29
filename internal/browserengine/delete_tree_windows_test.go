//go:build windows

package browserengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every test in this file uses in-memory objects. In particular, no test calls
// openVersionDeletion, NtCreateFile, FileDispositionInfo or a real cache path.
type fakeDeletionTree struct {
	nodes    map[string]*fakeDeletionHandle
	calls    []string
	openErr  map[string]error
	opened   []*fakeDeletionHandle
	policies map[string][2]bool
}

type fakeDeletionHandle struct {
	tree       *fakeDeletionTree
	name       string
	info       deletionInfo
	data       *bytes.Reader
	names      []string
	infoErr    error
	readErr    error
	namesErr   error
	disposeErr error
	closeErr   error
	closeCount int
	onDispose  func()
}

func (f *fakeDeletionHandle) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.data.Read(p)
}
func (f *fakeDeletionHandle) Info() (deletionInfo, error) { return f.info, f.infoErr }
func (f *fakeDeletionHandle) Names(limit int) ([]string, error) {
	if len(f.names) > limit {
		return append([]string(nil), f.names[:limit]...), nil
	}
	return append([]string(nil), f.names...), f.namesErr
}
func (f *fakeDeletionHandle) Dispose() error {
	f.tree.calls = append(f.tree.calls, "dispose:"+f.name)
	if f.onDispose != nil {
		f.onDispose()
	}
	return f.disposeErr
}
func (f *fakeDeletionHandle) Close() error {
	f.closeCount++
	f.tree.calls = append(f.tree.calls, "close:"+f.name)
	return f.closeErr
}
func (f *fakeDeletionTree) open(parent deletionHandle, name string, directory, remove bool) (deletionHandle, error) {
	p, ok := parent.(*fakeDeletionHandle)
	if !ok {
		return nil, ErrStore
	}
	key := path.Join(p.name, name)
	f.calls = append(f.calls, "open:"+key)
	f.policies[key] = [2]bool{directory, remove}
	if err := f.openErr[key]; err != nil {
		return nil, err
	}
	h := f.nodes[key]
	if h == nil {
		return nil, io.ErrUnexpectedEOF
	}
	f.opened = append(f.opened, h)
	return h, nil
}

func deletionFixture(t *testing.T) (*fakeDeletionTree, Receipt, string) {
	t.Helper()
	f := &fakeDeletionTree{nodes: map[string]*fakeDeletionHandle{}, openErr: map[string]error{}, policies: map[string][2]bool{}}
	version := "156.0.8078.47"
	base := Chrome + "/" + version
	receipt := Receipt{SchemaVersion: 1, Release: Release{EngineID: Chrome, Version: version, Platform: Platform}, Executable: "chrome.exe", Directories: []string{"locales", "locales/deep"}}
	data := map[string][]byte{"chrome.exe": []byte("not-an-executable-test-fixture"), "locales/en.pak": []byte("locale"), "locales/deep/a.dat": []byte("deep")}
	for name, content := range data {
		hash := sha256.Sum256(content)
		receipt.Files = append(receipt.Files, TreeFile{Name: name, Size: int64(len(content)), SHA256: hex.EncodeToString(hash[:])})
	}
	sort.Slice(receipt.Files, func(i, j int) bool { return receipt.Files[i].Name < receipt.Files[j].Name })
	add := func(name string, directory bool, content []byte) {
		f.nodes[name] = &fakeDeletionHandle{tree: f, name: name, info: deletionInfo{directory: directory, links: 1, size: int64(len(content))}, data: bytes.NewReader(content), namesErr: io.EOF}
	}
	for _, name := range []string{".", Chrome, base, base + "/locales", base + "/locales/deep"} {
		add(name, true, nil)
	}
	for name, content := range data {
		add(base+"/"+name, false, content)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	add(base+"/"+receiptName, false, encoded)
	for name := range f.nodes {
		if name != "." {
			parent := f.nodes[path.Dir(name)]
			parent.names = append(parent.names, path.Base(name))
		}
	}
	for _, node := range f.nodes {
		sort.Strings(node.names)
	}
	return f, receipt, base
}

func (f *fakeDeletionTree) prepare(receipt Receipt) (versionDeletion, error) {
	return prepareVersionDeletion(f.nodes["."], Chrome, receipt.Release.Version, receipt, f.open)
}
func dispositions(calls []string) []string {
	var result []string
	for _, call := range calls {
		if strings.HasPrefix(call, "dispose:") {
			result = append(result, strings.TrimPrefix(call, "dispose:"))
		}
	}
	return result
}
func assertDeletionClosedExactlyOnce(t *testing.T, f *fakeDeletionTree) {
	t.Helper()
	all := append([]*fakeDeletionHandle{f.nodes["."]}, f.opened...)
	for _, h := range all {
		if h.closeCount != 1 {
			t.Fatalf("handle %s closed %d times", h.name, h.closeCount)
		}
	}
}

func TestDeletionNativeOpenPolicyAndABIOnly(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, remove := range []bool{false, true} {
			p := deletionPolicy(directory, remove)
			if p.share != windows.FILE_SHARE_READ || p.disposition != windows.FILE_OPEN || p.attributes&windows.OBJ_DONT_REPARSE == 0 || p.attributes&windows.OBJ_CASE_INSENSITIVE == 0 || p.options&windows.FILE_OPEN_REPARSE_POINT == 0 || p.options&windows.FILE_SYNCHRONOUS_IO_NONALERT == 0 || p.access&windows.SYNCHRONIZE == 0 || p.access&windows.READ_CONTROL == 0 {
				t.Fatal("unsafe policy", p)
			}
			if (p.options&windows.FILE_DIRECTORY_FILE != 0) != directory || (p.options&windows.FILE_NON_DIRECTORY_FILE != 0) == directory || (p.access&windows.DELETE != 0) != remove || p.access&(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA|windows.FILE_WRITE_ATTRIBUTES|windows.FILE_WRITE_EA) != 0 {
				t.Fatal("wrong object/access policy", p)
			}
		}
	}
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{}) != 48 || unsafe.Sizeof(windows.IO_STATUS_BLOCK{}) != 16) {
		t.Fatal("unexpected native ABI")
	}
	if _, err := openDeletionChild(&fakeDeletionHandle{}, "x", false, true); err != ErrInput {
		t.Fatal("foreign handle reached native open")
	}
	for _, name := range []string{"", "../other", "a/b", `a\b`, "C:outside", "/root", "name.", "nul", "a\x00b"} {
		if _, err := openDeletionChild(&nativeDeletionHandle{file: &os.File{}}, name, false, true); err != ErrInput {
			t.Fatal("unsafe child reached native open", name)
		}
	}
	if _, err := openVersionDeletion(nil, Chrome, "156.0.8078.47", Receipt{}); err != ErrInput {
		t.Fatal("nil root reached filesystem")
	}
	for _, err := range []error{ErrBusy, windows.ERROR_SHARING_VIOLATION, windows.ERROR_LOCK_VIOLATION, windows.STATUS_SHARING_VIOLATION, windows.STATUS_DELETE_PENDING} {
		if deletionNativeError(err) != ErrBusy {
			t.Fatal("busy mapping", err)
		}
	}
	if deletionNativeError(windows.ERROR_ACCESS_DENIED) != ErrStore {
		t.Fatal("raw native error escaped")
	}
	for _, test := range []struct {
		handle windows.Handle
		status windows.IO_STATUS_BLOCK
		err    error
		valid  bool
	}{
		{7, windows.IO_STATUS_BLOCK{Information: 1}, nil, true},
		{0, windows.IO_STATUS_BLOCK{Information: 1}, nil, false},
		{windows.InvalidHandle, windows.IO_STATUS_BLOCK{Information: 1}, nil, false},
		{7, windows.IO_STATUS_BLOCK{Information: 0}, nil, false},
		{7, windows.IO_STATUS_BLOCK{Information: 2}, nil, false},
		{7, windows.IO_STATUS_BLOCK{Information: 3}, nil, false},
		{7, windows.IO_STATUS_BLOCK{Status: windows.STATUS_PENDING, Information: 1}, nil, false},
		{7, windows.IO_STATUS_BLOCK{Information: 1}, windows.STATUS_PENDING, false},
		{7, windows.IO_STATUS_BLOCK{Information: 1}, windows.ERROR_ACCESS_DENIED, false},
	} {
		if validDeletionOpenResult(test.handle, test.status, test.err) != test.valid {
			t.Fatal("unconfirmed native open accepted", test)
		}
	}
}

func TestDeletionPinsEntireExactTreeBeforeAnyMutation(t *testing.T) {
	f, receipt, base := deletionFixture(t)
	l, err := f.prepare(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispositions(f.calls)) != 0 || len(f.opened) != len(f.nodes)-1 {
		t.Fatal("preparation mutated or failed to pin every object")
	}
	if f.policies[Chrome] != [2]bool{true, false} {
		t.Fatal("engine parent must not be DELETE opened")
	}
	for name, policy := range f.policies {
		if name != Chrome && (!policy[1] || !strings.HasPrefix(name, base)) {
			t.Fatal("unbounded/delete-ineligible object", name, policy)
		}
	}
	count, size, err := l.Remove(context.Background())
	var expectedSize int64
	for name, node := range f.nodes {
		if strings.HasPrefix(name, base+"/") && !node.info.directory {
			expectedSize += node.info.size
		}
	}
	if err != nil || count != len(receipt.Files)+1 || size != expectedSize {
		t.Fatal(count, size, err)
	}
	got := dispositions(f.calls)
	want := []string{base + "/chrome.exe", base + "/locales/deep/a.dat", base + "/locales/en.pak", base + "/" + receiptName, base + "/locales/deep", base + "/locales", base}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("wrong deletion order", got)
	}
	l.Close()
	l.Close()
	assertDeletionClosedExactlyOnce(t, f)
	if n, bytes, err := l.Remove(context.Background()); n != 0 || bytes != 0 || err != ErrDeletePartial {
		t.Fatal("repeated deletion accepted")
	}
}

func TestDeletionPreparationFailuresNeverDeleteOrLeakHandles(t *testing.T) {
	cases := []struct {
		name string
		edit func(*fakeDeletionTree, *Receipt, string)
	}{
		{"hash", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.nodes[b+"/chrome.exe"].data = bytes.NewReader([]byte("different"))
		}},
		{"size", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b+"/chrome.exe"].info.size++ }},
		{"file-reparse", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b+"/chrome.exe"].info.reparse = true }},
		{"directory-reparse", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b+"/locales"].info.reparse = true }},
		{"hardlink", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b+"/chrome.exe"].info.links = 2 }},
		{"wrong-kind", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b+"/chrome.exe"].info.directory = true }},
		{"read-error", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.nodes[b+"/chrome.exe"].readErr = errors.New("synthetic")
		}},
		{"extra", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.nodes[b].names = append(f.nodes[b].names, "external.bin")
		}},
		{"extra-directory", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.nodes[b+"/locales"].names = append(f.nodes[b+"/locales"].names, "extra-dir")
		}},
		{"missing", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b].names = f.nodes[b].names[1:] }},
		{"alias", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.nodes[b].names[1] = strings.ToUpper(f.nodes[b].names[1])
		}},
		{"duplicate-member", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b].names[1] = f.nodes[b].names[0] }},
		{"names-error", func(f *fakeDeletionTree, _ *Receipt, b string) { f.nodes[b].namesErr = errors.New("synthetic") }},
		{"open-busy", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.openErr[b+"/chrome.exe"] = windows.STATUS_SHARING_VIOLATION
		}},
		{"info-error", func(f *fakeDeletionTree, _ *Receipt, b string) {
			f.nodes[b+"/locales"].infoErr = errors.New("synthetic")
		}},
		{"receipt-changed", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.MetadataSHA256 = strings.Repeat("a", 64) }},
		{"receipt-unknown", func(f *fakeDeletionTree, _ *Receipt, b string) {
			h := f.nodes[b+"/"+receiptName]
			data := append([]byte(`{"unexpected":true,`), mustDeletionReceipt(t, h)[1:]...)
			h.data, h.info.size = bytes.NewReader(data), int64(len(data))
		}},
		{"receipt-case-alias", func(f *fakeDeletionTree, _ *Receipt, b string) {
			h := f.nodes[b+"/"+receiptName]
			data := bytes.Replace(mustDeletionReceipt(t, h), []byte(`"schemaVersion"`), []byte(`"SchemaVersion"`), 1)
			h.data, h.info.size = bytes.NewReader(data), int64(len(data))
		}},
		{"receipt-duplicate", func(f *fakeDeletionTree, _ *Receipt, b string) {
			h := f.nodes[b+"/"+receiptName]
			data := append([]byte(`{"schemaVersion":1,`), mustDeletionReceipt(t, h)[1:]...)
			h.data, h.info.size = bytes.NewReader(data), int64(len(data))
		}},
		{"receipt-trailing", func(f *fakeDeletionTree, _ *Receipt, b string) {
			h := f.nodes[b+"/"+receiptName]
			data := append(mustDeletionReceipt(t, h), []byte(`{}`)...)
			h.data, h.info.size = bytes.NewReader(data), int64(len(data))
		}},
		{"traversal", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Files[0].Name = "../other-version/file" }},
		{"missing-parent", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Directories = []string{"locales/deep"} }},
		{"case-collision", func(_ *fakeDeletionTree, r *Receipt, _ string) {
			r.Files = append(r.Files, TreeFile{Name: "CHROME.EXE", Size: 0, SHA256: strings.Repeat("a", 64)})
		}},
		{"negative-size", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Files[0].Size = -1 }},
		{"wrong-selection", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Release.EngineID = Firefox }},
		{"too-many-files", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Files = make([]TreeFile, maxEntries+1) }},
		{"too-many-directories", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Directories = make([]string, maxEntries+1) }},
		{"file-directory-collision", func(_ *fakeDeletionTree, r *Receipt, _ string) { r.Files[0].Name = "locales" }},
		{"root-reparse", func(f *fakeDeletionTree, _ *Receipt, _ string) { f.nodes["."].info.reparse = true }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f, receipt, base := deletionFixture(t)
			test.edit(f, &receipt, base)
			l, err := f.prepare(receipt)
			if err == nil || l != nil {
				t.Fatal("invalid preparation accepted", err)
			}
			if test.name == "open-busy" && err != ErrBusy {
				t.Fatal("busy lost", err)
			}
			if len(dispositions(f.calls)) != 0 {
				t.Fatal("preparation deleted a member")
			}
			assertDeletionClosedExactlyOnce(t, f)
		})
	}
}

func TestDeletionDoesNotOpenOrRemoveUnselectedVersionsOrSiblingStores(t *testing.T) {
	f, receipt, _ := deletionFixture(t)
	sentinels := []string{Chrome + "/157.0.1.2/chrome.exe", Firefox + "/158.0/firefox.exe", "../BrowserInstances/session/chrome.exe", "../BrowserProfiles/record"}
	for _, name := range sentinels {
		f.nodes[name] = &fakeDeletionHandle{tree: f, name: name, info: deletionInfo{links: 1}, data: bytes.NewReader([]byte("sentinel"))}
	}
	l, err := f.prepare(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Remove(context.Background()); err != nil {
		t.Fatal(err)
	}
	l.Close()
	for _, name := range sentinels {
		if f.nodes[name].closeCount != 0 {
			t.Fatal("sibling opened", name)
		}
		for _, call := range f.calls {
			if strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(call, "open:"), "close:"), "dispose:") == name {
				t.Fatal("sibling affected", call)
			}
		}
	}
}

func mustDeletionReceipt(t *testing.T, h *fakeDeletionHandle) []byte {
	t.Helper()
	data, err := io.ReadAll(h.data)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDeletionPartialStopAndCancellationAreNeverSuccess(t *testing.T) {
	for _, kind := range []string{"first-dispose", "middle-dispose", "close", "directory", "cancel-before", "cancel-after-one"} {
		t.Run(kind, func(t *testing.T) {
			f, receipt, base := deletionFixture(t)
			l, err := f.prepare(receipt)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wantCount := 0
			switch kind {
			case "first-dispose":
				f.nodes[base+"/chrome.exe"].disposeErr = errors.New("synthetic")
			case "middle-dispose":
				f.nodes[base+"/locales/deep/a.dat"].disposeErr = errors.New("synthetic")
				wantCount = 1
			case "close":
				f.nodes[base+"/chrome.exe"].closeErr = errors.New("synthetic")
			case "directory":
				f.nodes[base+"/locales/deep"].disposeErr = errors.New("synthetic")
				wantCount = len(receipt.Files) + 1
			case "cancel-before":
				cancel()
			case "cancel-after-one":
				f.nodes[base+"/chrome.exe"].onDispose = cancel
				wantCount = 1
			}
			n, size, err := l.Remove(ctx)
			if err != ErrDeletePartial || n != wantCount || size < 0 {
				t.Fatal(n, size, err)
			}
			beforeClose := len(dispositions(f.calls))
			l.Close()
			l.Close()
			if len(dispositions(f.calls)) != beforeClose {
				t.Fatal("Close attempted additional deletion")
			}
			assertDeletionClosedExactlyOnce(t, f)
			if kind == "cancel-before" && beforeClose != 0 || kind != "directory" && beforeClose > 2 {
				t.Fatal("continued deleting after failure")
			}
		})
	}
}

func TestDeletionCloseAndRemoveSerializeWithoutDoubleClose(t *testing.T) {
	for attempt := 0; attempt < 30; attempt++ {
		f, receipt, _ := deletionFixture(t)
		l, err := f.prepare(receipt)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _, _ = l.Remove(context.Background()) }()
		go func() { defer wg.Done(); l.Close() }()
		wg.Wait()
		l.Close()
		assertDeletionClosedExactlyOnce(t, f)
		for _, name := range dispositions(f.calls) {
			if name == "." || name == Chrome {
				t.Fatal(fmt.Sprintf("deleted ancestor %q", name))
			}
		}
	}
}
