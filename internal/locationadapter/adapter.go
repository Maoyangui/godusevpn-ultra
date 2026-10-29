// Package locationadapter defines the trust and rollback boundary for optional
// Windows location-provider components. It deliberately does not execute a
// third-party binary or change Windows policy by itself. A platform backend
// may be added only after the component has a pinned source, license, hash,
// and an independently verified readback implementation.
package locationadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusUnsupported Status = "unsupported"
	StatusMissing     Status = "missing"
	StatusUnverified  Status = "unverified"
	StatusReady       Status = "ready"
	StatusApplied     Status = "applied"
	StatusRollback    Status = "rollback_required"
)

// Position is intentionally small: the backend decides which Windows
// location API it covers. Coordinates never imply that IP, Wi-Fi or account
// location has changed.
type Position struct {
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	AccuracyMeter float64 `json:"accuracyMeter,omitempty"`
}

func (p Position) valid() error {
	if math.IsNaN(p.Latitude) || math.IsInf(p.Latitude, 0) || p.Latitude < -90 || p.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}
	if math.IsNaN(p.Longitude) || math.IsInf(p.Longitude, 0) || p.Longitude < -180 || p.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}
	if math.IsNaN(p.AccuracyMeter) || math.IsInf(p.AccuracyMeter, 0) || p.AccuracyMeter < 0 {
		return errors.New("accuracy must be zero or positive")
	}
	return nil
}

func (p Position) equal(other Position) bool {
	return p.Latitude == other.Latitude && p.Longitude == other.Longitude && p.AccuracyMeter == other.AccuracyMeter
}

// Component is a user-selected, locally installed artifact. SHA256 is
// mandatory; an empty hash intentionally leaves the component unverified.
type Component struct {
	ID       string `json:"id"`
	Upstream string `json:"upstream"`
	Version  string `json:"version"`
	License  string `json:"license"`
	// ArtifactPath is local-only and intentionally omitted from JSON reports
	// and backups so a detection export cannot disclose the user's profile.
	ArtifactPath  string `json:"-"`
	SHA256        string `json:"sha256"`
	RequiresAdmin bool   `json:"requiresAdmin"`
}

// Snapshot is the backend's independent readback of its override state.
// Present means that override is configured, not merely that Windows can
// obtain an ordinary physical location. Raw must be stable restoration data;
// transient observation timestamps do not belong in it.
// Raw is kept for exact restoration by the backend and is never included in a
// user-facing report or log.
type Snapshot struct {
	Present  bool     `json:"present"`
	Position Position `json:"position"`
	// Raw is persisted only inside the local backup so a backend can restore
	// its exact provider state. It is never copied into Report or logs.
	Raw []byte `json:"raw,omitempty"`
}

// Backend is the read-only platform seam. A backend without AtomicBackend
// cannot be used for any mutation, including recovery.
type Backend interface {
	Read(context.Context) (Snapshot, error)
}

var (
	ErrAtomicUnsupported = errors.New("backend cannot guarantee atomic location transactions")
	ErrStateConflict     = errors.New("location state or ownership changed")
)

// AtomicBackend is a required mutation contract, not a read-then-write helper.
// PrepareApply must not change provider configuration. It allocates a durable
// ownership/generation token in the returned target's Raw, usable after a
// process restart, and returns the exact state CompareAndSwap will install.
//
// CompareAndSwap must check all of expected, including Raw ownership, and
// perform the replacement within one OS-enforced exclusive transaction. It
// must reject ownership loss/ABA changes with ErrStateConflict without changing
// provider state. On other errors the state may only be expected or replacement,
// never a partial state. It must also reject replacing an unrelated owner's
// session. A prior Read, process-local mutex, or comparison of coordinates alone
// is insufficient. If the platform cannot provide this guarantee, PrepareApply
// and CompareAndSwap must return ErrAtomicUnsupported before changing anything.
// Implementations must never accept arbitrary shell arguments.
type AtomicBackend interface {
	Backend
	// CheckAtomic is a non-mutating runtime capability check. It must fail
	// closed when an OS transaction/ownership guarantee is unavailable.
	CheckAtomic(context.Context) error
	PrepareApply(context.Context, Snapshot, Position) (Snapshot, error)
	CompareAndSwap(context.Context, Snapshot, Snapshot) error
}

