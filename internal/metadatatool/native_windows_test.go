//go:build windows

package metadatatool

import (
	"bytes"
	"golang.org/x/sys/windows"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only files created inside this test's fresh TEMP directory are touched.
// No native backend, service, policy, network or third-party executable is used.
func TestNativeSourcePinAndExclusiveCopyInTemp(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "selected.png")
	output := filepath.Join(dir, "new-copy.png")
	data := imageFixture(t, "PNG", color.NRGBA{R: 42, A: 255})
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := (nativeFiles{}).openInput(input)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.close()
	got, err := lease.read()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if writer, err := os.OpenFile(input, os.O_WRONLY, 0); err == nil {
		writer.Close()
		t.Fatal("source write not denied")
	}
	if err := os.Rename(input, input+".moved"); err == nil {
		t.Fatal("source replacement not denied")
	}
	if lease.unchanged(digest(data)) != nil {
		t.Fatal("unchanged source failed")
	}
	if err := (nativeFiles{}).saveNew(input, output, data); err != nil {
		t.Fatal("new copy failed", err)
	}
	if got, err := os.ReadFile(output); err != nil || !bytes.Equal(got, data) {
		t.Fatal(err)
	}
	if err := (nativeFiles{}).saveNew(input, output, []byte("must not overwrite")); err == nil {
		t.Fatal("existing copy overwritten")
	}
	if got, err := os.ReadFile(output); err != nil || !bytes.Equal(got, data) {
		t.Fatal("existing bytes changed", err)
	}
	if err := (nativeFiles{}).saveNew(input, input, data); err == nil {
		t.Fatal("original overwritten")
	}
	if err := (nativeFiles{}).saveNew(input, filepath.Join(dir, "mismatch.jpg"), data); err == nil {
		t.Fatal("format/extension mismatch allowed")
	}
}

func TestSourcePathSyntaxWithoutIO(t *testing.T) {
	for _, path := range []string{`C:\selected.jpg`, `D:\a b\测试.PNG`, `C:\pictures\new.jpeg`} {
		if imagePath(path) != nil {
			t.Fatal(path)
		}
	}
	for _, path := range []string{`relative.jpg`, `\\server\share\in.jpg`, `\\?\C:\in.jpg`, `C:\in.jpg:secret`, `C:\a\..\in.jpg`, `C:\CON.jpg`, `C:\nul\in.jpg`, `C:\COM¹.png`, `C:\LPT9.png`, `C:\trailing.\in.png`, `C:\in.gif`, `C:\folder\in.png.`, `C:\wild*\in.jpg`} {
		if imagePath(path) == nil {
			t.Fatal(path)
		}
	}
}
func TestBundleManifestCompleteAndClosed(t *testing.T) {
	files, dirs, err := parseBundleManifest(bundleManifest)
	if err != nil || len(files) != 1501 || !dirs["exiftool_files/lib/image/exiftool"] || files["exiftool_files/exiftool.pl"] != "9e8e817fab9fdc2e37f7cd023aa34e76d1ef561834dfd791a48a7e327cc8f4b1" {
		t.Fatal(err, len(files))
	}
	for _, bad := range []string{bundleManifest + strings.Split(bundleManifest, "\n")[0] + "\n", strings.Replace(bundleManifest, "  ExifTool.exe", "  ../ExifTool.exe", 1), strings.Replace(bundleManifest, "  ExifTool.exe", "  C:/ExifTool.exe", 1), strings.Replace(bundleManifest, "  ExifTool.exe", "  /ExifTool.exe", 1), "xyz" + bundleManifest[3:], strings.Join(strings.Split(bundleManifest, "\n")[1:], "\n")} {
		if _, _, err := parseBundleManifest(bad); err == nil {
			t.Fatal("accepted malformed manifest")
		}
	}
}
func TestCodeACLUsingSyntheticDescriptors(t *testing.T) {
	for _, test := range []struct {
		sd             string
		ancestor, want bool
	}{
		{"O:BAD:P(A;;FA;;;BA)(A;;FR;;;BU)", false, true},
		{"O:SYD:P(A;;FA;;;SY)(A;;FR;;;BU)", false, true},
		{"O:BUD:P(A;;FA;;;BA)", false, false},
		{"O:BAD:P(A;;FA;;;BA)(A;;FW;;;BU)", false, false},
		{"O:BAD:P(A;;FA;;;BA)(A;;WD;;;BU)", false, false},
		{"O:BAD:P(A;;FA;;;BA)(A;;DC;;;BU)", false, false},
		{"O:BAD:P(A;;FA;;;BA)(A;;GW;;;BU)", false, false},
		{"O:BAD:P(A;;FA;;;BA)(A;;0x2;;;BU)", true, true},
		{"O:BAD:P(A;;FA;;;BA)(A;;0x2;;;BU)", false, false},
	} {
		sd, err := windows.SecurityDescriptorFromString(test.sd)
		if err != nil {
			t.Fatal(err, test.sd)
		}
		if got := codeDescriptor(sd, test.ancestor) == nil; got != test.want {
			t.Fatal(test.sd, test.ancestor, got)
		}
	}
	if codeDescriptor(nil, false) == nil {
		t.Fatal("null descriptor accepted")
	}
}
func TestBackendEnvironmentExcludesInjection(t *testing.T) {
	values := backendEnvironment()
	if len(values) != 2 {
		t.Fatal(values)
	}
	for _, value := range values {
		if !strings.HasPrefix(value, "SystemRoot=") && !strings.HasPrefix(value, "WINDIR=") {
			t.Fatal(value)
		}
	}
}
