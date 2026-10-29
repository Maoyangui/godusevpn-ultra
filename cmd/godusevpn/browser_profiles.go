package main

import (
	"context"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserprofiles"
)

type browserProfilesBackend interface {
	List(context.Context) (browserprofiles.ListResult, error)
	Load(context.Context, string) (browserprofiles.Record, error)
	Save(context.Context, []byte) (browserprofiles.Record, error)
	Delete(context.Context, string, string) (browserprofiles.DeleteResult, error)
	Close()
}

// The store is lazy and independent of the browser launch controller. Opening
// this API does not authorize a binding, create a browser or contact a service.
func (a *App) profileStore() (browserProfilesBackend, context.Context, error) {
	a.browserProfilesMu.Lock()
	defer a.browserProfilesMu.Unlock()
	if a.browserProfilesDone || a.ctx == nil {
		return nil, nil, browserprofiles.ErrClosed
	}
	if e := a.ctx.Err(); e != nil {
		return nil, nil, e
	}
	if a.browserProfiles == nil {
		store, e := browserprofiles.New()
		if e != nil {
			return nil, nil, e
		}
		a.browserProfiles = store
	}
	return a.browserProfiles, a.ctx, nil
}

// BrowserProfilesList reads only existing records; an absent directory returns
// an empty list. It never creates or launches a profile.
func (a *App) BrowserProfilesList() (browserprofiles.ListResult, error) {
	store, ctx, e := a.profileStore()
	if e != nil {
		return browserprofiles.ListResult{Records: []browserprofiles.Summary{}}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return store.List(ctx)
}

// BrowserProfileSave accepts only the non-secret draft schema. In particular,
// callers must omit config.binding; live proxy/service data stays in memory.
// A new record omits id/expectedRevision, while rename/update requires both.
func (a *App) BrowserProfileSave(recordJSON string) (browserprofiles.Record, error) {
	if len(recordJSON) == 0 || len(recordJSON) > 64<<10 {
		return browserprofiles.Record{}, browserprofiles.ErrInput
	}
	store, ctx, e := a.profileStore()
	if e != nil {
		return browserprofiles.Record{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return store.Save(ctx, []byte(recordJSON))
}

// Loading is a draft operation, not evidence that an engine or network setting
// has been applied. The existing browser launch gate remains authoritative.
func (a *App) BrowserProfileLoad(id string) (browserprofiles.Record, error) {
	store, ctx, e := a.profileStore()
	if e != nil {
		return browserprofiles.Record{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return store.Load(ctx, id)
}

// BrowserProfileDelete requires the latest full-file revision and marks only
// that one verified record handle for deletion. It never recurses a directory.
func (a *App) BrowserProfileDelete(id, expectedRevision string) (browserprofiles.DeleteResult, error) {
	store, ctx, e := a.profileStore()
	if e != nil {
		return browserprofiles.DeleteResult{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return store.Delete(ctx, id, expectedRevision)
}

func (a *App) shutdownBrowserProfiles() {
	a.browserProfilesMu.Lock()
	a.browserProfilesDone = true
	store := a.browserProfiles
	a.browserProfiles = nil
	a.browserProfilesMu.Unlock()
	if store != nil {
		store.Close()
	}
}