type Report struct {
	Status       Status    `json:"status"`
	Reason       string    `json:"reason"`
	Scope        string    `json:"scope"`
	Component    Component `json:"component"`
	ComponentSHA string    `json:"componentSha256,omitempty"`
	HasBackup    bool      `json:"hasBackup"`
	Position     *Position `json:"position,omitempty"`
}

type Result struct {
	Report  Report `json:"report"`
	Changed bool   `json:"changed"`
}

type backup struct {
	Version   int       `json:"version"`
	Component Component `json:"component"`
	Snapshot  Snapshot  `json:"snapshot"`
	Expected  Snapshot  `json:"expected"`
	// Pending is written before a mutation. It permits recovery after a crash
	// only when the provider still matches the old state or the attempted target.
	Pending *Snapshot `json:"pending,omitempty"`
}

const backupVersion = 3

// Controllers are values and can be copied. Serialize their transactions at
// package scope rather than putting a copyable mutex inside Controller.
// A future real backend must additionally own its OS session exclusively;
// this lock cannot serialize an unrelated process.
var transactionMu sync.Mutex

type Controller struct {
	Component  Component
	Backend    Backend
	BackupPath string
	Platform   string
	Admin      func() bool
}

func (c Controller) componentCheck() (string, error) {
	if c.Platform != "windows" {
		return "", errors.New("location provider adapter is Windows-only")
	}
	if c.Component.ID == "" || c.Component.Upstream == "" || !strings.HasPrefix(strings.ToLower(c.Component.Upstream), "https://") {
		return "", errors.New("component source is missing or not HTTPS")
	}
	if c.Component.Version == "" {
		return "", errors.New("component version is missing")
	}
	switch strings.ToLower(strings.TrimSpace(c.Component.License)) {
	case "mit", "apache-2.0", "apache 2.0", "apache-2.0 license":
	default:
		return "", errors.New("component license is not in the approved list")
	}
	if err := validateLocalAbsolutePath(c.Component.ArtifactPath); err != nil {
		return "", fmt.Errorf("component path: %w", err)
	}
	if c.Component.SHA256 == "" || !isHash(c.Component.SHA256) {
		return "", errors.New("component SHA-256 is missing or invalid")
	}
	lstat, err := os.Lstat(c.Component.ArtifactPath)
	if err == nil && lstat.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("component artifact cannot be a symlink")
	}
	info, err := os.Stat(c.Component.ArtifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", os.ErrNotExist
		}
		return "", fmt.Errorf("component stat: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("component artifact is not a regular file")
	}
	f, err := os.Open(c.Component.ArtifactPath)
	if err != nil {
		return "", fmt.Errorf("component open: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", fmt.Errorf("component hash: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, c.Component.SHA256) {
		return got, fmt.Errorf("component SHA-256 mismatch: got %s", got)
	}
	return got, nil
}

func (c Controller) Inspect(ctx context.Context) (Report, error) {
	transactionMu.Lock()
	defer transactionMu.Unlock()
	r := Report{Status: StatusUnsupported, Scope: "Windows Location Provider / verified backend only", Component: c.Component}
	if c.Platform != "windows" {
		r.Reason = "仅支持 Windows；不会修改其他平台或隧道。"
		return r, nil
	}
	got, err := c.componentCheck()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.Status, r.Reason = StatusMissing, "未找到已核验的位置组件；不会下载或执行第三方程序。"
			return r, nil
		}
		r.Status, r.Reason, r.ComponentSHA = StatusUnverified, err.Error(), got
		return r, nil
	}
	r.ComponentSHA = got
	if c.Backend == nil {
		r.Status, r.Reason = StatusUnverified, "位置后端未接入；仅完成组件核验。"
		return r, nil
	}
	s, err := c.readSnapshot(ctx)
	if err != nil {
		r.Status, r.Reason = StatusUnverified, fmt.Sprintf("位置读回失败：%v", err)
		return r, nil
	}
	atomic, ok := c.Backend.(AtomicBackend)
	if !ok {
		r.Status, r.Reason = StatusUnverified, ErrAtomicUnsupported.Error()
		return r, nil
	}
	if err := atomic.CheckAtomic(ctx); err != nil {
		r.Status, r.Reason = StatusUnverified, err.Error()
		return r, nil
	}
	r.Status = StatusReady
	if s.Present {
		p := s.Position
		r.Position = &p
		r.Status = StatusApplied
	}
	if b, err := c.loadBackup(); err == nil {
		r.HasBackup = true
		if err := c.checkBackupComponent(b); err != nil {
			r.Status, r.Reason = StatusUnverified, err.Error()
			return r, nil
		}
		if b.Pending != nil || !snapshotEqual(s, b.Expected) {
			r.Status, r.Reason = StatusRollback, "位置事务未完成或状态被外部改动；不会覆盖，请先检查并恢复备份。"
			return r, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		r.Status, r.Reason = StatusUnverified, fmt.Sprintf("备份无法核验：%v", err)
		return r, nil
	}
	r.Reason = "组件元数据完整且本地文件匹配指定哈希；上游能力和来源仍需独立审计。范围仅限后端声明的 Windows 位置接口。"
	return r, nil
}

