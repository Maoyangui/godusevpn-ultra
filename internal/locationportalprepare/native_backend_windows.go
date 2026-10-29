//go:build windows && (amd64 || arm64)

package locationportalprepare

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Maoyangui/godusevpn/internal/localstate"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	"github.com/Maoyangui/godusevpn/internal/locationportalgate"
	"github.com/Maoyangui/godusevpn/internal/privacy"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const webManagementService = "WebManagement"
const webManagementRegistry = `SOFTWARE\Microsoft\Windows\CurrentVersion\WebManagement\Service`
const developerModeRegistry = `SOFTWARE\Microsoft\Windows\CurrentVersion\AppModelUnlock`
const preparationJournalName = "location-portal-preparation.v1.json"
const preparationLockName = "location-portal-preparation.v1.lock"

type nativePreparationBackend struct{ owner, machineDir string }

func NewNativeManager(owner string) (*Manager, error) {
	if !validOwnerSID(owner) {
		return nil, ErrUnsupported
	}
	base, e := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if e != nil {
		return nil, ErrUnsupported
	}
	if !verifyLocalDirectoryShape(base) {
		return nil, ErrUnsupported
	}
	dir := filepath.Join(base, "godusevpn-ultra", "privacy-machine")
	if privacy.ValidateMachineStoreDirectory(dir) != nil {
		return nil, ErrUnsupported
	}
	g, e := locationportalgate.NewGate()
	if e != nil {
		return nil, e
	}
	b := &nativePreparationBackend{owner: owner, machineDir: dir}
	return newManager(owner, false, b, g, b.openStore), nil
}
func verifyLocalDirectoryShape(p string) bool {
	return len(p) > 3 && len(filepath.VolumeName(p)) == 2 && filepath.IsAbs(p) && filepath.Clean(p) == p && !strings.HasPrefix(p, `\\`) && !strings.Contains(p[2:], ":")
}

