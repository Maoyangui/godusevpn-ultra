package browserengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrBusy          = errors.New("browser_engine_version_busy")
	ErrNotInstalled  = errors.New("browser_engine_version_not_installed")
	ErrDeletePartial = errors.New("browser_engine_delete_incomplete")
)

const storeLockName = ".ultra-engine-store.lock"

// versionDeletion owns the exact, verified objects it will remove. Opening it
// must not mutate the version; Remove never follows a path to a new object.
type versionDeletion interface {
	Remove(context.Context) (int, int64, error)
	Close()
}

type DeleteReceipt struct {
	EngineID string `json:"engineId"`
	Version  string `json:"version"`
	Deleted  bool   `json:"deleted"`
	Files    int    `json:"files"`
	Bytes    int64  `json:"bytes"`
}

func versionLockName(id, version string) string {
	hash := sha256.Sum256([]byte(id + "\x00" + version))
	return ".ultra-engine-use-" + hex.EncodeToString(hash[:]) + ".lock"
}

// Cache-only target validation precedes every filesystem access. cacheDirectory
// is derived by New, not supplied by a Wails request or a cache receipt.
func (m *Manager) deleteTarget(id, version string) (string, error) {
	if m == nil || !validSelection(id, version) {
		return "", ErrInput
	}
	if m.cacheDirectory == "" || m.directory != m.cacheDirectory || !filepath.IsAbs(m.directory) || filepath.Clean(m.directory) != m.directory {
		return "", ErrStore
	}
	target := filepath.Join(m.directory, id, version)
	relative, err := filepath.Rel(m.directory, target)
	if err != nil || relative != filepath.Join(id, version) || !filepath.IsAbs(target) || target == m.directory || strings.HasPrefix(relative, "..") {
		return "", ErrInput
	}
	return target, nil
}

// DeleteVersion removes only a fully verified cache version. It never scans or
// removes BrowserInstances, profiles, cookies, other versions, or unknown files.
// Files/Bytes include the receipt; on error they are partial action counts, not
// a claim that the requested version has disappeared.
func (m *Manager) DeleteVersion(ctx context.Context, id, version string) (DeleteReceipt, error) {
	result := DeleteReceipt{EngineID: id, Version: version}
	if _, err := m.deleteTarget(id, version); err != nil {
		return result, err
	}
	if ctx == nil || ctx.Err() != nil {
		return result, ErrStore
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	root, closeRoot, err := m.openStore(false)
	if err != nil {
		return result, err
	}
	defer closeRoot()
	unlockUsage, err := m.lockFile(m.directory, versionLockName(id, version), true)
	if err != nil {
		return result, err
	}
	defer unlockUsage()
	parent, err := root.Lstat(id)
	if os.IsNotExist(err) {
		return result, ErrNotInstalled
	}
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return result, ErrIntegrity
	}
	info, err := root.Lstat(id + "/" + version)
	if os.IsNotExist(err) {
		return result, ErrNotInstalled
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return result, ErrIntegrity
	}
	item, err := m.inspectStore(root, id, version)
	if err != nil {
		return result, err
	}
	if ctx.Err() != nil || m.deleteTree == nil {
		return result, ErrStore
	}
	tree, err := m.deleteTree(root, id, version, item.Receipt)
	if err != nil {
		return result, err
	}
	defer tree.Close()
	result.Files, result.Bytes, err = tree.Remove(ctx)
	if err != nil {
		if result.Files > 0 {
			recordDeleteFailure(root, result)
		}
		return result, ErrDeletePartial
	}
	// All deletion handles must have been closed before checking absence. An
	// open delete-pending object is not a successful deletion receipt.
	tree.Close()
	if _, err := root.Lstat(id + "/" + version); !os.IsNotExist(err) {
		recordDeleteFailure(root, result)
		return result, ErrDeletePartial
	}
	result.Deleted = true
	return result, nil
}

func recordDeleteFailure(root *os.Root, result DeleteReceipt) {
	// A best-effort, bounded local explanation. Never overwrite/follow an
	// existing marker and never perform a wider recovery delete automatically.
	data, err := json.Marshal(struct {
		DeleteReceipt
		State string    `json:"state"`
		At    time.Time `json:"at"`
	}{result, "incomplete_manual_review_required", time.Now().UTC()})
	if err != nil {
		return
	}
	name := strings.TrimSuffix(versionLockName(result.EngineID, result.Version), ".lock") + ".delete-failed.json"
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(data)
	_ = file.Sync()
}

// A shared version lease starts before any session download and lasts until
// CodeLease.Close after confirmed native cleanup. It coordinates independent
// managers/processes even though execution occurs in a separate instance tree.
func (m *Manager) acquireVersionUsage(id, version string) (func(), error) {
	if _, err := m.deleteTarget(id, version); err != nil {
		return nil, err
	}
	if m.acquire == nil || m.lockFile == nil {
		return nil, ErrStore
	}
	release, err := m.acquire(m.directory, true)
	if err != nil {
		return nil, ErrStore
	}
	root, err := os.OpenRoot(m.directory)
	if err != nil {
		release()
		return nil, ErrStore
	}
	if m.verifyRoot != nil && m.verifyRoot(root, m.directory) != nil {
		root.Close()
		release()
		return nil, ErrStore
	}
	unlock, err := m.lockFile(m.directory, versionLockName(id, version), false)
	if err != nil {
		root.Close()
		release()
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { unlock(); root.Close(); release() }) }, nil
}
