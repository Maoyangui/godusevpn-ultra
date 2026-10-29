package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserprofiles"
)

type fakeProfileStore struct {
	calls               []string
	saved, id, revision string
	closed              int
	err                 error
}

func (s *fakeProfileStore) List(ctx context.Context) (browserprofiles.ListResult, error) {
	s.calls = append(s.calls, "list")
	if _, ok := ctx.Deadline(); !ok {
		panic("missing API deadline")
	}
	return browserprofiles.ListResult{Records: []browserprofiles.Summary{{ID: "test", Name: "中文配置"}}}, s.err
}
func (s *fakeProfileStore) Load(ctx context.Context, id string) (browserprofiles.Record, error) {
	s.calls = append(s.calls, "load")
	s.id = id
	return browserprofiles.Record{ID: id, Name: "中文配置", RequiresLaunchValidation: true}, s.err
}
func (s *fakeProfileStore) Save(ctx context.Context, data []byte) (browserprofiles.Record, error) {
	s.calls = append(s.calls, "save")
	s.saved = string(data)
	return browserprofiles.Record{ID: "new-backend-id", RequiresLaunchValidation: true}, s.err
}
func (s *fakeProfileStore) Delete(ctx context.Context, id, revision string) (browserprofiles.DeleteResult, error) {
	s.calls = append(s.calls, "delete")
	s.id = id
	s.revision = revision
	return browserprofiles.DeleteResult{ID: id, Deleted: s.err == nil}, s.err
}
func (s *fakeProfileStore) Close() { s.closed++ }
func TestBrowserProfilesBridgeUsesOnlyInjectedDraftBackend(t *testing.T) {
	backend := &fakeProfileStore{}
	app := &App{ctx: context.Background(), browserProfiles: backend}
	list, e := app.BrowserProfilesList()
	if e != nil || len(list.Records) != 1 || list.Applied {
		t.Fatal(e)
	}
	request := `{"schemaVersion":1,"name":"中文名称","config":{},"engine":{"engineId":"webview2","version":""}}`
	saved, e := app.BrowserProfileSave(request)
	if e != nil || saved.ID != "new-backend-id" || saved.Applied || !saved.RequiresLaunchValidation || backend.saved != request {
		t.Fatal("save contract changed")
	}
	loaded, e := app.BrowserProfileLoad("backend-id")
	if e != nil || loaded.ID != "backend-id" || loaded.Applied || backend.id != "backend-id" {
		t.Fatal("load contract changed")
	}
	deleted, e := app.BrowserProfileDelete("backend-id", "expected")
	if e != nil || !deleted.Deleted || backend.id != "backend-id" || backend.revision != "expected" {
		t.Fatal("delete lost CAS")
	}
	if app.browser != nil || app.browserEngine != nil {
		t.Fatal("persistence initialized launch controllers")
	}
	app.shutdownBrowserProfiles()
	app.shutdownBrowserProfiles()
	if backend.closed != 1 || app.browserProfiles != nil {
		t.Fatal("store not closed exactly once")
	}
	if _, e = app.BrowserProfilesList(); !errors.Is(e, browserprofiles.ErrClosed) || len(backend.calls) != 4 {
		t.Fatal("closed store reused")
	}
}
func TestBrowserProfilesBridgeRejectsInactiveOrUnboundedInputWithoutNativeAPI(t *testing.T) {
	app := &App{}
	if _, e := app.BrowserProfilesList(); !errors.Is(e, browserprofiles.ErrClosed) {
		t.Fatal(e)
	}
	backend := &fakeProfileStore{}
	app = &App{ctx: context.Background(), browserProfiles: backend}
	for _, input := range []string{"", strings.Repeat("x", (64<<10)+1)} {
		if _, e := app.BrowserProfileSave(input); !errors.Is(e, browserprofiles.ErrInput) {
			t.Fatal(e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.ctx = ctx
	if _, e := app.BrowserProfilesList(); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := app.BrowserProfileLoad("anything"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := app.BrowserProfileDelete("anything", "anything"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := app.BrowserProfileSave("{}"); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if len(backend.calls) != 0 {
		t.Fatal("invalid/cancelled API touched backend")
	}
}
func TestBrowserProfilesBridgePreservesSafeBackendErrors(t *testing.T) {
	backend := &fakeProfileStore{err: browserprofiles.ErrConflict}
	app := &App{ctx: context.Background(), browserProfiles: backend}
	if _, e := app.BrowserProfileSave("{}"); !errors.Is(e, browserprofiles.ErrConflict) {
		t.Fatal(e)
	}
	if _, e := app.BrowserProfileDelete("id", "revision"); !errors.Is(e, browserprofiles.ErrConflict) {
		t.Fatal(e)
	}
}
