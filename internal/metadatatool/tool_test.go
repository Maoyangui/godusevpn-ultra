package metadatatool

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
)

type fixtureInput struct {
	data            []byte
	changed, closed bool
}

func (f *fixtureInput) read() ([]byte, error) { return append([]byte(nil), f.data...), nil }
func (f *fixtureInput) unchanged(expected string) error {
	if f.changed || digest(f.data) != expected {
		return ErrChanged
	}
	return nil
}
func (f *fixtureInput) close() { f.closed = true }

type fixtureFiles struct {
	input   *fixtureInput
	output  []byte
	saves   int
	saveErr error
}

func (f *fixtureFiles) openInput(string) (inputLease, error) { return f.input, nil }
func (f *fixtureFiles) saveNew(_, _ string, data []byte) error {
	f.saves++
	if f.saveErr != nil {
		return f.saveErr
	}
	f.output = append([]byte(nil), data...)
	return nil
}

type fixtureRunner struct {
	calls   int
	runFunc func(context.Context, []string, []byte, int) ([]byte, error)
}

func (r *fixtureRunner) run(ctx context.Context, _ backendLease, args []string, input []byte, limit int) ([]byte, error) {
	r.calls++
	return r.runFunc(ctx, args, input, limit)
}
func imageFixture(t *testing.T, format string, pixel color.NRGBA) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			im.SetNRGBA(x, y, pixel)
		}
	}
	var b bytes.Buffer
	var err error
	if format == "JPEG" {
		err = jpeg.Encode(&b, im, &jpeg.Options{Quality: 95})
	} else {
		err = png.Encode(&b, im)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func metadataFixture(format string, tags map[string]json.RawMessage) []byte {
	values := map[string]json.RawMessage{"SourceFile": json.RawMessage(`"-"`), "ExifTool:ExifToolVersion": json.RawMessage(`13.59`), "File:FileType": json.RawMessage(`"` + format + `"`)}
	for k, v := range tags {
		values[k] = v
	}
	result, _ := json.Marshal([]map[string]json.RawMessage{values})
	return result
}
func fakeTool(t *testing.T, data []byte, format string) (*Tool, *fixtureFiles, *fixtureRunner, *int) {
	t.Helper()
	store := &fixtureFiles{input: &fixtureInput{data: data}}
	released := 0
	runner := &fixtureRunner{runFunc: func(ctx context.Context, args []string, input []byte, limit int) ([]byte, error) {
		if args[0] != "-config" || args[1] != "" || args[len(args)-1] != "-" || !bytes.Equal(input, data) {
			t.Fatal("unsafe backend command/input")
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing operation timeout")
		}
		if contains(args, "-j") {
			if limit != maxJSON {
				t.Fatal(limit)
			}
			return metadataFixture(format, map[string]json.RawMessage{"IFD0:Artist": json.RawMessage(`"private artist"`), "GPS:GPSLatitude": json.RawMessage(`1.234`)}), nil
		}
		if limit != maxImage {
			t.Fatal(limit)
		}
		return append([]byte(nil), data...), nil
	}}
	tool := &Tool{runner: runner, files: store, acquire: func() (backendLease, error) { return backendLease{release: func() { released++ }}, nil }}
	return tool, store, runner, &released
}
func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func TestPreviewMinimalAndExplicitValues(t *testing.T) {
	data := imageFixture(t, "PNG", color.NRGBA{R: 43, A: 255})
	tool, store, runner, released := fakeTool(t, data, "PNG")
	preview, err := tool.Preview(context.Background(), `C:\chosen.png`)
	if err != nil {
		t.Fatal(err)
	}
	if preview.SourceSHA256 != digest(data) || preview.Format != "PNG" || len(preview.Tags) != 2 || preview.Tags["GPS:GPSLatitude"].Field != "exif-gps" || preview.Tags["GPS:GPSLatitude"].JSONBytes != 5 {
		t.Fatal(preview)
	}
	encoded, _ := json.Marshal(preview)
	if bytes.Contains(encoded, []byte("1.234")) || bytes.Contains(encoded, []byte("private artist")) {
		t.Fatal("normal preview leaked values")
	}
	if !store.input.closed || *released != 1 {
		t.Fatal("leases not closed")
	}
	values, err := tool.ReadValues(context.Background(), `C:\chosen.png`, preview.SourceSHA256, []string{"exif-gps"})
	if err != nil || len(values) != 1 || string(values["GPS:GPSLatitude"]) != "1.234" {
		t.Fatal(values, err)
	}
	calls := runner.calls
	_, err = tool.ReadValues(context.Background(), `C:\chosen.png`, strings.Repeat("0", 64), []string{"exif-gps"})
	if !errors.Is(err, ErrChanged) || runner.calls != calls {
		t.Fatal("stale reveal reached backend", err)
	}
}
func TestCopyVerificationAndNoOriginalWrite(t *testing.T) {
	for _, format := range []string{"JPEG", "PNG"} {
		t.Run(format, func(t *testing.T) {
			data := imageFixture(t, format, color.NRGBA{G: 71, A: 255})
			tool, store, runner, released := fakeTool(t, data, format)
			reads := 0
			runner.runFunc = func(_ context.Context, args []string, input []byte, limit int) ([]byte, error) {
				if !bytes.Equal(input, data) {
					t.Fatal("original snapshot changed")
				}
				if contains(args, "-j") {
					reads++
					tags := map[string]json.RawMessage{"IFD0:Software": json.RawMessage(`"kept"`)}
					if reads == 1 {
						tags["IFD0:Artist"] = json.RawMessage(`"removed"`)
					}
					return metadataFixture(format, tags), nil
				}
				if !contains(args, "-IFD0:Artist=") || contains(args, "-all=") || contains(args, "-overwrite_original") || args[len(args)-3] != "-o" || args[len(args)-2] != "-" {
					t.Fatal(args)
				}
				return data, nil
			}
			result, err := tool.CreateCopy(context.Background(), CopyRequest{InputPath: `C:\input.jpg`, OutputPath: `C:\new.jpg`, PreviewSHA256: digest(data), Remove: []string{"artist"}})
			if err != nil {
				t.Fatal(err)
			}
			if store.saves != 1 || !bytes.Equal(store.input.data, data) || !bytes.Equal(store.output, data) || result.SourceSHA256 != digest(data) || len(result.Removed) != 1 || runner.calls != 3 || *released != 1 {
				t.Fatal(result, store.saves, runner.calls)
			}
			// The fixture now has no artist even before the request. A verified
			// no-op copy must not report a field as actually removed.
			noOp, err := tool.CreateCopy(context.Background(), CopyRequest{InputPath: `C:\input.jpg`, OutputPath: `C:\new2.jpg`, PreviewSHA256: digest(data), Remove: []string{"artist"}})
			if err != nil || len(noOp.Removed) != 0 || len(noOp.Selected) != 1 {
				t.Fatal("false removal receipt", noOp, err)
			}
		})
	}
}
func TestCopyRefusesUnverifiedResults(t *testing.T) {
	for _, failure := range []string{"selected-remains", "other-changes", "other-disappears", "unknown-tag", "pixel-changes", "trailer", "bad-output", "read-error", "write-error", "changed-source", "stale-preview", "destination-exists", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			data := imageFixture(t, "PNG", color.NRGBA{R: 31, A: 255})
			tool, store, runner, _ := fakeTool(t, data, "PNG")
			reads := 0
			runner.runFunc = func(ctx context.Context, args []string, input []byte, limit int) ([]byte, error) {
				if contains(args, "-j") {
					reads++
					if failure == "read-error" {
						return nil, errors.New("private path must not escape")
					}
					tags := map[string]json.RawMessage{"IFD0:Software": json.RawMessage(`"original"`)}
					if reads == 1 || failure == "selected-remains" {
						tags["IFD0:Artist"] = json.RawMessage(`"remove"`)
					}
					if reads == 2 {
						switch failure {
						case "other-changes":
							tags["IFD0:Software"] = json.RawMessage(`"changed"`)
						case "other-disappears":
							delete(tags, "IFD0:Software")
						case "unknown-tag":
							tags["XMP-dc:Creator"] = json.RawMessage(`"unexpected"`)
						}
					}
					return metadataFixture("PNG", tags), nil
				}
				switch failure {
				case "write-error":
					return nil, errors.New("private stderr")
				case "pixel-changes":
					return imageFixture(t, "PNG", color.NRGBA{B: 99, A: 255}), nil
				case "trailer":
					return append(append([]byte(nil), data...), 42), nil
				case "bad-output":
					return []byte("not image"), nil
				}
				return data, nil
			}
			request := CopyRequest{InputPath: `C:\in.png`, OutputPath: `C:\new.png`, PreviewSHA256: digest(data), Remove: []string{"artist"}}
			if failure == "changed-source" {
				store.input.changed = true
			}
			if failure == "stale-preview" {
				request.PreviewSHA256 = strings.Repeat("0", 64)
			}
			if failure == "destination-exists" {
				store.saveErr = ErrDestination
			}
			ctx := context.Background()
			if failure == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			_, err := tool.CreateCopy(ctx, request)
			if err == nil {
				t.Fatal("unsafe copy succeeded")
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("raw diagnostic escaped")
			}
			if failure != "destination-exists" && store.saves != 0 {
				t.Fatal("output created before verification")
			}
			if !bytes.Equal(store.input.data, data) || !store.input.closed {
				t.Fatal("original/lease changed")
			}
		})
	}
}
func TestStrictMetadataJSON(t *testing.T) {
	valid := string(metadataFixture("JPEG", map[string]json.RawMessage{"GPS:GPSLongitude": json.RawMessage(`123.5`)}))
	if _, err := parseMetadata([]byte(valid), "JPEG"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", `[]`, `[{},{}]`, valid + `{}`, strings.Replace(valid, `"SourceFile":"-"`, `"SourceFile":"-","SourceFile":"-"`, 1), strings.Replace(valid, `"13.59"`, `"13.58"`, 1), strings.Replace(valid, `:13.59`, `:13.58`, 1), strings.Replace(valid, `"JPEG"`, `"PNG"`, 1), strings.Replace(valid, `"SourceFile":"-"`, `"SourceFile":"C:\\secret.jpg"`, 1), strings.Replace(valid, `"GPS:GPSLongitude"`, `"ExifTool:Warning"`, 1), strings.Replace(valid, `"GPS:GPSLongitude"`, `"XMP-dc:Creator"`, 1), strings.Repeat(" ", maxJSON+1)} {
		if invalid == valid {
			continue
		}
		if _, err := parseMetadata([]byte(invalid), "JPEG"); err == nil {
			t.Fatalf("accepted %.150s", invalid)
		}
	}
}
func TestFieldArgumentsAreClosed(t *testing.T) {
	for _, selection := range [][]string{nil, {"artist", "artist"}, {"-all="}, {"gps"}, {"Artist"}, {"artist\x00"}, {"camera", "exif-gps", "software", "description", "copyright", "artist", "capture-time", "extra"}} {
		if _, _, err := selectedFields(selection); err == nil {
			t.Fatal(selection)
		}
	}
	all := Fields()
	all[0].Tags[0] = "-execute"
	if Fields()[0].Tags[0] != "IFD0:Artist" {
		t.Fatal("mutable registry")
	}
	selected, ordered, err := selectedFields([]string{"exif-gps", "artist"})
	if err != nil || strings.Join(ordered, ",") != "artist,exif-gps" {
		t.Fatal(err, ordered)
	}
	args := writeArguments(selected)
	if args[0] != "-config" || args[1] != "" || !contains(args, "-GPS:all=") || !contains(args, "-IFD0:Artist=") || contains(args, "-all=") {
		t.Fatal(args)
	}
	for _, argument := range append(readArguments(), args...) {
		if strings.Contains(argument, `C:\`) {
			t.Fatal("path in backend args")
		}
	}
	camera, _, err := selectedFields([]string{"camera"})
	if err != nil {
		t.Fatal(err)
	}
	cameraArgs := writeArguments(camera)
	// Fixed Exif.pm 0xa431 Name is SerialNumber; BodySerialNumber is only
	// the EXIF-spec name mentioned in Notes, not a writable ExifTool alias.
	if !contains(cameraArgs, "-ExifIFD:SerialNumber=") || contains(cameraArgs, "-ExifIFD:BodySerialNumber=") {
		t.Fatal(cameraArgs)
	}
}
func TestConservativeImageScope(t *testing.T) {
	pngData := imageFixture(t, "PNG", color.NRGBA{A: 255})
	jpegData := imageFixture(t, "JPEG", color.NRGBA{A: 255})
	if _, err := inspectImage(pngData); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectImage(jpegData); err != nil {
		t.Fatal(err)
	}
	var animation bytes.Buffer
	animation.Write(pngData[:33]) // Keep IHDR first, as required by valid APNG.
	chunk := make([]byte, 20)
	binary.BigEndian.PutUint32(chunk, 8)
	copy(chunk[4:], "acTL")
	binary.BigEndian.PutUint32(chunk[8:], 2)
	binary.BigEndian.PutUint32(chunk[16:], crc32.ChecksumIEEE(chunk[4:16]))
	animation.Write(chunk)
	animation.Write(pngData[33:])
	mpf := append([]byte(nil), jpegData[:2]...)
	mpf = append(mpf, []byte{0xff, 0xe2, 0, 6, 'M', 'P', 'F', 0}...)
	mpf = append(mpf, jpegData[2:]...)
	for _, data := range [][]byte{nil, []byte("GIF89a"), append(append([]byte(nil), pngData...), 0), append(append([]byte(nil), jpegData...), 0), pngData[:len(pngData)-1], jpegData[:len(jpegData)-1], animation.Bytes(), mpf} {
		if _, err := inspectImage(data); err == nil {
			t.Fatal("accepted unsupported/malformed image")
		}
	}
	corrupt := append([]byte(nil), pngData...)
	corrupt[len(corrupt)-5] ^= 1
	if singlePNG(corrupt) {
		t.Fatal("CRC bypass")
	}
}
func TestOutputBound(t *testing.T) {
	b := &boundedBuffer{limit: 3}
	if n, err := b.Write([]byte("abcd")); n != 3 || !errors.Is(err, ErrOutputLimit) || b.String() != "abc" || !b.exceeded {
		t.Fatal(n, err, b.String())
	}
}

// This helper is our own Go test executable, never ExifTool or a Perl runtime.
func TestRunnerFixtureProcess(t *testing.T) {
	mode := ""
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "METADATA_TEST_FIXTURE=") {
			mode = strings.TrimPrefix(arg, "METADATA_TEST_FIXTURE=")
		}
	}
	if mode == "" {
		return
	}
	switch mode {
	case "ok":
		_, _ = os.Stdout.Write([]byte("fixture"))
	case "large":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("X"), 10000))
	case "stderr":
		_, _ = os.Stderr.Write([]byte("private metadata"))
	case "timeout":
		time.Sleep(30 * time.Second)
	}
	os.Exit(0)
}
func TestProductionRunnerWithOwnFixture(t *testing.T) {
	if testing.CoverMode() != "" {
		t.Skip("coverage-instrumented child emits runtime diagnostics when the production runner strips GOCOVERDIR; run this fixture in the normal test pass")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	lease := backendLease{executable: executable, directory: t.TempDir()}
	for _, mode := range []string{"ok", "large", "stderr", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if mode == "timeout" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			arguments := []string{"-test.run=^TestRunnerFixtureProcess$", "--", "METADATA_TEST_FIXTURE=" + mode}
			output, err := (nativeRunner{}).run(ctx, lease, arguments, nil, 8)
			if mode == "ok" {
				if err != nil || string(output) != "fixture" {
					t.Fatal(string(output), err)
				}
			} else if err == nil || len(output) != 0 || strings.Contains(err.Error(), "private") {
				t.Fatal("runner did not fail closed", err)
			}
		})
	}
}
