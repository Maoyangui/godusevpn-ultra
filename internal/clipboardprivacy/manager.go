// Package clipboardprivacy tracks only secrets explicitly copied by this app.
// It never monitors or clears arbitrary clipboard contents.
package clipboardprivacy

import (
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrUnsupported  = errors.New("CLIPBOARD_UNSUPPORTED")
	ErrBusy         = errors.New("CLIPBOARD_BUSY")
	ErrUnavailable  = errors.New("CLIPBOARD_UNAVAILABLE")
	ErrUnverifiable = errors.New("CLIPBOARD_UNVERIFIABLE")
	ErrInvalid      = errors.New("CLIPBOARD_INVALID_OPTIONS_OR_TEXT")
	ErrClosed       = errors.New("CLIPBOARD_CLOSED")
)

const MaxSecretBytes = 32 << 10

type Options struct {
	Enabled      bool `json:"enabled"`
	DelaySeconds int  `json:"delaySeconds"`
}

func DefaultOptions() Options { return Options{DelaySeconds: 30} }
func (o Options) Validate() error {
	if o.DelaySeconds < 5 || o.DelaySeconds > 600 {
		return ErrInvalid
	}
	return nil
}

// Receipt stays in process memory and is never part of a report or log.
type Receipt struct {
	Sequence uint32
	Digest   [sha256.Size]byte
}

// ClearIfUnchanged must compare both sequence and content digest and clear
// while holding the same native clipboard lock. Separate read/write calls are
// not a valid implementation: another app could copy between them.
type Backend interface {
	Supported() bool
	WriteSecret(string) (Receipt, error)
	ClearIfUnchanged(Receipt) (bool, error)
	Close() error
}
type Timer interface{ Stop() bool }
type Clock struct {
	Now       func() time.Time
	AfterFunc func(time.Duration, func()) Timer
}
type Report struct {
	Supported         bool    `json:"supported"`
	Options           Options `json:"options"`
	Pending           bool    `json:"pending"`
	DeadlineUnixMilli int64   `json:"deadlineUnixMilli,omitempty"`
	LastCode          string  `json:"lastCode"`
	Closed            bool    `json:"closed"`
}
type Manager struct {
	mu         sync.Mutex
	backend    Backend
	clock      Clock
	options    Options
	receipt    *Receipt
	timer      Timer
	generation uint64
	deadline   time.Time
	last       string
	closed     bool
}

func validSecret(text string) bool {
	return text != "" && len(text) <= MaxSecretBytes && utf8.ValidString(text) && !strings.ContainsRune(text, 0)
}
func New(backend Backend, options Options) *Manager {
	return NewWithClock(backend, options, Clock{Now: time.Now, AfterFunc: func(d time.Duration, f func()) Timer { return time.AfterFunc(d, f) }})
}
func NewWithClock(backend Backend, options Options, clock Clock) *Manager {
	if options.Validate() != nil {
		options = DefaultOptions()
	}
	return &Manager{backend: backend, options: options, clock: clock, last: "idle"}
}
func (m *Manager) reportLocked() Report {
	r := Report{Supported: m.backend != nil && m.backend.Supported(), Options: m.options, Pending: m.receipt != nil, LastCode: m.last, Closed: m.closed}
	if r.Pending {
		r.DeadlineUnixMilli = m.deadline.UnixMilli()
	}
	return r
}
func (m *Manager) Report() Report { m.mu.Lock(); defer m.mu.Unlock(); return m.reportLocked() }
func (m *Manager) stopTimerLocked() {
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
	m.generation++
}
func (m *Manager) forgetLocked() { m.receipt = nil; m.deadline = time.Time{} }
func (m *Manager) Configure(options Options) (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if options.Validate() != nil {
		return m.reportLocked(), ErrInvalid
	}
	if m.closed {
		return m.reportLocked(), ErrClosed
	}
	if m.backend == nil || !m.backend.Supported() {
		return m.reportLocked(), ErrUnsupported
	}
	m.options = options
	if !options.Enabled {
		m.stopTimerLocked()
		m.forgetLocked()
		m.last = "disabled"
	}
	return m.reportLocked(), nil
}
func (m *Manager) CopySecret(text string) (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.reportLocked(), ErrClosed
	}
	if m.backend == nil || !m.backend.Supported() {
		return m.reportLocked(), ErrUnsupported
	}
	if !validSecret(text) {
		return m.reportLocked(), ErrInvalid
	}
	receipt, err := m.backend.WriteSecret(text)
	if err != nil {
		m.last = "copy_failed"
		return m.reportLocked(), ErrUnavailable
	}
	m.stopTimerLocked()
	m.forgetLocked()
	if !m.options.Enabled {
		m.last = "copied_without_timer"
		return m.reportLocked(), nil
	}
	if receipt.Sequence == 0 || receipt.Digest != sha256.Sum256([]byte(text)) {
		m.last = "copied_unverifiable"
		return m.reportLocked(), ErrUnverifiable
	}
	m.receipt = &receipt
	m.deadline = m.clock.Now().Add(time.Duration(m.options.DelaySeconds) * time.Second)
	m.last = "pending"
	generation := m.generation
	m.timer = m.clock.AfterFunc(time.Duration(m.options.DelaySeconds)*time.Second, func() { m.expire(generation, 0) })
	return m.reportLocked(), nil
}
func (m *Manager) expire(generation uint64, attempt int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.receipt == nil || generation != m.generation {
		return
	}
	m.timer = nil
	changed, err := m.backend.ClearIfUnchanged(*m.receipt)
	if err != nil {
		m.last = "clear_failed"
		if errors.Is(err, ErrBusy) && attempt < 2 {
			m.timer = m.clock.AfterFunc(250*time.Millisecond, func() { m.expire(generation, attempt+1) })
		}
		return
	}
	m.forgetLocked()
	if changed {
		m.last = "cleared"
	} else {
		m.last = "changed_preserved"
	}
}
func (m *Manager) Cancel() (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.reportLocked(), ErrClosed
	}
	m.stopTimerLocked()
	m.forgetLocked()
	m.last = "cancelled"
	return m.reportLocked(), nil
}
func (m *Manager) ClearNow() (Report, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.reportLocked(), ErrClosed
	}
	m.stopTimerLocked()
	if m.receipt == nil {
		m.last = "nothing_owned"
		return m.reportLocked(), nil
	}
	changed, err := m.backend.ClearIfUnchanged(*m.receipt)
	if err != nil {
		m.last = "clear_failed"
		return m.reportLocked(), ErrUnavailable
	}
	m.forgetLocked()
	if changed {
		m.last = "cleared"
	} else {
		m.last = "changed_preserved"
	}
	return m.reportLocked(), nil
}
func (m *Manager) Close() Report {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return m.reportLocked()
	}
	m.stopTimerLocked()
	if m.receipt != nil {
		changed, err := m.backend.ClearIfUnchanged(*m.receipt)
		if err != nil {
			m.last = "close_clear_failed"
		} else if changed {
			m.last = "cleared_on_close"
		} else {
			m.last = "changed_preserved"
		}
	}
	m.forgetLocked()
	m.closed = true
	if m.backend != nil {
		_ = m.backend.Close()
	}
	return m.reportLocked()
}