func (c Controller) Apply(ctx context.Context, p Position) (Result, error) {
	transactionMu.Lock()
	defer transactionMu.Unlock()
	if err := p.valid(); err != nil {
		return Result{}, err
	}
	if c.Admin == nil || !c.Admin() {
		return Result{}, errors.New("需要明确的管理员权限；未授权不会影响隧道")
	}
	got, err := c.componentCheck()
	if err != nil {
		return Result{Report: Report{Status: StatusUnverified, Reason: err.Error(), Component: c.Component, ComponentSHA: got}}, err
	}
	if c.Backend == nil {
		return Result{}, errors.New("位置后端未接入；未执行任何修改")
	}
	atomic, ok := c.Backend.(AtomicBackend)
	if !ok {
		return Result{}, ErrAtomicUnsupported
	}
	if err := atomic.CheckAtomic(ctx); err != nil {
		return Result{}, fmt.Errorf("atomic transaction capability: %w", err)
	}
	before, err := c.readSnapshot(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("read before apply: %w", err)
	}
	b, err := c.loadBackup()
	if errors.Is(err, os.ErrNotExist) {
		b = backup{Version: backupVersion, Component: c.Component, Snapshot: cloneSnapshot(before), Expected: cloneSnapshot(before)}
	} else if err != nil {
		return Result{}, fmt.Errorf("existing backup: %w", err)
	} else {
		if err := c.checkBackupComponent(b); err != nil {
			return Result{}, err
		}
		if b.Pending != nil {
			return Result{}, errors.New("previous location transaction is incomplete; restore its backup first")
		}
		if !snapshotEqual(before, b.Expected) {
			return Result{}, fmt.Errorf("refusing to overwrite external location state: %w", ErrStateConflict)
		}
	}
	target, err := atomic.PrepareApply(ctx, cloneSnapshot(before), p)
	if err != nil {
		return Result{}, fmt.Errorf("prepare atomic apply: %w", err)
	}
	target = cloneSnapshot(target)
	if !target.Present || !target.Position.equal(p) || len(target.Raw) == 0 {
		return Result{}, errors.New("prepared target must contain the exact position and durable ownership token")
	}
	b.Pending = snapshotPointer(target)
	if err := c.writeBackup(b); err != nil {
		return Result{}, fmt.Errorf("backup before apply: %w", err)
	}
	if err := atomic.CompareAndSwap(ctx, cloneSnapshot(before), cloneSnapshot(target)); err != nil {
		return c.rollback(ctx, atomic, b, before, target, got, fmt.Errorf("apply: %w", err))
	}
	after, err := c.readSnapshot(ctx)
	if err != nil {
		return c.rollback(ctx, atomic, b, before, target, got, fmt.Errorf("read after apply: %w", err))
	}
	if !snapshotEqual(after, target) {
		return c.rollback(ctx, atomic, b, before, target, got, errors.New("应用后读回与完整目标/所有权不一致"))
	}
	b.Expected, b.Pending = cloneSnapshot(after), nil
	if err := c.writeBackup(b); err != nil {
		return c.rollback(ctx, atomic, b, before, target, got, fmt.Errorf("commit location receipt: %w", err))
	}
	q := p
	return Result{Changed: !snapshotEqual(before, target), Report: Report{Status: StatusApplied, Reason: "已原子应用并独立读回；只影响后端声明的 Windows 位置接口。", Component: c.Component, ComponentSHA: got, HasBackup: true, Position: &q}}, nil
}

