package clipboardprivacy

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	text                   string
	sequence               uint32
	writes, clears, closes int
	writeErr, clearErr     error
	zeroSequence           bool
	unsupported            bool
}

func (b *fakeBackend) Supported() bool { return !b.unsupported }
func (b *fakeBackend) WriteSecret(text string) (Receipt, error) {
	b.writes++
	if b.writeErr != nil {
		return Receipt{}, b.writeErr
	}
	b.sequence++
	b.text = text
	sequence := b.sequence
	if b.zeroSequence {
		sequence = 0
	}
	return Receipt{sequence, sha256.Sum256([]byte(text))}, nil
}
func (b *fakeBackend) ClearIfUnchanged(r Receipt) (bool, error) {
	b.clears++
	if b.clearErr != nil {
		return false, b.clearErr
	}
	if b.sequence != r.Sequence || sha256.Sum256([]byte(b.text)) != r.Digest {
		return false, nil
	}
	b.text = ""
	b.sequence++
	return true, nil
}
func (b *fakeBackend) Close() error { b.closes++; return nil }

type fakeTimer struct {
	callback func()
	stopped  bool
	delay    time.Duration
}

func (t *fakeTimer) Stop() bool { was := !t.stopped; t.stopped = true; return was }

type fakeClock struct{ timers []*fakeTimer }

func (c *fakeClock) clock() Clock {
	return Clock{Now: func() time.Time { return time.Unix(100, 0) }, AfterFunc: func(d time.Duration, f func()) Timer {
		t := &fakeTimer{callback: f, delay: d}
		c.timers = append(c.timers, t)
		return t
	}}
}
func fixtureManager(enabled bool) (*Manager, *fakeBackend, *fakeClock) {
	b := &fakeBackend{}
	c := &fakeClock{}
	return NewWithClock(b, Options{enabled, 30}, c.clock()), b, c
}

