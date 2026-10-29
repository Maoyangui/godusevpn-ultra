package privacydata

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func testImage() image.Image {
	im := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			im.Set(x, y, color.RGBA{uint8(x * 60), uint8(y * 80), uint8((x + y) * 30), 255})
		}
	}
	return im
}
func baseJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, testImage(), &jpeg.Options{Quality: 91}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func basePNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, testImage()); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func jpgSegment(marker byte, p []byte) []byte {
	b := []byte{0xff, marker, 0, 0}
	binary.BigEndian.PutUint16(b[2:], uint16(len(p)+2))
	return append(b, p...)
}
func pngChunk(kind string, p []byte) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(p)))
	b.WriteString(kind)
	b.Write(p)
	sum := crc32.ChecksumIEEE(append([]byte(kind), p...))
	_ = binary.Write(&b, binary.BigEndian, sum)
	return b.Bytes()
}
func compressed(p []byte) []byte {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, _ = w.Write(p)
	_ = w.Close()
	return b.Bytes()
}
func saveInput(t *testing.T, b []byte) (string, string) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "selected.image")
	dst := filepath.Join(dir, "cleaned.image")
	if err := os.WriteFile(src, b, 0600); err != nil {
		t.Fatal(err)
	}
	return src, dst
}
func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func comparePixels(t *testing.T, a, b []byte, format string) {
	t.Helper()
	var before, after image.Image
	var err error
	if format == "jpeg" {
		before, err = jpeg.Decode(bytes.NewReader(a))
		if err == nil {
			after, err = jpeg.Decode(bytes.NewReader(b))
		}
	} else {
		before, err = png.Decode(bytes.NewReader(a))
		if err == nil {
			after, err = png.Decode(bytes.NewReader(b))
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	if before.Bounds() != after.Bounds() {
		t.Fatal("bounds changed")
	}
	for y := before.Bounds().Min.Y; y < before.Bounds().Max.Y; y++ {
		for x := before.Bounds().Min.X; x < before.Bounds().Max.X; x++ {
			ar, ag, ab, aa := before.At(x, y).RGBA()
			br, bg, bb, ba := after.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				t.Fatal("pixels changed")
			}
		}
	}
}

func TestJPEGRemovesOnlyTargetBlocksAndKeepsICCEncodedPixels(t *testing.T) {
	base := baseJPEG(t)
	icc := jpgSegment(0xe2, append([]byte("ICC_PROFILE\x00\x01\x01"), []byte("SYNTHETIC_COLOUR_PROFILE")...))
	secret := []byte("GPS_DO_NOT_RETURN_31.12345_PRIVATE")
	input := append([]byte{}, base[:2]...)
	input = append(input, jpgSegment(0xe1, append([]byte("Exif\x00\x00"), secret...))...)
	input = append(input, icc...)
	input = append(input, jpgSegment(0xed, secret)...)
	input = append(input, jpgSegment(0xfe, secret)...)
	input = append(input, base[2:]...)
	src, dst := saveInput(t, input)
	r, err := Inspect(src)
	if err != nil || r.Format != "jpeg" || len(r.MetadataBlocks) != 3 || !r.HasMetadata {
		t.Fatal(r, err)
	}
	j, _ := json.Marshal(r)
	if bytes.Contains(j, secret) {
		t.Fatal("metadata contents leaked into report")
	}
	r, err = Clean(src, dst)
	if err != nil || len(r.RemovedBlocks) != 3 || r.Output != dst {
		t.Fatal(r, err)
	}
	expected := append([]byte{}, base[:2]...)
	expected = append(expected, icc...)
	expected = append(expected, base[2:]...)
	output := readFile(t, dst)
	if !bytes.Equal(expected, output) {
		t.Fatal("non-target JPEG bytes changed")
	}
	if !bytes.Equal(readFile(t, src), input) {
		t.Fatal("source changed")
	}
	comparePixels(t, input, output, "jpeg")
	r, err = Inspect(dst)
	if err != nil || r.HasMetadata || len(r.MetadataBlocks) != 0 {
		t.Fatal(r, err)
	}
}

func TestPNGRemovesMetadataKeepsColourUnknownAncillaryAndIDAT(t *testing.T) {
	base := basePNG(t)
	secret := []byte("PRIVATE_GPS_PAYLOAD_NEVER_REPORT")
	icc := pngChunk("iCCP", append([]byte("Synthetic\x00\x00"), compressed([]byte("ICC test profile"))...))
	other := pngChunk("vpAg", []byte("preserved unknown ancillary"))
	input := append([]byte{}, base[:33]...)
	input = append(input, icc...)
	input = append(input, pngChunk("eXIf", secret)...)
	input = append(input, pngChunk("tEXt", append([]byte("Comment\x00"), secret...))...)
	input = append(input, pngChunk("zTXt", append([]byte("Comment\x00\x00"), compressed(secret)...))...)
	input = append(input, pngChunk("iTXt", append([]byte("Comment\x00\x00\x00\x00\x00"), secret...))...)
	input = append(input, other...)
	input = append(input, base[33:]...)
	src, dst := saveInput(t, input)
	r, err := Inspect(src)
	if err != nil || r.Format != "png" || len(r.MetadataBlocks) != 4 {
		t.Fatal(r, err)
	}
	j, _ := json.Marshal(r)
	if bytes.Contains(j, secret) {
		t.Fatal("PNG contents leaked")
	}
	r, err = Clean(src, dst)
	if err != nil || len(r.RemovedBlocks) != 4 {
		t.Fatal(r, err)
	}
	expected := append([]byte{}, base[:33]...)
	expected = append(expected, icc...)
	expected = append(expected, other...)
	expected = append(expected, base[33:]...)
	output := readFile(t, dst)
	if !bytes.Equal(expected, output) {
		t.Fatal("colour profile, unknown chunk or IDAT changed")
	}
	comparePixels(t, input, output, "png")
	if !bytes.Equal(readFile(t, src), input) {
		t.Fatal("source changed")
	}
	r, err = Inspect(dst)
	if err != nil || r.HasMetadata {
		t.Fatal(r, err)
	}
}

func TestNoMetadataStillCreatesIndependentUnchangedCopy(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run(format, func(t *testing.T) {
			b := basePNG(t)
			if format == "jpeg" {
				b = baseJPEG(t)
			}
			src, dst := saveInput(t, b)
			r, err := Clean(src, dst)
			if err != nil || len(r.RemovedBlocks) != 0 || r.HasMetadata || !bytes.Equal(b, readFile(t, dst)) {
				t.Fatal(r, err)
			}
		})
	}
}

