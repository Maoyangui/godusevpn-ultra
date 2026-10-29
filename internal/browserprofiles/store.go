package browserprofiles

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxJournal = 8 << 20
	maxFrame   = 48 << 10
	maxRecords = 128
	extension  = ".ubp"
)

var journalMagic = []byte("GodUseVPN Ultra browser profile journal v1\n")
var commitMagic = []byte("UBPCMT1\n")

type recordFile interface {
	io.ReadWriteSeeker
	Stat() (os.FileInfo, error)
	Sync() error
	Close() error
}
type directory interface {
	names() ([]string, error)
	open(string, bool, bool) (recordFile, error)
	remove(recordFile) error
	close()
}
type storage interface{ begin(bool) (directory, error) }
type Manager struct {
	storage storage
	mu      sync.Mutex
	entropy io.Reader
	now     func() time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	closed  atomic.Bool
}

func New() (*Manager, error) {
	fs, e := platformStorage()
	if e != nil {
		return nil, e
	}
	return newManager(fs), nil
}
func newManager(fs storage) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{storage: fs, entropy: rand.Reader, now: time.Now, ctx: ctx, cancel: cancel}
}
func (m *Manager) Close() {
	if m.closed.CompareAndSwap(false, true) {
		m.cancel()
	}
}
func (m *Manager) active(ctx context.Context) error {
	if ctx == nil {
		return ErrInput
	}
	if m.closed.Load() {
		return ErrClosed
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
func hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func (m *Manager) nonce() (string, error) {
	var b [16]byte
	if _, e := io.ReadFull(m.entropy, b[:]); e != nil {
		return "", ErrStore
	}
	return hex.EncodeToString(b[:]), nil
}

type commit struct {
	SchemaVersion int             `json:"schemaVersion"`
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Config        json.RawMessage `json:"config"`
	Engine        EngineSelection `json:"engine"`
	UpdatedAt     time.Time       `json:"updatedAt"`
	Previous      string          `json:"previousRevision"`
	Nonce         string          `json:"nonce"`
}
type loaded struct {
	record     Record
	raw        []byte
	lastCommit string
}

func readJournal(f recordFile, id string) (loaded, error) {
	var value loaded
	stat, e := f.Stat()
	if e != nil || stat.Size() < int64(len(journalMagic)) || stat.Size() > maxJournal {
		return value, ErrStore
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return value, ErrStore
	}
	data, e := io.ReadAll(io.LimitReader(f, maxJournal+1))
	if e != nil || len(data) > maxJournal || !bytes.HasPrefix(data, journalMagic) {
		return value, ErrStore
	}
	offset := len(journalMagic)
	previous := ""
	found := false
	recovery := false
	for offset < len(data) {
		if len(data)-offset < 4 {
			recovery = true
			break
		}
		n := int(binary.BigEndian.Uint32(data[offset:]))
		if n < 2 || n > maxFrame {
			return value, ErrStore
		}
		size := 4 + n + 32 + 4 + len(commitMagic)
		if size > len(data)-offset {
			recovery = true
			break
		}
		payload := data[offset+4 : offset+4+n]
		checksum := data[offset+4+n : offset+4+n+32]
		sum := sha256.Sum256(payload)
		if !bytes.Equal(checksum, sum[:]) || binary.BigEndian.Uint32(data[offset+4+n+32:]) != uint32(n) || !bytes.Equal(data[offset+size-len(commitMagic):offset+size], commitMagic) {
			return value, ErrStore
		}
		var entry commit
		if strictJSON(payload, &entry) != nil || entry.SchemaVersion != 1 || entry.ID != id || !validName(entry.Name) || !validateEngine(entry.Engine) || entry.Previous != previous || entry.UpdatedAt.IsZero() || !idPattern.MatchString("p-"+entry.Nonce) {
			return value, ErrStore
		}
		config, e := canonicalDraft(entry.Config, id)
		if e != nil || !bytes.Equal(config, entry.Config) {
			return value, ErrStore
		}
		value.record = Record{SchemaVersion: 1, ID: id, Name: entry.Name, Config: config, Engine: entry.Engine, UpdatedAt: entry.UpdatedAt, RequiresLaunchValidation: true}
		previous = hex.EncodeToString(sum[:])
		found = true
		offset += size
	}
	if !found {
		return value, ErrStore
	}
	value.record.Revision = hash(data)
	value.record.RecoveryNeeded = recovery
	value.raw = data
	value.lastCommit = previous
	return value, nil
}
func encodeCommit(entry commit) ([]byte, error) {
	payload, e := json.Marshal(entry)
	if e != nil || len(payload) > maxFrame {
		return nil, ErrInput
	}
	result := make([]byte, 4+len(payload)+32+4+len(commitMagic))
	binary.BigEndian.PutUint32(result, uint32(len(payload)))
	copy(result[4:], payload)
	sum := sha256.Sum256(payload)
	copy(result[4+len(payload):], sum[:])
	binary.BigEndian.PutUint32(result[4+len(payload)+32:], uint32(len(payload)))
	copy(result[len(result)-len(commitMagic):], commitMagic)
	return result, nil
}
func (m *Manager) watchFile(ctx context.Context, f recordFile) func() {
	stopCaller := context.AfterFunc(ctx, func() { _ = f.Close() })
	stopStore := context.AfterFunc(m.ctx, func() { _ = f.Close() })
	return func() { stopCaller(); stopStore() }
}
func validNames(names []string) ([]string, error) {
	if len(names) > maxRecords {
		return nil, ErrFull
	}
	result := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if !strings.HasSuffix(name, extension) || !idPattern.MatchString(strings.TrimSuffix(name, extension)) || seen[strings.ToLower(name)] {
			return nil, ErrStore
		}
		seen[strings.ToLower(name)] = true
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func (m *Manager) List(ctx context.Context) (ListResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := ListResult{Records: []Summary{}}
	if e := m.active(ctx); e != nil {
		return result, e
	}
	dir, e := m.storage.begin(false)
	if errors.Is(e, os.ErrNotExist) {
		return result, nil
	}
	if e != nil {
		return result, safeError(e)
	}
	defer dir.close()
	names, e := dir.names()
	if e != nil {
		return result, safeError(e)
	}
	names, e = validNames(names)
	if e != nil {
		return result, e
	}
	for _, name := range names {
		if e = m.active(ctx); e != nil {
			return result, e
		}
		id := strings.TrimSuffix(name, extension)
		f, e := dir.open(name, false, false)
		if e != nil {
			return result, safeError(e)
		}
		stop := m.watchFile(ctx, f)
		value, e := readJournal(f, id)
		stop()
		_ = f.Close()
		if e != nil {
			return result, e
		}
		r := value.record
		result.Records = append(result.Records, Summary{r.ID, r.Name, r.Revision, r.Engine, r.UpdatedAt, r.RecoveryNeeded})
	}
	if e = m.active(ctx); e != nil {
		return ListResult{Records: []Summary{}}, e
	}
	return result, nil
}
func (m *Manager) Load(ctx context.Context, id string) (Record, error) {
	if !idPattern.MatchString(id) {
		return Record{}, ErrInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.active(ctx); e != nil {
		return Record{}, e
	}
	dir, e := m.storage.begin(false)
	if e != nil {
		return Record{}, safeError(e)
	}
	defer dir.close()
	f, e := dir.open(id+extension, false, false)
	if e != nil {
		return Record{}, safeError(e)
	}
	defer f.Close()
	stop := m.watchFile(ctx, f)
	defer stop()
	value, e := readJournal(f, id)
	if e != nil {
		return Record{}, e
	}
	if e = m.active(ctx); e != nil {
		return Record{}, e
	}
	return value.record, nil
}
func (m *Manager) Save(ctx context.Context, data []byte) (Record, error) {
	request, e := decodeSave(data)
	if e != nil {
		return Record{}, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e = m.active(ctx); e != nil {
		return Record{}, e
	}
	create := request.ID == ""
	id := request.ID
	if create {
		nonce, e := m.nonce()
		if e != nil {
			return Record{}, e
		}
		id = "p-" + nonce
	}
	config, e := canonicalDraft(request.Config, id)
	if e != nil {
		return Record{}, e
	}
	dir, e := m.storage.begin(true)
	if e != nil {
		return Record{}, safeError(e)
	}
	defer dir.close()
	if create {
		names, e := dir.names()
		if e != nil {
			return Record{}, safeError(e)
		}
		names, e = validNames(names)
		if e != nil {
			return Record{}, e
		}
		if len(names) >= maxRecords {
			return Record{}, ErrFull
		}
	}
	f, e := dir.open(id+extension, true, create)
	if e != nil {
		return Record{}, safeError(e)
	}
	defer f.Close()
	// Writes retain their verified handle through cleanup/commit. Closing it
	// asynchronously would lose the ability to delete a failed fresh record by
	// handle and could leave an unusable partial create. Bounded local I/O is
	// followed by cancellation checks; an interrupted caller may need to reload
	// the revision to distinguish a durable commit from an uncommitted attempt.
	committed := false
	if create {
		defer func() {
			if !committed {
				_ = dir.remove(f)
			}
		}()
	}
	previous := ""
	var existing []byte
	if !create {
		loaded, e := readJournal(f, id)
		if e != nil {
			return Record{}, e
		}
		if loaded.record.Revision != request.ExpectedRevision {
			return Record{}, ErrConflict
		}
		if loaded.record.RecoveryNeeded {
			return Record{}, ErrStore
		}
		existing = loaded.raw
		previous = loaded.lastCommit
	} else {
		existing = journalMagic
	}
	nonce, e := m.nonce()
	if e != nil {
		return Record{}, e
	}
	entry := commit{1, id, request.Name, config, request.Engine, m.now().UTC(), previous, nonce}
	frame, e := encodeCommit(entry)
	if e != nil {
		return Record{}, e
	}
	if len(existing)+len(frame) > maxJournal {
		return Record{}, ErrFull
	}
	if e = m.active(ctx); e != nil {
		return Record{}, e
	}
	if create {
		if n, e := f.Write(journalMagic); e != nil || n != len(journalMagic) {
			return Record{}, ErrStore
		}
	} else if _, e = f.Seek(0, io.SeekEnd); e != nil {
		return Record{}, ErrStore
	}
	if n, e := f.Write(frame); e != nil || n != len(frame) {
		return Record{}, ErrStore
	}
	if f.Sync() != nil {
		return Record{}, ErrStore
	}
	committed = true
	if e = m.active(ctx); e != nil {
		return Record{}, e
	}
	return Record{SchemaVersion: 1, ID: id, Name: entry.Name, Revision: hash(append(append([]byte{}, existing...), frame...)), Config: config, Engine: entry.Engine, UpdatedAt: entry.UpdatedAt, RequiresLaunchValidation: true}, nil
}
func (m *Manager) Delete(ctx context.Context, id, revision string) (DeleteResult, error) {
	if !idPattern.MatchString(id) || !revisionPattern.MatchString(revision) {
		return DeleteResult{}, ErrInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.active(ctx); e != nil {
		return DeleteResult{}, e
	}
	dir, e := m.storage.begin(false)
	if e != nil {
		return DeleteResult{}, safeError(e)
	}
	defer dir.close()
	f, e := dir.open(id+extension, true, false)
	if e != nil {
		return DeleteResult{}, safeError(e)
	}
	defer f.Close()
	stop := m.watchFile(ctx, f)
	defer stop()
	value, e := readJournal(f, id)
	if e != nil {
		return DeleteResult{}, e
	}
	if value.record.Revision != revision {
		return DeleteResult{}, ErrConflict
	}
	if e = m.active(ctx); e != nil {
		return DeleteResult{}, e
	}
	if e = dir.remove(f); e != nil {
		return DeleteResult{}, safeError(e)
	}
	return DeleteResult{ID: id, Deleted: true}, nil
}
func safeError(e error) error {
	switch {
	case errors.Is(e, os.ErrNotExist):
		return ErrNotFound
	case errors.Is(e, ErrConflict):
		return ErrConflict
	case errors.Is(e, ErrBusy):
		return ErrBusy
	case errors.Is(e, ErrFull):
		return ErrFull
	}
	return ErrStore
}
