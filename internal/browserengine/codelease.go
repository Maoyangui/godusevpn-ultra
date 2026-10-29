package browserengine

import (
	"context"
	"path/filepath"
	"regexp"
	"sync"
)

var instanceIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// CodeLease binds a fresh official-origin archive's in-memory tree to held
// no-write/no-delete-sharing file handles. A writable cached receipt cannot
// authorize code here. Each session has a distinct executable path so its WFP
// port restriction cannot conflict with another profile of the same version.
type CodeLease struct {
	instanceID, directory, mainPath string
	programs                        []string
	release                         func()
	once                            sync.Once
}

func (l *CodeLease) InstanceID() string { return l.instanceID }
func (l *CodeLease) Directory() string  { return l.directory }
func (l *CodeLease) MainPath() string   { return l.mainPath }
func (l *CodeLease) Programs() []string { return append([]string(nil), l.programs...) }
func (l *CodeLease) Close() {
	if l != nil {
		l.once.Do(func() {
			if l.release != nil {
				l.release()
			}
		})
	}
}

func (l *CodeLease) holdUsage(usage func()) {
	release := l.release
	l.release = func() {
		if release != nil {
			release()
		}
		usage()
	}
}

// InstanceDirectory derives a session path from this manager's fixed user
// store. It does not create a directory or authorize execution of its contents.
func (m *Manager) InstanceDirectory(instanceID string) (string, error) {
	if m == nil || !instanceIDPattern.MatchString(instanceID) || m.directory == "" {
		return "", ErrInput
	}
	return filepath.Join(filepath.Dir(m.directory), "BrowserInstances", instanceID), nil
}

// AcquireCodeLease currently re-authenticates the complete archive through its
// fixed official HTTPS origin on every explicit session start. The protected
// machine-template cache optimization is a separate unfinished helper action;
// this method does not silently trust an ordinary-user cache manifest instead.
func (m *Manager) AcquireCodeLease(ctx context.Context, id, version, instanceID string) (*CodeLease, error) {
	if !validSelection(id, version) || !instanceIDPattern.MatchString(instanceID) {
		return nil, ErrInput
	}
	if m.client == nil {
		return nil, ErrTransport
	}
	usage, err := m.acquireVersionUsage(id, version)
	if err != nil {
		return nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			usage()
		}
	}()
	instanceRoot, err := m.InstanceDirectory(instanceID)
	if err != nil {
		return nil, err
	}
	instance := &Manager{directory: instanceRoot, sessionInstance: true, client: m.client, acquire: m.acquire, verifyVersion: m.verifyVersion, verifyRoot: m.verifyRoot}
	fresh, err := instance.DownloadVersion(ctx, id, version)
	if err != nil {
		return nil, err
	}
	// fresh.Receipt came from this download's authenticated bytes in memory,
	// rather than reloading its ordinary-user writable disk copy.
	lease, err := pinCodeTree(instanceRoot, id, version, instanceID, fresh.Receipt)
	if err != nil {
		return nil, err
	}
	lease.holdUsage(usage)
	transferred = true
	return lease, nil
}