func TestDefaultDisabledAndExplicitCopyOnly(t *testing.T) {
	b := &fakeBackend{}
	c := &fakeClock{}
	m := NewWithClock(b, DefaultOptions(), c.clock())
	if m.Report().Options.Enabled || b.writes != 0 || b.clears != 0 {
		t.Fatal("construction/report touched clipboard")
	}
	r, e := m.CopySecret("synthetic-secret")
	if e != nil || r.Pending || r.LastCode != "copied_without_timer" || b.writes != 1 || len(c.timers) != 0 {
		t.Fatal("default must copy without tracking")
	}
	m.Close()
	if b.clears != 0 {
		t.Fatal("disabled copy was later cleared")
	}
}
func TestExpirePreservesEveryLaterCopy(t *testing.T) {
	for _, change := range []string{"identical-content-new-sequence", "different-content-same-sequence", "different-both"} {
		t.Run(change, func(t *testing.T) {
			m, b, c := fixtureManager(true)
			m.CopySecret("synthetic-secret")
			switch change {
			case "identical-content-new-sequence":
				b.sequence++
			case "different-content-same-sequence":
				b.text = "user data"
			case "different-both":
				b.sequence++
				b.text = "user data"
			}
			want := b.text
			c.timers[0].callback()
			r := m.Report()
			if b.text != want || r.Pending || r.LastCode != "changed_preserved" {
				t.Fatal("later clipboard must be preserved")
			}
		})
	}
	m, b, c := fixtureManager(true)
	m.CopySecret("synthetic-secret")
	c.timers[0].callback()
	if b.text != "" || m.Report().LastCode != "cleared" {
		t.Fatal("unchanged receipt must clear")
	}
}
func TestStaleTimerCancelDisableAndDelay(t *testing.T) {
	m, b, c := fixtureManager(true)
	m.CopySecret("first")
	old := c.timers[0]
	m.CopySecret("second")
	old.callback()
	if b.text != "second" || b.clears != 0 {
		t.Fatal("stale callback cleared new copy")
	}
	m.Configure(Options{true, 60})
	if c.timers[1].delay != 30*time.Second {
		t.Fatal("changing delay changed active deadline")
	}
	m.Cancel()
	c.timers[1].callback()
	if b.text != "second" || b.clears != 0 {
		t.Fatal("cancel must leave clipboard")
	}
	m.CopySecret("third")
	if c.timers[2].delay != 60*time.Second {
		t.Fatal("next copy did not use delay")
	}
	m.Configure(Options{false, 60})
	c.timers[2].callback()
	m.Close()
	if b.text != "third" || b.clears != 0 {
		t.Fatal("disable must stop tracking without mutation")
	}
}
func TestBoundedBusyRetriesAndManualRecovery(t *testing.T) {
	m, b, c := fixtureManager(true)
	m.CopySecret("synthetic-secret")
	b.clearErr = ErrBusy
	for i := 0; i < 3; i++ {
		c.timers[i].callback()
	}
	if b.clears != 3 || len(c.timers) != 3 || !m.Report().Pending || m.Report().LastCode != "clear_failed" {
		t.Fatal("busy retries not bounded/pending lost")
	}
	b.clearErr = nil
	r, e := m.ClearNow()
	if e != nil || r.Pending || b.text != "" {
		t.Fatal("manual retry failed")
	}
}
func TestCopyFailureKeepsPreviousRecoveryAndRejectsUnverifiable(t *testing.T) {
	m, b, c := fixtureManager(true)
	m.CopySecret("first")
	b.writeErr = errors.New("synthetic secret must not escape")
	r, e := m.CopySecret("second")
	if e != ErrUnavailable || !r.Pending || len(c.timers) != 1 {
		t.Fatal("copy failure lost previous timer")
	}
	b.writeErr = nil
	c.timers[0].callback()
	if b.text != "" {
		t.Fatal("old recovery failed")
	}
	b.zeroSequence = true
	r, e = m.CopySecret("third")
	if e != ErrUnverifiable || r.Pending || len(c.timers) != 1 {
		t.Fatal("unverified receipt armed cleanup")
	}
}
func TestCloseConditionalAndErrors(t *testing.T) {
	for _, kind := range []string{"owned", "external", "busy"} {
		t.Run(kind, func(t *testing.T) {
			m, b, c := fixtureManager(true)
			m.CopySecret("synthetic-secret")
			if kind == "external" {
				b.sequence++
				b.text = "user data"
			}
			if kind == "busy" {
				b.clearErr = ErrBusy
			}
			r := m.Close()
			c.timers[0].callback()
			m.Close()
			if !r.Closed || r.Pending || b.closes != 1 || b.clears != 1 {
				t.Fatal("close not bounded/idempotent")
			}
			if kind == "owned" && b.text != "" || kind == "external" && b.text != "user data" || kind == "busy" && r.LastCode != "close_clear_failed" {
				t.Fatal("unsafe close")
			}
			if _, e := m.CopySecret("after"); e != ErrClosed {
				t.Fatal("copy after close")
			}
		})
	}
}
func TestInvalidInputsAndReportsContainNoSecret(t *testing.T) {
	m, b, _ := fixtureManager(true)
	for _, text := range []string{"", strings.Repeat("x", MaxSecretBytes+1), "x\x00y", string([]byte{0xff})} {
		if _, e := m.CopySecret(text); e != ErrInvalid {
			t.Fatal("invalid text accepted")
		}
	}
	for _, delay := range []int{0, 4, 601} {
		if _, e := m.Configure(Options{true, delay}); e != ErrInvalid {
			t.Fatal("invalid delay")
		}
	}
	if b.writes != 0 {
		t.Fatal("invalid input reached backend")
	}
	secret := "https://example.invalid/sub?token=SYNTHETIC_CLIPBOARD_SECRET"
	m.CopySecret(secret)
	data, _ := json.Marshal(m.Report())
	if strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "sequence") || strings.Contains(string(data), "digest") {
		t.Fatal("report exposed secret/receipt")
	}
	b.unsupported = true
	if _, e := m.CopySecret("x"); e != ErrUnsupported {
		t.Fatal("unsupported backend accepted")
	}
}
func TestOptionsFileOnlyStoresPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "options.json")
	options, e := LoadOptions(path)
	if e != nil || options != DefaultOptions() {
		t.Fatal("missing preferences default")
	}
	if e = SaveOptions(path, Options{true, 45}); e != nil {
		t.Fatal(e)
	}
	options, e = LoadOptions(path)
	if e != nil || options != (Options{true, 45}) {
		t.Fatal("roundtrip failed")
	}
	if e = SaveOptions(path, Options{false, 55}); e != nil {
		t.Fatal("replace preferences failed")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "digest") {
		t.Fatal("preferences contain receipt")
	}
	for _, bad := range []string{`{}`, `{"schemaVersion":2,"options":{"enabled":true,"delaySeconds":30}}`, `{"schemaVersion":1,"options":{"enabled":true,"delaySeconds":30,"secret":"x"}}`, `{"schemaVersion":1,"options":{"enabled":true,"delaySeconds":30}} {}`, strings.Repeat("x", 4097)} {
		os.WriteFile(path, []byte(bad), 0600)
		got, e := LoadOptions(path)
		if e != ErrInvalid || got != DefaultOptions() {
			t.Fatal("invalid preferences did not default disabled")
		}
	}
}
