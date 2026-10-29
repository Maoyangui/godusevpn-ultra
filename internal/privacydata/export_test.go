package privacydata

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReportExportNeverOverwritesAndRejectsDevices(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "report.json")
	if err := WriteNewLocal(p, []byte(`{"local":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := WriteNewLocal(p, []byte("overwrite")); err == nil {
		t.Fatal("overwrote report")
	}
	b, _ := os.ReadFile(p)
	if string(b) != `{"local":true}` {
		t.Fatal("changed original output")
	}
	for _, name := range []string{"NUL.json", "CON", "NUL .json", "COM¹.txt", "LPT².txt", "bad.", "report.json:secret"} {
		if err := WriteNewLocal(filepath.Join(dir, name), []byte("x")); err == nil {
			t.Fatal("accepted device or ambiguous path", name)
		}
	}
}

func TestReportWriteFailureRemovesOnlyNewOutput(t *testing.T) {
	p := filepath.Join(t.TempDir(), "failed.json")
	createReadOnly := func(path string) (*os.File, func() error, error) {
		if e := os.WriteFile(path, []byte("partial"), 0600); e != nil {
			return nil, nil, e
		}
		f, e := os.Open(path)
		return f, func() error { _ = f.Close(); return os.Remove(path) }, e
	}
	if err := writeNewLocalUsing(p, []byte("report"), createReadOnly); err == nil {
		t.Fatal("ignored write failure")
	}
	if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("kept partial output", err)
	}
}
