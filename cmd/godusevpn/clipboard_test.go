package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/clipboardprivacy"
)

type clipboardFake struct {
	text     string
	sequence uint32
	clears   int
}

func (*clipboardFake) Supported() bool { return true }
func (b *clipboardFake) WriteSecret(s string) (clipboardprivacy.Receipt, error) {
	b.text = s
	b.sequence++
	return clipboardprivacy.Receipt{Sequence: b.sequence, Digest: sha256.Sum256([]byte(s))}, nil
}
func (b *clipboardFake) ClearIfUnchanged(r clipboardprivacy.Receipt) (bool, error) {
	b.clears++
	if r.Sequence != b.sequence || r.Digest != sha256.Sum256([]byte(b.text)) {
		return false, nil
	}
	b.text = ""
	return true, nil
}
func (*clipboardFake) Close() error { return nil }
func TestClipboardBridgeOnlyExplicitIDAndNoSecretReturn(t *testing.T) {
	b := &clipboardFake{}
	m := clipboardprivacy.New(b, clipboardprivacy.DefaultOptions())
	secret := "https://example.invalid/sub?token=SYNTHETIC_ONLY"
	var ids []string
	var saved []clipboardprivacy.Options
	a := &App{ctx: context.Background(), clipboard: &clipboardBridge{manager: m, save: func(o clipboardprivacy.Options) error { saved = append(saved, o); return nil }, profileSecret: func(id string) (string, error) {
		ids = append(ids, id)
		if id != "fixture" {
			return "", errors.New(secret)
		}
		return secret, nil
	}}}
	if b.text != "" || a.ClipboardPrivacyReport().Options.Enabled {
		t.Fatal("report copied or enabled")
	}
	r, e := a.ClipboardPrivacyCopyProfile("fixture")
	if e != nil || r.Pending || b.text != secret || len(ids) != 1 || ids[0] != "fixture" {
		t.Fatal("explicit copy failed")
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "SYNTHETIC") {
		t.Fatal("secret in report")
	}
	if _, e = a.ClipboardPrivacyCopyProfile("missing"); e != errClipboardSecret || strings.Contains(e.Error(), "SYNTHETIC") {
		t.Fatal("source error exposed secret")
	}
	if _, e = a.ClipboardPrivacySave(clipboardprivacy.Options{Enabled: true, DelaySeconds: 60}); e != nil || len(saved) != 1 {
		t.Fatal("separate preference save failed")
	}
	a.ClipboardPrivacyCopyProfile("fixture")
	if !a.ClipboardPrivacyReport().Pending {
		t.Fatal("enabled copy not pending")
	}
	a.ClipboardPrivacyCancel()
	a.shutdownClipboard()
	if b.clears != 0 || b.text != secret {
		t.Fatal("cancelled copy cleared on exit")
	}
	if _, e = a.ClipboardPrivacyCopyProfile("fixture"); e != clipboardprivacy.ErrClosed {
		t.Fatal("postshutdown copy")
	}
}
func TestClipboardPreferenceFailureDoesNotEnableOrClear(t *testing.T) {
	b := &clipboardFake{}
	a := &App{ctx: context.Background(), clipboard: &clipboardBridge{manager: clipboardprivacy.New(b, clipboardprivacy.DefaultOptions()), save: func(clipboardprivacy.Options) error { return errors.New("private path must not escape") }}}
	r, e := a.ClipboardPrivacySave(clipboardprivacy.Options{Enabled: true, DelaySeconds: 30})
	if e != clipboardprivacy.ErrUnavailable || r.Options.Enabled || b.clears != 0 {
		t.Fatal("failed save changed state")
	}
}