func (c Controller) rollback(ctx context.Context, atomic AtomicBackend, b backup, before, attempted Snapshot, got string, cause error) (Result, error) {
	// A cancelled apply must not prevent a bounded cleanup attempt.
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	reason := fmt.Sprintf("应用失败，最初备份已保留：%v", cause)
	// The OS-side CAS must reject an external change even if it occurs after
	// the failed apply/readback. Never restore unconditionally after a Read.
	restoreErr := atomic.CompareAndSwap(cleanup, cloneSnapshot(attempted), cloneSnapshot(before))
	restored, readErr := c.readSnapshot(cleanup)
	if readErr == nil && snapshotEqual(restored, before) {
		b.Expected, b.Pending = cloneSnapshot(before), nil
		if err := c.writeBackup(b); err != nil {
			reason += "; 已恢复但事务记录写入失败，备份仍保留"
		} else {
			reason += "; 已恢复本次操作前状态并独立读回"
		}
	} else if restoreErr != nil {
		reason += "; 条件恢复未完成，未覆盖不匹配的外部状态；备份保留"
	} else {
		reason += "; 自动恢复未通过独立读回，请检查备份和会话状态"
	}
	return Result{Report: Report{Status: StatusRollback, Reason: reason, Component: c.Component, ComponentSHA: got, HasBackup: true}}, cause
}

func (c Controller) Restore(ctx context.Context) (Result, error) {
	transactionMu.Lock()
	defer transactionMu.Unlock()
	if c.Admin == nil || !c.Admin() {
		return Result{}, errors.New("需要明确的管理员权限；未授权不会影响隧道")
	}
	if c.Backend == nil {
		return Result{}, errors.New("位置后端未接入；未执行恢复")
	}
	atomic, ok := c.Backend.(AtomicBackend)
	if !ok {
		return Result{}, ErrAtomicUnsupported
	}
	if err := atomic.CheckAtomic(ctx); err != nil {
		return Result{}, fmt.Errorf("atomic transaction capability: %w", err)
	}
	b, err := c.loadBackup()
	if err != nil {
		return Result{}, err
	}
	if _, err := c.componentCheck(); err != nil {
		return Result{}, fmt.Errorf("component verification before restore: %w", err)
	}
	if err := c.checkBackupComponent(b); err != nil {
		return Result{}, err
	}
	before, err := c.readSnapshot(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("read before restore: %w", err)
	}
	if !snapshotEqual(before, b.Expected) && (b.Pending == nil || !snapshotEqual(before, *b.Pending)) {
		return Result{}, fmt.Errorf("refusing to restore over external location state: %w", ErrStateConflict)
	}
	changed := !snapshotEqual(before, b.Snapshot)
	if !changed {
		b.Expected, b.Pending = cloneSnapshot(before), nil
		if err := c.writeBackup(b); err != nil {
			return Result{}, fmt.Errorf("record unchanged restore: %w", err)
		}
		return Result{Report: Report{Status: StatusReady, Reason: "独立读回已为最初状态；备份仍保留。", Component: b.Component, HasBackup: true}}, nil
	}
	// An interrupted apply may have left the complete Pending target active.
	// Preserve that accepted source before replacing Pending with the restore
	// target, so an interruption of this restore remains recoverable as well.
	b.Expected = cloneSnapshot(before)
	b.Pending = snapshotPointer(b.Snapshot)
	if err := c.writeBackup(b); err != nil {
		return Result{}, fmt.Errorf("backup before restore: %w", err)
	}
	if err := atomic.CompareAndSwap(ctx, cloneSnapshot(before), cloneSnapshot(b.Snapshot)); err != nil {
		return Result{}, fmt.Errorf("restore: %w", err)
	}
	after, err := c.readSnapshot(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("read after restore: %w", err)
	}
	if !snapshotEqual(after, b.Snapshot) {
		return Result{}, errors.New("恢复后读回不一致；备份已保留")
	}
	b.Expected, b.Pending = cloneSnapshot(after), nil
	if err := c.writeBackup(b); err != nil {
		return Result{}, fmt.Errorf("record restored state: %w", err)
	}
	return Result{Changed: changed, Report: Report{Status: StatusReady, Reason: "已恢复并独立读回最初状态；备份仍保留。", Component: b.Component, HasBackup: true}}, nil
}

