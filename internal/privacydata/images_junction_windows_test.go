//go:build windows

package privacydata

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestImageToolsRefuseJunctionSourcesAndDestinations(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	image := basePNG(t)
	source := filepath.Join(root, "source.png")
	if err := os.WriteFile(source, image, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "image.png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "junction")
	makeExportJunction(t, root, link, target)
	redirectedSource, redirectedDestination := filepath.Join(link, "image.png"), filepath.Join(link, "output.png")
	if _, err := Inspect(redirectedSource); err == nil {
		t.Fatal("inspect followed a parent junction")
	}
	if _, err := Clean(redirectedSource, filepath.Join(root, "from-redirected.png")); err == nil {
		t.Fatal("clean accepted a redirected source")
	}
	if _, err := Clean(source, redirectedDestination); err == nil {
		t.Fatal("clean accepted a redirected destination")
	}
	if f, discard, err := openDestination(redirectedDestination); err == nil {
		_ = discard()
		_ = f.Close()
		t.Fatal("direct destination opener followed a parent junction")
	}
	for _, p := range []string{filepath.Join(target, "output.png"), filepath.Join(root, "from-redirected.png")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("rejected image path created an output")
		}
	}
	if !bytes.Equal(readFile(t, source), image) || !bytes.Equal(readFile(t, filepath.Join(target, "image.png")), image) {
		t.Fatal("rejected image operation changed source bytes")
	}
	if _, err := Clean(source, filepath.Join(root, "ordinary.png")); err != nil {
		t.Fatal("junction refusal left ordinary image tools unusable", err)
	}
}

func assertFileToolParentPinned(t *testing.T, parent string) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(parent)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err == nil {
		windows.CloseHandle(h)
		t.Fatal("file tool released the parent before creating its output")
	}
	if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("parent write was not refused by its sharing lease", err)
	}
}

func TestFileToolsKeepParentLeaseAcrossOutputCreation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.png")
	if err := os.WriteFile(source, basePNG(t), 0600); err != nil {
		t.Fatal(err)
	}
	createImage := func(path string) (*os.File, func() error, error) {
		assertFileToolParentPinned(t, root)
		return openDestination(path)
	}
	if _, err := cleanUsing(source, filepath.Join(root, "image.png"), createImage); err != nil {
		t.Fatal("pinned image output failed", err)
	}
	createReport := func(path string) (*os.File, func() error, error) {
		assertFileToolParentPinned(t, root)
		return openPrivateDestination(path)
	}
	if err := writeNewLocalUsing(filepath.Join(root, "report.json"), []byte("synthetic"), createReport); err != nil {
		t.Fatal("pinned report output failed", err)
	}
}
