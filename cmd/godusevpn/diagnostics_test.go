package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/ipc"
)

func TestDiagnosticExportBindsReviewedBytesBeforeChoosingDestination(t *testing.T) {
	b := []byte("synthetic in-memory archive fixture")
	h := sha256.Sum256(b)
	a := ipc.DiagnosticArchive{Filename: "godusevpn-ultra-diag-fixture.zip", Data: b, SHA256: hex.EncodeToString(h[:])}
	for _, kind := range []string{"different_preview", "corrupt_bytes", "path_name", "too_large"} {
		t.Run(kind, func(t *testing.T) {
			bad, expected := a, a.SHA256
			switch kind {
			case "different_preview":
				expected = "not-the-reviewed-hash"
			case "corrupt_bytes":
				bad.Data = []byte("changed")
			case "path_name":
				bad.Filename = `..\godusevpn-ultra-diag-fixture.zip`
			case "too_large":
				bad.Data = make([]byte, ipc.DiagnosticMaxZIPBytes+1)
			}
			choose := func(context.Context, string) (string, error) {
				t.Fatal("opened picker before validation")
				return "", nil
			}
			write := func(string, []byte) error { t.Fatal("wrote unreviewed output"); return nil }
			if _, err := saveReviewedDiagnostic(context.Background(), expected, bad, choose, write); err == nil {
				t.Fatal("accepted unreviewed diagnostic")
			}
		})
	}
	count := 0
	choose := func(context.Context, string) (string, error) { return "", nil }
	write := func(string, []byte) error { count++; return nil }
	if saved, err := saveReviewedDiagnostic(context.Background(), a.SHA256, a, choose, write); saved != "" || err != nil || count != 0 {
		t.Fatal("cancel caused a write")
	}
	choose = func(context.Context, string) (string, error) { return "fixture-output.zip", nil }
	write = func(path string, got []byte) error {
		if path != "fixture-output.zip" || string(got) != string(b) {
			t.Fatal("saved different bytes")
		}
		count++
		return errors.New("synthetic disk failure")
	}
	if saved, err := saveReviewedDiagnostic(context.Background(), a.SHA256, a, choose, write); saved != "" || err == nil || count != 1 {
		t.Fatal("write failure misreported")
	}
}