func snapshotEqual(a, b Snapshot) bool {
	return a.Present == b.Present && (!a.Present || a.Position.equal(b.Position)) && bytes.Equal(a.Raw, b.Raw)
}

func cloneSnapshot(s Snapshot) Snapshot {
	s.Raw = bytes.Clone(s.Raw)
	return s
}

func snapshotPointer(s Snapshot) *Snapshot {
	copy := cloneSnapshot(s)
	return &copy
}

func (c Controller) readSnapshot(ctx context.Context) (Snapshot, error) {
	s, err := c.Backend.Read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	s = cloneSnapshot(s)
	if err := validSnapshot(s); err != nil {
		return Snapshot{}, fmt.Errorf("invalid provider snapshot: %w", err)
	}
	return s, nil
}

func validSnapshot(s Snapshot) error {
	if s.Present {
		return s.Position.valid()
	}
	return nil
}

func (c Controller) checkBackupComponent(b backup) error {
	a, current := b.Component, c.Component
	if a.ID != current.ID || a.Upstream != current.Upstream || a.Version != current.Version ||
		!strings.EqualFold(strings.TrimSpace(a.License), strings.TrimSpace(current.License)) ||
		!strings.EqualFold(a.SHA256, current.SHA256) || a.RequiresAdmin != current.RequiresAdmin {
		return errors.New("backup component identity, source, version, license or hash does not match the verified component")
	}
	return nil
}
func (c Controller) loadBackup() (backup, error) {
	if err := validateBackupPath(c.BackupPath); err != nil {
		return backup{}, err
	}
	data, err := os.ReadFile(c.BackupPath)
	if err != nil {
		return backup{}, fmt.Errorf("read backup: %w", err)
	}
	var b backup
	if err := json.Unmarshal(data, &b); err != nil {
		return backup{}, fmt.Errorf("decode backup: %w", err)
	}
	if b.Version != backupVersion {
		return backup{}, errors.New("unsupported backup version")
	}
	for _, snapshot := range []Snapshot{b.Snapshot, b.Expected} {
		if err := validSnapshot(snapshot); err != nil {
			return backup{}, fmt.Errorf("invalid backup state: %w", err)
		}
	}
	if b.Pending != nil {
		if err := validSnapshot(*b.Pending); err != nil {
			return backup{}, fmt.Errorf("invalid pending backup state: %w", err)
		}
	}
	return b, nil
}
func (c Controller) writeBackup(b backup) error {
	if err := validateBackupPath(c.BackupPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.BackupPath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(b)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.BackupPath), ".location-backup-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, c.BackupPath)
}
func validateBackupPath(path string) error {
	if err := validateLocalAbsolutePath(path); err != nil {
		return fmt.Errorf("backup path: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("backup path cannot be a symlink")
	}
	return nil
}
func isUNC(path string) bool { return strings.HasPrefix(strings.ReplaceAll(path, "/", `\`), `\\`) }

// Check lexical hazards before Lstat/Stat/Open: even a read-only Inspect must
// not touch a UNC share or a Windows device/alternate-stream namespace.
func validateLocalAbsolutePath(path string) error {
	if path == "" || !filepath.IsAbs(path) || isUNC(path) {
		return errors.New("path must be a local absolute path without a device namespace")
	}
	if runtime.GOOS != "windows" {
		return nil
	}
	if len(path) < 3 || !((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) ||
		path[1] != ':' || (path[2] != '\\' && path[2] != '/') {
		return errors.New("Windows path must use an absolute local drive letter")
	}
	tail := strings.ReplaceAll(path[3:], "/", `\`)
	if strings.ContainsAny(tail, ":*?\"<>|\x00") {
		return errors.New("Windows path contains an alternate stream or invalid character")
	}
	for _, part := range strings.Split(tail, `\`) {
		if part == "." || part == ".." || strings.TrimRight(part, " .") != part {
			return errors.New("Windows path contains ambiguous traversal or trailing characters")
		}
		name := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if name == "CON" || name == "PRN" || name == "AUX" || name == "NUL" ||
			(len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9') {
			return errors.New("Windows path contains a reserved device name")
		}
	}
	return nil
}
func isHash(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