func TestNeverOverwriteSourceOrExistingDestination(t *testing.T) {
	src, dst := saveInput(t, basePNG(t))
	if _, err := Clean(src, src); err == nil {
		t.Fatal("same file accepted")
	}
	existing := []byte("DO NOT OVERWRITE")
	if err := os.WriteFile(dst, existing, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(src, dst); err == nil {
		t.Fatal("existing output overwritten")
	}
	if !bytes.Equal(readFile(t, dst), existing) {
		t.Fatal("existing output changed")
	}
}

func TestUnsupportedAndDamagedInputsNeverCreateOutput(t *testing.T) {
	jpegBytes := baseJPEG(t)
	pngBytes := basePNG(t)
	crcBad := append([]byte{}, pngBytes...)
	crcBad[29] ^= 1
	badLength := append([]byte{}, pngBytes...)
	binary.BigEndian.PutUint32(badLength[8:12], 0xffffffff)
	for name, b := range map[string][]byte{"unsupported": []byte("GIF89a"), "empty": {}, "jpeg-truncated": jpegBytes[:len(jpegBytes)-2], "jpeg-trailing": append(append([]byte{}, jpegBytes...), []byte("secret appended")...), "png-crc": crcBad, "png-length": badLength, "png-truncated": pngBytes[:len(pngBytes)-1], "png-trailing": append(append([]byte{}, pngBytes...), 0)} {
		t.Run(name, func(t *testing.T) {
			src, dst := saveInput(t, b)
			if _, err := Inspect(src); err == nil {
				t.Fatal("damaged input accepted")
			}
			if _, err := Clean(src, dst); err == nil {
				t.Fatal("damaged input cleaned")
			}
			if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed operation kept output", err)
			}
			if !bytes.Equal(readFile(t, src), b) {
				t.Fatal("source changed")
			}
		})
	}
}

func TestRejectInvalidEncodedPixelsEvenWithValidPNGCRC(t *testing.T) {
	base := basePNG(t)
	var out bytes.Buffer
	out.Write(base[:8])
	for off := 8; off < len(base); {
		n := int(binary.BigEndian.Uint32(base[off : off+4]))
		kind := string(base[off+4 : off+8])
		if kind == "IDAT" {
			out.Write(pngChunk(kind, []byte("not valid zlib data")))
		} else {
			out.Write(base[off : off+n+12])
		}
		off += n + 12
	}
	src, _ := saveInput(t, out.Bytes())
	if _, err := Inspect(src); err == nil {
		t.Fatal("invalid pixel data accepted")
	}
}

