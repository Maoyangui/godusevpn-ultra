package browserenginehost

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

type orderedClose struct {
	calls *[]string
	name  string
}

func (c orderedClose) Close() { *c.calls = append(*c.calls, c.name) }

type orderedJob struct {
	calls *[]string
	err   error
}
type observedJob struct {
	orderedJob
	count atomic.Uint32
}

func (j *observedJob) active() (uint32, error) { return j.count.Load(), nil }

func (j orderedJob) terminateAndWait(context.Context) error {
	*j.calls = append(*j.calls, "kill-and-verify-zero")
	return j.err
}
func (j orderedJob) active() (uint32, error) { return 0, j.err }
func (j orderedJob) close()                  { *j.calls = append(*j.calls, "handles") }

type orderedGuard struct {
	calls *[]string
	err   error
}

func (g orderedGuard) Install(context.Context, browserengineguard.InstallRequest) (browserengineguard.Result, error) {
	return browserengineguard.Result{}, ErrRefused
}
func (g orderedGuard) Report(context.Context, string) (browserengineguard.Result, error) {
	return browserengineguard.Result{}, ErrRefused
}
func (g orderedGuard) Remove(_ context.Context, id string) (browserengineguard.Result, error) {
	*g.calls = append(*g.calls, "remove-and-verify-absent")
	return browserengineguard.Result{SessionID: id, Verified: true, State: "removed_after_job_empty"}, g.err
}
func fixtureLifetime(calls *[]string, jobErr, guardErr error) *lifetime {
	s := &Session{id: strings.Repeat("a", 64), status: Status{State: "ready", Ready: true, CanNavigate: true}, done: make(chan struct{})}
	return &lifetime{session: s, cancel: func() { *calls = append(*calls, "cancel") }, frontend: orderedClose{calls, "frontend"}, backend: orderedClose{calls, "backend"}, process: orderedJob{calls, jobErr}, driver: orderedClose{calls, "protocol-pipe"}, code: orderedClose{calls, "code-pins"}, profile: orderedClose{calls, "profile-pins"}, guard: orderedGuard{calls, guardErr}, guardAttempted: true}
}
func TestLifetimeRevokeBeforePipeCloseAndGuardRemoval(t *testing.T) {
	var calls []string
	l := fixtureLifetime(&calls, nil, nil)
	if l.close() != nil || strings.Join(calls, ",") != "frontend,backend,cancel,kill-and-verify-zero,protocol-pipe,remove-and-verify-absent,code-pins,profile-pins,handles" {
		t.Fatal("unsafe teardown ordering", calls)
	}
	if l.session.Status().State != "closed" || l.session.Status().Ready {
		t.Fatal("bad closed state")
	}
	awaitSignal(t, l.session.Done())
	l.close()
	if len(calls) != 9 {
		t.Fatal("teardown repeated mutations")
	}
}
func TestLifetimeUnknownJobRetainsPinsAndFilters(t *testing.T) {
	var calls []string
	l := fixtureLifetime(&calls, errors.New("unknown Job"), nil)
	job := &observedJob{orderedJob: orderedJob{calls: &calls, err: ErrRefused}}
	job.count.Store(1)
	l.process = job
	if l.close() == nil || strings.Join(calls, ",") != "frontend,backend,cancel,kill-and-verify-zero" {
		t.Fatal("unknown Job lost pins or filters", calls)
	}
	if l.session.Status().State != "closing_or_unknown" || l.session.Status().CanNavigate {
		t.Fatal("unknown Job exposed ready")
	}
	select {
	case <-l.session.Done():
		t.Fatal("unknown Job declared finished")
	default:
	}
	unresolvedLifetimes.Lock()
	retained := unresolvedLifetimes.items[l.session.id] == l
	unresolvedLifetimes.Unlock()
	if !retained {
		t.Fatal("errored owner did not retain code pins against GC")
	}
	job.count.Store(0)
	awaitSignal(t, l.session.Done())
	if strings.Join(calls, ",") != "frontend,backend,cancel,kill-and-verify-zero,protocol-pipe,remove-and-verify-absent,code-pins,profile-pins,handles" {
		t.Fatal("late zero was not independently observed before cleanup", calls)
	}
}
func TestLifetimeUnknownRemoveNeverRetriesOrClaimsRemoved(t *testing.T) {
	var calls []string
	l := fixtureLifetime(&calls, nil, ErrRefused)
	if l.close() == nil || l.session.Status().State != "closed_filters_unknown" {
		t.Fatal("unknown removal claimed success")
	}
	if strings.Contains(strings.Join(calls, ","), "code-pins") || strings.Contains(strings.Join(calls, ","), "profile-pins") {
		t.Fatal("unknown filter cleanup released code/version usage or profile lease", calls)
	}
	unresolvedLifetimes.Lock()
	retained := unresolvedLifetimes.items[l.session.id] == l
	delete(unresolvedLifetimes.items, l.session.id) // fake-only test ownership
	unresolvedLifetimes.Unlock()
	if !retained {
		t.Fatal("unknown cleanup lost strong ownership of usage lease")
	}
	before := len(calls)
	l.close()
	if len(calls) != before {
		t.Fatal("unknown mutation retried")
	}
	awaitSignal(t, l.session.Done())
}
