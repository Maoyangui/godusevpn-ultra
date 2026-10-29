//go:build windows

package privacydata

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// This fixture creates one mount-point junction whose source and target are
// siblings under a new TEMP root. It never names a UNC path or changes policy.
func makeExportJunction(t *testing.T, root, source, target string) {
	t.Helper()
	for _, p := range []string{source, target} {
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			t.Fatal("junction fixture escaped its TEMP root")
		}
	}
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(source)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal("could not open isolated junction fixture", err)
	}
	defer windows.CloseHandle(h)
	sub, err := windows.UTF16FromString(`\??\` + target)
	if err != nil {
		t.Fatal(err)
	}
	print, err := windows.UTF16FromString(target)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 16+2*(len(sub)+len(print)))
	binary.LittleEndian.PutUint32(data[0:4], 0xa0000003)
	binary.LittleEndian.PutUint16(data[4:6], uint16(len(data)-8))
	binary.LittleEndian.PutUint16(data[10:12], uint16(2*(len(sub)-1)))
	binary.LittleEndian.PutUint16(data[12:14], uint16(2*len(sub)))
	binary.LittleEndian.PutUint16(data[14:16], uint16(2*(len(print)-1)))
	for i, u := range append(sub, print...) {
		binary.LittleEndian.PutUint16(data[16+2*i:], u)
	}
	var returned uint32
	if err := windows.DeviceIoControl(h, 0x000900a4, &data[0], uint32(len(data)), nil, 0, &returned, nil); err != nil {
		t.Fatal("could not create isolated junction fixture", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(source); err != nil {
			t.Error("could not remove isolated junction leaf", err)
		}
	})
}

func TestReportExportRejectsExistingParentJunctionWithoutCreatingTargetOutput(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "existing.txt")
	if err := os.WriteFile(sentinel, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "junction")
	makeExportJunction(t, root, link, target)
	if err := WriteNewLocal(filepath.Join(link, "report.json"), []byte("private-report")); err == nil {
		t.Fatal("export accepted an existing parent junction")
	}
	if _, err := os.Stat(filepath.Join(target, "report.json")); !os.IsNotExist(err) {
		t.Fatal("export created output through the rejected junction")
	}
	if b, err := os.ReadFile(sentinel); err != nil || string(b) != "original" {
		t.Fatal("fixture target content changed")
	}
	// The refusal must not leave a lease that prevents an unrelated local save.
	if err := WriteNewLocal(filepath.Join(root, "ordinary.json"), []byte("ordinary")); err != nil {
		t.Fatal("refused parent leaked a lease", err)
	}
}
