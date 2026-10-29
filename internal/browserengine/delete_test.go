package browserengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// TEMP-only bounded backend; production uses the separately tested relative
// native-handle backend. No test calls New or the user's actual cache.
type tempDeletion struct {
	store  *os.Root
	tree   *os.Root
	rel    string
	files  []TreeFile
	dirs   []string
	failAt int
	noop   bool
	once   sync.Once
}

func (d *tempDeletion) Close() { d.once.Do(func() { _ = d.tree.Close() }) }
func (d *tempDeletion) Remove(ctx context.Context) (int, int64, error) {
	if d.noop {
		return 0, 0, nil
	}
	files, bytes := 0, int64(0)
	for _, file := range d.files {
		if ctx.Err() != nil || d.failAt > 0 && files == d.failAt {
			return files, bytes, ErrDeletePartial
		}
		if err := d.tree.Remove(file.Name); err != nil {
			return files, bytes, err
		}
		files++
		bytes += file.Size
	}
	for _, name := range d.dirs {
		if err := d.tree.Remove(name); err != nil {
			return files, bytes, err
		}
	}
	d.Close()
	return files, bytes, d.store.Remove(d.rel)
}
func tempDeleteFactory(failAt int, noop bool) func(*os.Root, string, string, Receipt) (versionDeletion, error) {
	return func(root *os.Root, id, version string, receipt Receipt) (versionDeletion, error) {
		tree, err := root.OpenRoot(id + "/" + version)
		if err != nil {
			return nil, err
		}
		info, err := tree.Stat(receiptName)
		if err != nil {
			tree.Close()
			return nil, err
		}
		files := append([]TreeFile(nil), receipt.Files...)
		files = append(files, TreeFile{Name: receiptName, Size: info.Size()})
		dirs := append([]string(nil), receipt.Directories...)
		sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
		return &tempDeletion{store: root, tree: tree, rel: id + "/" + version, files: files, dirs: dirs, failAt: failAt, noop: noop}, nil
	}
}
func copyTestManager(m *Manager) *Manager {
	return &Manager{directory: m.directory, cacheDirectory: m.cacheDirectory, client: m.client, acquire: m.acquire, verifyVersion: m.verifyVersion, verifyRoot: m.verifyRoot, lockFile: m.lockFile, deleteTree: m.deleteTree}
}
func deleteManager(t *testing.T) *Manager {
	archive := chromeFixture(t)
	m := fakeManager(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			version := strings.TrimSuffix(filepath.Base(r.URL.Path), ".json")
			return response(chromeMetadata(version)), nil
		}
		return response(archive), nil
	})
	m.deleteTree = tempDeleteFactory(0, false)
	return m
}
func addDeleteVersion(t *testing.T, m *Manager, version string) {
	t.Helper()
	if _, err := m.DownloadVersion(context.Background(), Chrome, version); err != nil {
		t.Fatal(err)
	}
}
func TestDeleteExactVersionPreservesOtherVersionAndData(t *testing.T) {
	m := deleteManager(t)
	addDeleteVersion(t, m, "156.0.8078.47")
	addDeleteVersion(t, m, "156.0.8078.48")
	// Sibling profiles/instances are outside the only permitted root.
	for _, leaf := range []string{"BrowserProfiles", "BrowserInstances"} {
		dir := filepath.Join(filepath.Dir(m.directory), leaf, t.Name())
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(filepath.Join(dir, "sentinel")); _ = os.Remove(dir) })
		if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := m.DeleteVersion(context.Background(), Chrome, "156.0.8078.47")
	if err != nil || !result.Deleted || result.Files != 7 || result.Bytes <= 0 {
		t.Fatal(result, err)
	}
	if _, err := os.Lstat(filepath.Join(m.directory, Chrome, "156.0.8078.47")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := m.InspectInstalled(Chrome, "156.0.8078.48"); err != nil {
		t.Fatal(err)
	}
	for _, leaf := range []string{"BrowserProfiles", "BrowserInstances"} {
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(m.directory), leaf, t.Name(), "sentinel")); err != nil || string(b) != "keep" {
			t.Fatal(err)
		}
	}
	if receipt, err := m.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); !errors.Is(err, ErrNotInstalled) || receipt.Deleted {
		t.Fatal(receipt, err)
	}
}
func TestDeleteRefusesInvalidUnownedAndUnverifiedBeforeMutation(t *testing.T) {
	for _, test := range []struct{ id, version string }{{"../BrowserProfiles", "156.0.8078.47"}, {Chrome, "../156.0.8078.47"}, {Chrome, "156.0.8078.47:ads"}, {Firefox, "https://evil.invalid"}} {
		m := deleteManager(t)
		calls := 0
		m.acquire = func(string, bool) (func(), error) { calls++; return func() {}, nil }
		if result, err := m.DeleteVersion(context.Background(), test.id, test.version); err == nil || result.Deleted || calls != 0 {
			t.Fatal(result, err, calls)
		}
	}
	m := deleteManager(t)
	addDeleteVersion(t, m, "156.0.8078.47")
	m.cacheDirectory = ""
	if _, err := m.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrStore {
		t.Fatal(err)
	}
	m.cacheDirectory = m.directory
	for _, mutation := range []string{"extra", "changed", "missing-receipt"} {
		t.Run(mutation, func(t *testing.T) {
			m := deleteManager(t)
			addDeleteVersion(t, m, "156.0.8078.47")
			dir := filepath.Join(m.directory, Chrome, "156.0.8078.47")
			switch mutation {
			case "extra":
				_ = os.WriteFile(filepath.Join(dir, "unknown.dll"), []byte("keep"), 0600)
			case "changed":
				_ = os.WriteFile(filepath.Join(dir, "chrome.dll"), []byte("changed"), 0600)
			case "missing-receipt":
				_ = os.Remove(filepath.Join(dir, receiptName))
			}
			m.deleteTree = func(*os.Root, string, string, Receipt) (versionDeletion, error) {
				t.Fatal("unverified tree reached delete")
				return nil, ErrStore
			}
			if result, err := m.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err == nil || result.Deleted {
				t.Fatal(result, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "chrome.exe")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestDeletePartialAndFalseBackendSuccessNeverClaimDeleted(t *testing.T) {
	for _, mode := range []string{"partial", "noop", "reader-busy", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			m := deleteManager(t)
			addDeleteVersion(t, m, "156.0.8078.47")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "partial":
				m.deleteTree = tempDeleteFactory(2, false)
			case "noop":
				m.deleteTree = tempDeleteFactory(0, true)
			case "reader-busy":
				m.deleteTree = func(*os.Root, string, string, Receipt) (versionDeletion, error) { return nil, ErrBusy }
			case "cancelled":
				cancel()
			}
			result, err := m.DeleteVersion(ctx, Chrome, "156.0.8078.47")
			if err == nil || result.Deleted {
				t.Fatal(result, err)
			}
			if mode == "partial" && (!errors.Is(err, ErrDeletePartial) || result.Files != 2) {
				t.Fatal(result, err)
			}
			if mode == "partial" {
				name := strings.TrimSuffix(versionLockName(Chrome, "156.0.8078.47"), ".lock") + ".delete-failed.json"
				data, readErr := os.ReadFile(filepath.Join(m.directory, name))
				var marker struct {
					DeleteReceipt
					State string `json:"state"`
				}
				if readErr != nil || json.Unmarshal(data, &marker) != nil || marker.Deleted || marker.Files != 2 || marker.EngineID != Chrome || marker.Version != "156.0.8078.47" || marker.State != "incomplete_manual_review_required" {
					t.Fatal("partial action was not recorded accurately", string(data), readErr)
				}
			}
			if _, err := os.Stat(filepath.Join(m.directory, Chrome, "156.0.8078.47")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestDeleteCrossManagerStoreAndUsageExclusion(t *testing.T) {
	m := deleteManager(t)
	addDeleteVersion(t, m, "156.0.8078.47")
	other := copyTestManager(m)
	root, closeRoot, err := m.openStore(false)
	if err != nil {
		t.Fatal(err)
	}
	_ = root
	if _, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrBusy {
		t.Fatal(err)
	}
	closeRoot()
	usage, err := m.acquireVersionUsage(Chrome, "156.0.8078.47")
	if err != nil {
		t.Fatal(err)
	}
	secondUsage, err := other.acquireVersionUsage(Chrome, "156.0.8078.47")
	if err != nil {
		t.Fatal(err)
	}
	lease := &CodeLease{}
	lease.holdUsage(usage)
	if _, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrBusy {
		t.Fatal(err)
	}
	lease.Close()
	lease.Close()
	if _, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrBusy {
		t.Fatal(err)
	}
	secondUsage()
	if receipt, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != nil || !receipt.Deleted {
		t.Fatal(receipt, err)
	}
}
func TestAcquireCodeLeaseReservesVersionBeforeDownloadAndReleasesOnFailure(t *testing.T) {
	m := deleteManager(t)
	addDeleteVersion(t, m, "156.0.8078.47")
	other := copyTestManager(m)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	m.lockFile = func(directory, name string, exclusive bool) (func(), error) {
		if directory != m.directory {
			t.Error("cache lock contaminated session instance root")
			return nil, ErrStore
		}
		return fakeStoreLock(directory, name, exclusive)
	}
	m.client = &http.Client{Transport: fakeHTTP(func(*http.Request) (*http.Response, error) { close(entered); <-release; return nil, ErrSource })}
	go func() {
		_, err := m.AcquireCodeLease(context.Background(), Chrome, "156.0.8078.47", strings.Repeat("b", 64))
		done <- err
	}()
	<-entered
	if receipt, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrBusy || receipt.Deleted {
		t.Fatal(receipt, err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("download failure ignored")
	}
	instance, err := m.InstanceDirectory(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(instance)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != Chrome {
			t.Fatal("cache artifact in exact session tree", entry.Name())
		}
	}
	if receipt, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != nil || !receipt.Deleted {
		t.Fatal(receipt, err)
	}
}

func TestDeleteRefusesAnotherManagersActiveDownload(t *testing.T) {
	m := deleteManager(t)
	addDeleteVersion(t, m, "156.0.8078.47")
	other := copyTestManager(m)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	m.client = &http.Client{Transport: fakeHTTP(func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return nil, ErrSource
	})}
	go func() { _, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.48"); done <- err }()
	<-entered
	if result, err := other.DeleteVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrBusy || result.Deleted {
		t.Fatal(result, err)
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("fake source failure ignored")
	}
	if _, err := other.InspectInstalled(Chrome, "156.0.8078.47"); err != nil {
		t.Fatal(err)
	}
}