func regValue(k registry.Key, name string) (registryValue, error) {
	v, typ, e := k.GetIntegerValue(name)
	if errors.Is(e, registry.ErrNotExist) {
		return registryValue{}, nil
	}
	if e != nil || typ != registry.DWORD {
		return registryValue{}, ErrReadback
	}
	return registryValue{true, uint32(v)}, nil
}
func (b *nativePreparationBackend) snapshot(ctx context.Context) (systemState, error) {
	if err := ctx.Err(); err != nil {
		return systemState{}, err
	}
	var s systemState
	state, e := nativeCapabilityState(ctx)
	if e != nil {
		return s, e
	}
	s.Capability = state
	k, e := registry.OpenKey(registry.LOCAL_MACHINE, developerModeRegistry, registry.QUERY_VALUE)
	if e == nil {
		s.DeveloperMode, e = regValue(k, "AllowDevelopmentWithoutDevLicense")
		k.Close()
	} else if !errors.Is(e, registry.ErrNotExist) {
		return s, e
	}
	k, e = registry.OpenKey(registry.LOCAL_MACHINE, webManagementRegistry, registry.QUERY_VALUE)
	if e == nil {
		s.PortalKeyPresent = true
		for name, out := range map[string]*registryValue{"UseDynamicPorts": &s.DynamicPorts, "HttpPort": &s.HTTPPort, "HttpsPort": &s.HTTPSPort, "UseDefaultAuthorizer": &s.Authorizer} {
			*out, e = regValue(k, name)
			if e != nil {
				k.Close()
				return s, e
			}
		}
		k.Close()
	} else if !errors.Is(e, registry.ErrNotExist) {
		return s, e
	}
	s.Service, e = serviceSnapshot(webManagementService)
	if e != nil {
		return s, e
	}
	s.SSHBroker, e = serviceSnapshot("SshBroker")
	if e != nil {
		return s, e
	}
	s.SSHProxy, e = serviceSnapshot("SshProxy")
	if e != nil {
		return s, e
	}
	return s, nil
}
func serviceSnapshot(name string) (serviceState, error) {
	m, e := mgr.Connect()
	if e != nil {
		return serviceState{}, e
	}
	defer m.Disconnect()
	s, e := m.OpenService(name)
	if errors.Is(e, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return serviceState{}, nil
	}
	if e != nil {
		return serviceState{}, e
	}
	defer s.Close()
	c, e := s.Config()
	if e != nil {
		return serviceState{}, e
	}
	if name == webManagementService {
		binary := strings.Trim(strings.TrimSpace(c.BinaryPathName), `"`)
		account := strings.TrimSpace(c.ServiceStartName)
		if !verifyLocalSystemPath(binary) || !(strings.EqualFold(account, "LocalSystem") || strings.EqualFold(account, `NT AUTHORITY\LocalSystem`)) {
			return serviceState{}, ErrUnsupported
		}
	}
	q, e := s.Query()
	if e != nil {
		return serviceState{}, e
	}
	return serviceState{true, c.StartType, uint32(q.State)}, nil
}
func (b *nativePreparationBackend) addCapability(ctx context.Context) (bool, error) {
	return nativeAddCapability(ctx)
}
func (b *nativePreparationBackend) removeCapability(ctx context.Context) (bool, error) {
	return nativeRemoveCapability(ctx)
}
func (b *nativePreparationBackend) setDeveloperMode(_ context.Context, v registryValue) error {
	k, e := registry.OpenKey(registry.LOCAL_MACHINE, developerModeRegistry, registry.SET_VALUE)
	if errors.Is(e, registry.ErrNotExist) && v.Present {
		k, _, e = registry.CreateKey(registry.LOCAL_MACHINE, developerModeRegistry, registry.SET_VALUE)
	}
	if e != nil {
		return e
	}
	defer k.Close()
	if !v.Present {
		return k.DeleteValue("AllowDevelopmentWithoutDevLicense")
	}
	return k.SetDWordValue("AllowDevelopmentWithoutDevLicense", v.Value)
}
func (b *nativePreparationBackend) configure(ctx context.Context, c credentials) error {
	k, e := registry.OpenKey(registry.LOCAL_MACHINE, webManagementRegistry, registry.SET_VALUE)
	if e != nil {
		return e
	}
	defer k.Close()
	for n, v := range map[string]uint32{"UseDynamicPorts": 0, "HttpPort": HTTPPort, "HttpsPort": HTTPSPort, "UseDefaultAuthorizer": 1} {
		if e = k.SetDWordValue(n, v); e != nil {
			return e
		}
	}
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	path := filepath.Join(systemDir, "WebManagement.exe")
	if !verifyLocalSystemPath(path) {
		return ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := execCommandFixedContext(ctx, path, "-Credentials", c.username, c.password)
	if err := cmd.Run(); err != nil {
		return ErrReadback
	}
	return nil
}
func (b *nativePreparationBackend) start(ctx context.Context) error {
	return serviceChange(ctx, webManagementService, true)
}
func (b *nativePreparationBackend) stopDisabled(ctx context.Context) error {
	return serviceChange(ctx, webManagementService, false)
}
func (b *nativePreparationBackend) secureNewSSH(_ context.Context, before systemState) error {
	after, e := b.snapshot(context.Background())
	if e != nil {
		return e
	}
	if !safeNewSSH(before, after) {
		return ErrConflict
	}
	return nil
}
func (b *nativePreparationBackend) restoreSettings(_ context.Context, before systemState) error {
	k, e := registry.OpenKey(registry.LOCAL_MACHINE, webManagementRegistry, registry.SET_VALUE)
	if e != nil && before.PortalKeyPresent {
		return e
	}
	if e == nil {
		defer k.Close()
		for n, v := range map[string]registryValue{"UseDynamicPorts": before.DynamicPorts, "HttpPort": before.HTTPPort, "HttpsPort": before.HTTPSPort, "UseDefaultAuthorizer": before.Authorizer} {
			if v.Present {
				if e = k.SetDWordValue(n, v.Value); e != nil {
					return e
				}
			} else {
				_ = k.DeleteValue(n)
			}
		}
	}
	return b.setDeveloperMode(context.Background(), before.DeveloperMode)
}
func (b *nativePreparationBackend) certificate(ctx context.Context) (string, error) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://127.0.0.1:58443/", nil)
	if e != nil {
		return "", e
	}
	resp, e := (&http.Client{Transport: tr, Timeout: 6 * time.Second}).Do(req)
	if e != nil {
		return "", ErrReadback
	}
	defer resp.Body.Close()
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return "", ErrReadback
	}
	sum := sha256.Sum256(resp.TLS.PeerCertificates[0].Raw)
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return hex.EncodeToString(sum[:]), nil
}
func (b *nativePreparationBackend) verifyConnection(ctx context.Context, c locationportal.Config) error {
	cl, e := locationportal.NewClient(c)
	if e != nil {
		return e
	}
	defer cl.Close()
	_, e = cl.Check(ctx)
	return e
}
func (b *nativePreparationBackend) overrideDisabled(ctx context.Context, c locationportal.Config) (bool, error) {
	cl, e := locationportal.NewClient(c)
	if e != nil {
		return false, e
	}
	defer cl.Close()
	on, e := cl.Override(ctx)
	return !on, e
}
func (b *nativePreparationBackend) seal(ctx context.Context, c locationportal.Config, o, t, p string) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return nativeSealConfig(ctx, c, o, t, p)
}
func (b *nativePreparationBackend) unseal(ctx context.Context, box []byte, o, t, p string) (locationportal.Config, error) {
	if e := ctx.Err(); e != nil {
		return locationportal.Config{}, e
	}
	return nativeUnsealConfig(ctx, box, o, t, p)
}