func TestRejectPathsAndDirectories(t *testing.T) {
	for _, p := range []string{`\\server\share\photo.jpg`, `\\?\C:\photo.jpg`, `\\.\PhysicalDrive0`, `C:\photo.jpg:secret`, `relative.jpg`, `/tmp/NUL.png`, `/tmp/photo.`, `/tmp/../photo.jpg`} {
		if _, err := Inspect(p); err == nil {
			t.Fatal("unsafe path accepted", p)
		}
	}
	if _, err := Inspect(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
	src, _ := saveInput(t, basePNG(t))
	if _, err := Clean(src, t.TempDir()); err == nil {
		t.Fatal("directory output accepted")
	}
}

func TestRejectSymlinkInputAndParent(t *testing.T) {
	dir := t.TempDir()
	src, dst := saveInput(t, basePNG(t))
	link := filepath.Join(dir, "linked.png")
	if err := os.Symlink(src, link); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	if _, err := Inspect(link); err == nil {
		t.Fatal("symlink input accepted")
	}
	parent := filepath.Join(dir, "linked-dir")
	if err := os.Symlink(filepath.Dir(src), parent); err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(src, filepath.Join(parent, filepath.Base(dst))); err == nil {
		t.Fatal("symlink parent accepted")
	}
}

type failWriter struct{ remaining int }

func (w *failWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, errors.New("simulated disk failure")
	}
	w.remaining -= len(p)
	return len(p), nil
}
func TestParsersPropagateOutputFailure(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		b := basePNG(t)
		if format == "jpeg" {
			b = baseJPEG(t)
		}
		if _, err := process(bytes.NewReader(b), &failWriter{12}, format, int64(len(b))); err == nil {
			t.Fatal("write failure ignored", format)
		}
	}
}

func TestDestinationDiscardDeletesOnlyOwnNewFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "new.png")
	f, discard, err := openDestination(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(f, "partial"); err != nil {
		t.Fatal(err)
	}
	if err = discard(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial output retained", err)
	}
}

func TestPNGTooManyMetadataBlocksBounded(t *testing.T) {
	base := basePNG(t)
	b := append([]byte{}, base[:33]...)
	for i := 0; i < 4097; i++ {
		b = append(b, pngChunk("tEXt", nil)...)
	}
	b = append(b, base[33:]...)
	src, _ := saveInput(t, b)
	if _, err := Inspect(src); err == nil {
		t.Fatal("unbounded metadata accepted")
	}
}

func TestCleanWriteFailureRemovesItsOwnPartialOutput(t *testing.T) {
	src, dst := saveInput(t, basePNG(t))
	createReadOnlyOutput := func(path string) (*os.File, func() error, error) {
		created, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, nil, err
		}
		_, err = created.Write([]byte("partial"))
		_ = created.Close()
		if err != nil {
			return nil, nil, err
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		return f, func() error { _ = f.Close(); return os.Remove(path) }, nil
	}
	if _, err := cleanUsing(src, dst, createReadOnlyOutput); err == nil {
		t.Fatal("read-only output did not fail")
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed clean kept partial output", err)
	}
	if !bytes.Equal(readFile(t, src), basePNG(t)) {
		t.Fatal("failed clean changed source")
	}
}

func TestJPEGScannerPreservesMultipleScanStuffingAndRestartBytes(t *testing.T) {
	// Structural scanner fixture: the public API additionally requires a valid
	// image decoder result, tested separately with generated JPEGs above.
	header := jpgSegment(0xda, []byte{1, 1, 0, 0, 63, 0})
	scan1 := []byte{1, 2, 0xff, 0, 3, 0xff, 0xd0, 4}
	scan2 := []byte{5, 0xff, 0xff, 0xd1, 6}
	var input bytes.Buffer
	input.Write([]byte{0xff, 0xd8})
	input.Write(header)
	input.Write(scan1)
	input.Write(jpgSegment(0xe1, []byte("private between scans")))
	input.Write(header)
	input.Write(scan2)
	input.Write([]byte{0xff, 0xd9})
	var expected bytes.Buffer
	expected.Write([]byte{0xff, 0xd8})
	expected.Write(header)
	expected.Write(scan1)
	expected.Write(header)
	expected.Write(scan2)
	expected.Write([]byte{0xff, 0xd9})
	var output bytes.Buffer
	blocks, err := processJPEG(bytes.NewReader(input.Bytes()), &output, int64(input.Len()))
	if err != nil || len(blocks) != 1 || !bytes.Equal(output.Bytes(), expected.Bytes()) {
		t.Fatal(blocks, err, "scanner altered encoded scan data")
	}
}
