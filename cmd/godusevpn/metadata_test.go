package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/metadatatool"
)

type metadataFixture struct {
	previews, reads, copies int
	path, hash              string
	request                 metadatatool.CopyRequest
}

func (f *metadataFixture) Backend() metadatatool.BackendInfo {
	return metadatatool.BackendInfo{Available: true}
}
func (f *metadataFixture) Preview(_ context.Context, path string) (metadatatool.Preview, error) {
	f.previews++
	f.path = path
	return metadatatool.Preview{SourceSHA256: "fixture-hash", Format: "png", Tags: map[string]metadatatool.TagSummary{"GPS:Latitude": {Field: "exif-gps", JSONBytes: 1}}}, nil
}
func (f *metadataFixture) ReadValues(_ context.Context, path, hash string, _ []string) (map[string]json.RawMessage, error) {
	f.reads++
	f.path = path
	f.hash = hash
	return map[string]json.RawMessage{"GPS:Latitude": json.RawMessage(`1`)}, nil
}
func (f *metadataFixture) CreateCopy(_ context.Context, request metadatatool.CopyRequest) (metadatatool.CopyResult, error) {
	f.copies++
	f.request = request
	return metadatatool.CopyResult{SourceSHA256: request.PreviewSHA256, CopySHA256: "copy-fixture"}, nil
}
func metadataTestController(f *metadataFixture) *metadataController {
	return &metadataController{tool: f, choose: func(context.Context) (string, error) { return `C:\fixture\image.png`, nil }, save: func(context.Context, string) (string, error) { return `C:\fixture\new.png`, nil }}
}

func TestMetadataNativeSelectionAndExplicitReveal(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	selection, err := c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.PreviewID) != 32 || f.reads != 0 || f.copies != 0 || selection.Preview.Tags["GPS:Latitude"].JSONBytes != 1 {
		t.Fatal("preview must not reveal actual values or copy")
	}
	data, _ := json.Marshal(selection)
	var public map[string]any
	_ = json.Unmarshal(data, &public)
	if _, ok := public["inputPath"]; ok {
		t.Fatal("native selected source path exposed as editable request")
	}
	if _, err := c.values(context.Background(), selection.PreviewID, []string{"exif-gps"}); err != nil {
		t.Fatal(err)
	}
	if f.reads != 1 || f.hash != "fixture-hash" || f.path != `C:\fixture\image.png` {
		t.Fatal("explicit reveal lost native selection/hash binding")
	}
	if _, err := c.copy(context.Background(), selection.PreviewID, []string{"artist"}); err != nil {
		t.Fatal(err)
	}
	if f.copies != 1 || f.request.InputPath != `C:\fixture\image.png` || f.request.OutputPath != `C:\fixture\new.png` || f.request.PreviewSHA256 != "fixture-hash" {
		t.Fatal("new copy lost bound source")
	}
}
func TestMetadataExpiredAndUnknownSelection(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	selection, err := c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.values(context.Background(), "other", []string{"artist"}); !errors.Is(err, errMetadataSession) {
		t.Fatal("unknown token accepted")
	}
	if _, err := c.copy(context.Background(), selection.PreviewID, []string{"artist"}); !errors.Is(err, errMetadataSession) {
		t.Fatal("invalidated selection reused")
	}
	selection, err = c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.expires = time.Now().Add(-time.Second)
	if _, err := c.values(context.Background(), selection.PreviewID, []string{"artist"}); !errors.Is(err, errMetadataSession) || f.reads != 0 || f.copies != 0 {
		t.Fatal("expired token reached backend")
	}
}
func TestMetadataCancelAndDialogExpiry(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	selection, err := c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.save = func(context.Context, string) (string, error) {
		c.expires = time.Now().Add(-time.Second)
		return `C:\fixture\new.png`, nil
	}
	if _, err := c.copy(context.Background(), selection.PreviewID, []string{"artist"}); !errors.Is(err, errMetadataSession) || f.copies != 0 {
		t.Fatal("selection expired during save dialog but copy continued")
	}
	c.choose = func(context.Context) (string, error) { return "", nil }
	if result, err := c.preview(context.Background()); err != nil || result != nil || f.previews != 1 {
		t.Fatal("cancel invoked backend")
	}
}
func TestMetadataBusyDoesNotOpenDialogs(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	c.busy = true
	if _, err := c.preview(context.Background()); !errors.Is(err, errMetadataBusy) || f.previews != 0 {
		t.Fatal("busy operation was repeated")
	}
}

func TestMetadataSelectionRequiredBeforeRevealOrSave(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	selection, err := c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	saves := 0
	c.save = func(context.Context, string) (string, error) { saves++; return `C:\fixture\new.png`, nil }
	for _, fields := range [][]string{nil, {}, {"artist", "artist"}, {"unknown"}, {"-all="}, {"exif-gps", "camera", "artist", "copyright", "description", "software", "capture-time", "extra"}} {
		if _, err := c.values(context.Background(), selection.PreviewID, fields); !errors.Is(err, metadatatool.ErrSelection) {
			t.Fatal("invalid reveal selection accepted", fields, err)
		}
		if _, err := c.copy(context.Background(), selection.PreviewID, fields); !errors.Is(err, metadatatool.ErrSelection) {
			t.Fatal("invalid copy selection accepted", fields, err)
		}
	}
	if saves != 0 || f.reads != 0 || f.copies != 0 {
		t.Fatal("invalid selection reached native dialog/backend")
	}
}
func TestMetadataCancelledContextCannotOpenOrResumePicker(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	chooseCalls := 0
	originalChoose := c.choose
	c.choose = func(ctx context.Context) (string, error) { chooseCalls++; return originalChoose(ctx) }
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.preview(cancelled); !errors.Is(err, context.Canceled) || chooseCalls != 0 || f.previews != 0 {
		t.Fatal("cancelled preview opened picker/backend", err)
	}
	selection, err := c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.values(cancelled, selection.PreviewID, []string{"artist"}); !errors.Is(err, context.Canceled) || f.reads != 0 {
		t.Fatal("cancelled reveal reached backend", err)
	}
	if _, err := c.copy(cancelled, selection.PreviewID, []string{"artist"}); !errors.Is(err, context.Canceled) || f.copies != 0 {
		t.Fatal("cancelled copy reached backend", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	c.save = func(context.Context, string) (string, error) { stop(); return `C:\fixture\new.png`, nil }
	if result, err := c.copy(ctx, selection.PreviewID, []string{"artist"}); !errors.Is(err, context.Canceled) || result != nil || f.copies != 0 {
		t.Fatal("copy resumed after cancelled save dialog", err)
	}
	ctx, stop = context.WithCancel(context.Background())
	c.choose = func(context.Context) (string, error) { stop(); return `C:\fixture\image.png`, nil }
	if result, err := c.preview(ctx); !errors.Is(err, context.Canceled) || result != nil || f.previews != 1 {
		t.Fatal("preview resumed after cancelled choose dialog", err)
	}
}
func TestMetadataCancelSaveDoesNotReportCopy(t *testing.T) {
	f := &metadataFixture{}
	c := metadataTestController(f)
	selection, err := c.preview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.save = func(context.Context, string) (string, error) { return "", nil }
	if result, err := c.copy(context.Background(), selection.PreviewID, []string{"artist"}); err != nil || result != nil || f.copies != 0 {
		t.Fatal("cancelled save falsely returned a copy", result, err)
	}
}