type nativePrepStore struct {
	release       func()
	lock, journal *os.File
	overlap       windows.Overlapped
	locked        bool
}

func (b *nativePreparationBackend) openStore(create bool) (store, error) {
	release, e := localstate.AcquireDirectory(b.machineDir, false)
	if e != nil {
		return nil, e
	}
	lock, e := localstate.OpenRegular(filepath.Join(b.machineDir, preparationLockName), create)
	if e != nil {
		release()
		return nil, e
	}
	var overlap windows.Overlapped
	if e = windows.LockFileEx(windows.Handle(lock.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, ^uint32(0), ^uint32(0), &overlap); e != nil {
		lock.Close()
		release()
		return nil, ErrBusy
	}
	j, e := localstate.OpenRegular(filepath.Join(b.machineDir, preparationJournalName), create)
	if e != nil {
		_ = windows.UnlockFileEx(windows.Handle(lock.Fd()), 0, ^uint32(0), ^uint32(0), &overlap)
		lock.Close()
		release()
		return nil, e
	}
	return &nativePrepStore{release: release, lock: lock, journal: j, overlap: overlap, locked: true}, nil
}
func (s *nativePrepStore) load() (*journal, error) {
	if _, e := s.journal.Seek(0, 0); e != nil {
		return nil, ErrStore
	}
	raw, e := io.ReadAll(io.LimitReader(s.journal, maxJournal+1))
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil || len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > maxJournal {
		return nil, ErrStore
	}
	return decodeJournal(raw)
}
func (s *nativePrepStore) save(j *journal) error {
	raw, e := encodeJournal(j)
	if e != nil {
		return e
	}
	if _, e = s.journal.Seek(0, 0); e != nil {
		return ErrStore
	}
	if e = s.journal.Truncate(0); e != nil {
		return ErrStore
	}
	if _, e = s.journal.Write(raw); e != nil {
		return ErrStore
	}
	return s.journal.Sync()
}
func (s *nativePrepStore) close() error {
	var e error
	if s.journal != nil {
		e = s.journal.Close()
	}
	if s.locked && s.lock != nil {
		_ = windows.UnlockFileEx(windows.Handle(s.lock.Fd()), 0, ^uint32(0), ^uint32(0), &s.overlap)
		s.locked = false
	}
	if s.lock != nil && e == nil {
		e = s.lock.Close()
	}
	if s.release != nil {
		s.release()
	}
	return e
}

func verifyLocalSystemPath(path string) bool {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return false
	}
	expected := filepath.Join(systemDir, "WebManagement.exe")
	a, e := filepath.EvalSymlinks(path)
	if e != nil {
		return false
	}
	b, e := filepath.EvalSymlinks(expected)
	if e != nil || !strings.EqualFold(a, b) {
		return false
	}
	i, e := os.Stat(a)
	return e == nil && i.Mode().IsRegular()
}
func execCommandFixed(path string, args ...string) *exec.Cmd {
	c := exec.Command(path, args...)
	c.Dir = filepath.Dir(path)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c
}
func execCommandFixedContext(ctx context.Context, path string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, path, args...)
	c.Dir = filepath.Dir(path)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c
}
func serviceChange(ctx context.Context, name string, start bool) error {
	m, e := mgr.Connect()
	if e != nil {
		return e
	}
	defer m.Disconnect()
	s, e := m.OpenService(name)
	if errors.Is(e, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return ErrUnsupported
	}
	if e != nil {
		return e
	}
	defer s.Close()
	cfg, e := s.Config()
	if e != nil {
		return e
	}
	if !start {
		cfg.StartType = mgr.StartDisabled
		if e = s.UpdateConfig(cfg); e != nil {
			return e
		}
		q, e := s.Query()
		if e != nil {
			return e
		}
		if q.State == svc.Running || q.State == svc.StartPending {
			if _, e = s.Control(svc.Stop); e != nil {
				return e
			}
		}
	} else {
		if cfg.StartType == mgr.StartDisabled {
			cfg.StartType = mgr.StartManual
			if e = s.UpdateConfig(cfg); e != nil {
				return e
			}
		}
		if e = s.Start(); e != nil && !errors.Is(e, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return e
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if e := ctx.Err(); e != nil {
			return e
		}
		q, e := s.Query()
		if e != nil {
			return e
		}
		if start && q.State == svc.Running {
			return nil
		}
		if !start && q.State == svc.Stopped {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ErrReadback
}
