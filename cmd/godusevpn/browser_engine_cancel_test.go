package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
	"github.com/Maoyangui/godusevpn/internal/browserproxy"
)

func engineRunForTest(t *testing.T, c *browserEngineController) *browserEngineRun {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		t.Fatal("expected tracked startup owner")
	}
	return c.current
}

func awaitEngineRun(t *testing.T, r *browserEngineRun) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatal("synthetic startup did not finish")
	}
}

func engineResultPromptly(t *testing.T, call func() BrowserEngineLaunchResult) BrowserEngineLaunchResult {
	t.Helper()
	result := make(chan BrowserEngineLaunchResult, 1)
	go func() { result <- call() }()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("status/cancel blocked on synthetic startup cleanup")
		return BrowserEngineLaunchResult{}
	}
}

func TestEngineControllerBeginCancelAndFreshRetry(t *testing.T) {
	c, _, _ := engineFixture(t)
	entered := make(chan context.Context, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	c.deps.start = func(ctx context.Context, _ *browserengine.Manager, _ browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
		entered <- ctx
		<-release
		return nil, ctx.Err()
	}
	started := engineResultPromptly(t, func() BrowserEngineLaunchResult {
		result, err := c.beginLaunch(engineConfigFixture, engineProxyFixture)
		if err != nil {
			t.Error("valid asynchronous launch rejected")
		}
		return result
	})
	r := engineRunForTest(t, c)
	ctx := <-entered
	if !engineSessionID.MatchString(started.StartID) || started.Status.State != "starting" || !started.CanCancelStart || started.Code != "starting" {
		t.Fatal("begin did not return cancellable startup identity")
	}
	for _, id := range []string{"", strings.ToUpper(started.StartID), strings.Repeat("z", 64), strings.Repeat("0", 64)} {
		if _, err := c.cancelStart(id); err == nil {
			t.Fatal("invalid or unrelated cancellation token accepted")
		}
	}
	if ctx.Err() != nil {
		t.Fatal("unrelated cancellation changed active startup")
	}
	for i := 0; i < 3; i++ {
		cancelled := engineResultPromptly(t, func() BrowserEngineLaunchResult {
			result, err := c.cancelStart(started.StartID)
			if err != nil {
				t.Error("matching pending cancellation rejected")
			}
			return result
		})
		if cancelled.Status.State != "cancelling" || cancelled.CanCancelStart || cancelled.Status.Ready || cancelled.Code != "cancellation_requested" {
			t.Fatal("requested cancellation claimed terminal or ready")
		}
	}
	if ctx.Err() == nil {
		t.Fatal("pending startup context was not cancelled")
	}
	if _, err := c.beginLaunch(engineConfigFixture, engineProxyFixture); err == nil {
		t.Fatal("new launch admitted before cancelled backend returned")
	}
	releaseOnce.Do(func() { close(release) })
	awaitEngineRun(t, r)
	c.finish(r)
	if result := c.status(); result.Status.State != "start_cancelled" || result.Code != "start_cancelled" || result.CanCancelStart || result.StartID != started.StartID {
		t.Fatal("completed cancellation terminal lost")
	}
	if _, err := c.cancelStart(started.StartID); err != nil {
		t.Fatal("terminal cancellation was not idempotent")
	}

	// The next run gets its own token and context. A delayed old UI callback
	// cannot cancel it, even though both requests use the same saved profile.
	enteredAgain := make(chan context.Context, 1)
	releaseAgain := make(chan struct{})
	defer close(releaseAgain)
	c.deps.start = func(ctx context.Context, _ *browserengine.Manager, _ browserenginehost.Config, _ browserenginehost.Guard) (browserEngineSession, error) {
		enteredAgain <- ctx
		<-releaseAgain
		return nil, ctx.Err()
	}
	next, err := c.beginLaunch(engineConfigFixture, engineProxyFixture)
	if err != nil || next.StartID == started.StartID {
		t.Fatal("retry reused cancelled startup identity")
	}
	newContext := <-enteredAgain
	if _, err := c.cancelStart(started.StartID); err == nil || newContext.Err() != nil {
		t.Fatal("stale token cancelled a new startup")
	}
	if _, err := c.cancelStart(next.StartID); err != nil {
		t.Fatal("fresh startup not cancellable")
	}
}

func TestEngineControllerCancelledLateSessionClosedExactlyOnce(t *testing.T) {
	c, session, _ := engineFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		close(entered)
		<-release
		return session, nil // Deliberately ignores context, modeling a late API.
	}
	started, err := c.beginLaunch(engineConfigFixture, engineProxyFixture)
	if err != nil {
		t.Fatal(err)
	}
	r := engineRunForTest(t, c)
	<-entered
	if _, err := c.cancelStart(started.StartID); err != nil {
		t.Fatal(err)
	}
	releaseOnce.Do(func() { close(release) })
	awaitEngineRun(t, r)
	c.finish(r)
	result, err := c.cancelStart(started.StartID)
	if err != nil || result.Status.State != "start_cancelled" || result.Status.Ready || result.Status.CanNavigate || session.closes.Load() != 1 {
		t.Fatal("late session escaped or repeated cancellation mutated it again")
	}
	if c.navigate("https://example.invalid/") == nil {
		t.Fatal("cancelled late session remained navigable")
	}
}

type blockingEngineClose struct {
	*fakeEngineSession
	entered chan struct{}
	release chan struct{}
	called  atomic.Int32
}

func (s *blockingEngineClose) Close() error {
	s.called.Add(1)
	close(s.entered)
	<-s.release
	return s.fakeEngineSession.Close()
}

func TestEngineControllerCancelAndStatusDoNotWaitForCleanup(t *testing.T) {
	c, session, _ := engineFixture(t)
	returned := &blockingEngineClose{fakeEngineSession: session, entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(returned.release) })
	startEntered, startRelease := make(chan struct{}), make(chan struct{})
	c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		close(startEntered)
		<-startRelease
		return returned, nil
	}
	started, err := c.beginLaunch(engineConfigFixture, engineProxyFixture)
	if err != nil {
		t.Fatal(err)
	}
	r := engineRunForTest(t, c)
	<-startEntered
	if _, err := c.cancelStart(started.StartID); err != nil {
		t.Fatal(err)
	}
	close(startRelease)
	<-returned.entered
	result := engineResultPromptly(t, c.status)
	if result.Status.Ready || result.Status.CanNavigate || result.CanCancelStart || result.Status.State != "cancelling" {
		t.Fatal("blocking cleanup misreported as completed or ready")
	}
	engineResultPromptly(t, func() BrowserEngineLaunchResult { result, _ := c.cancelStart(started.StartID); return result })
	if returned.called.Load() != 1 {
		t.Fatal("duplicate cancellation invoked Close again")
	}
	if _, err := c.beginLaunch(engineConfigFixture, engineProxyFixture); err == nil {
		t.Fatal("new run overlapped a blocking cleanup")
	}
	releaseOnce.Do(func() { close(returned.release) })
	awaitEngineRun(t, r)
	if c.snapshot(r).Status.State != "start_cancelled" || returned.called.Load() != 1 {
		t.Fatal("confirmed late cleanup did not finish cancellation")
	}
}

func TestEngineControllerUnknownCancelledOwnerBlocksRetry(t *testing.T) {
	for _, state := range []string{"ready", "closed_filters_unknown", "closed"} {
		t.Run(state, func(t *testing.T) {
			c, session, _ := engineFixture(t)
			session.closeErr = errors.New("synthetic cleanup uncertainty")
			if state != "ready" {
				session.status.State, session.status.Ready, session.status.CanNavigate = state, false, false
				session.once.Do(func() { close(session.done) })
			}
			entered, release := make(chan struct{}), make(chan struct{})
			c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
				close(entered)
				<-release
				return session, errors.New("synthetic failed startup")
			}
			started, err := c.beginLaunch(engineConfigFixture, engineProxyFixture)
			if err != nil {
				t.Fatal(err)
			}
			r := engineRunForTest(t, c)
			<-entered
			if _, err := c.cancelStart(started.StartID); err != nil {
				t.Fatal(err)
			}
			close(release)
			awaitEngineRun(t, r)
			c.finish(r)
			result := c.status()
			if result.Status.Ready || result.Status.CanNavigate || result.Code != "cleanup_unconfirmed" || result.CanCancelStart || result.Status.State == "start_cancelled" {
				t.Fatal("unknown owner falsely became terminal cancellation")
			}
			if _, err := c.cancelStart(started.StartID); err != nil || session.closes.Load() != 1 {
				t.Fatal("unknown repeated cancellation mutated cleanup")
			}
			if _, err := c.beginLaunch(engineConfigFixture, engineProxyFixture); err == nil {
				t.Fatal("unknown process/filter owner forgotten")
			}
		})
	}
}

func TestEngineControllerReadyUsesCloseAndRefusesStartCancellation(t *testing.T) {
	c, session, _ := engineFixture(t)
	result, err := c.launch(engineConfigFixture, engineProxyFixture)
	if err != nil || result.CanCancelStart || result.Code != "ready" {
		t.Fatal("ready publication invalid")
	}
	if _, err := c.cancelStart(result.StartID); err == nil || session.closes.Load() != 0 {
		t.Fatal("start cancellation closed an already published session")
	}
	if err := c.close(false); err != nil {
		t.Fatal(err)
	}
	if session.closes.Load() != 1 {
		t.Fatal("ordinary close did not own ready session")
	}
}

type earlyDoneEngineClose struct {
	*fakeEngineSession
	entered chan struct{}
	release chan struct{}
}

func (s *earlyDoneEngineClose) Close() error {
	_ = s.fakeEngineSession.Close()
	close(s.entered)
	<-s.release
	return errors.New("synthetic contradiction: close result still unknown")
}

func TestEngineControllerDoneCannotClearOwnerBeforeCloseResult(t *testing.T) {
	c, session, _ := engineFixture(t)
	returned := &earlyDoneEngineClose{fakeEngineSession: session, entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(returned.release) })
	c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
		return returned, nil
	}
	if _, err := c.launch(engineConfigFixture, engineProxyFixture); err != nil {
		t.Fatal(err)
	}
	r := engineRunForTest(t, c)
	closed := make(chan error, 1)
	go func() { closed <- c.close(false) }()
	<-returned.entered
	c.finish(r)
	if result := engineResultPromptly(t, c.status); result.Status.State != "closing_or_unknown" || result.Code != "cleanup_unconfirmed" {
		t.Fatal("Done or closed status bypassed pending Close result")
	}
	if _, err := c.beginLaunch(engineConfigFixture, engineProxyFixture); err == nil {
		t.Fatal("pending Close released ownership")
	}
	releaseOnce.Do(func() { close(returned.release) })
	if err := <-closed; err == nil {
		t.Fatal("contradictory Close error ignored")
	}
	c.finish(r)
	if result := c.status(); result.Status.State != "closing_or_unknown" || result.Code != "cleanup_unconfirmed" {
		t.Fatal("Close error was discarded because Done happened to be closed")
	}
	if _, err := c.beginLaunch(engineConfigFixture, engineProxyFixture); err == nil {
		t.Fatal("unknown Close error released ownership")
	}
}

func TestEngineControllerCancellationAndReadyPublicationAreSerialized(t *testing.T) {
	for i := 0; i < 40; i++ {
		c, session, _ := engineFixture(t)
		entered, release := make(chan struct{}), make(chan struct{})
		c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
			close(entered)
			<-release
			return session, nil
		}
		started, err := c.beginLaunch(engineConfigFixture, engineProxyFixture)
		if err != nil {
			t.Fatal(err)
		}
		r := engineRunForTest(t, c)
		<-entered
		cancelResult := make(chan error, 1)
		go func() { _, err := c.cancelStart(started.StartID); cancelResult <- err }()
		close(release)
		cancelErr := <-cancelResult
		awaitEngineRun(t, r)
		result := c.snapshot(r)
		if cancelErr == nil {
			if result.Status.Ready || result.Status.CanNavigate || result.Status.State != "start_cancelled" || session.closes.Load() != 1 {
				t.Fatal("accepted cancellation was followed by ready publication")
			}
		} else if !result.Status.Ready || result.Status.State != "ready" || session.closes.Load() != 0 {
			t.Fatal("ready winner was silently closed by rejected cancellation")
		}
		_ = c.close(false)
	}
}

func TestEngineControllerAsyncFailureCodesContainNoPrivateError(t *testing.T) {
	for _, test := range []struct{ stage, code string }{
		{"missing", "host_not_ready"}, {"guard", "helper_required"}, {"manager", "version_store_unavailable"},
		{"not-ready", "host_not_ready"}, {"start", "host_start_failed"},
	} {
		t.Run(test.stage, func(t *testing.T) {
			c, _, _ := engineFixture(t)
			privateErr := errors.New("PRIVATE_ERROR_WITH_SYNTHETIC_PASSWORD")
			switch test.stage {
			case "missing":
				c.deps.start = nil
			case "guard":
				c.deps.guard = func() (browserenginehost.Guard, error) { return nil, privateErr }
			case "manager":
				c.deps.manager = func(browserproxy.Config) (*browserengine.Manager, error) { return nil, privateErr }
			case "not-ready":
				c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
					return nil, browserenginehost.ErrNotReady
				}
			case "start":
				c.deps.start = func(context.Context, *browserengine.Manager, browserenginehost.Config, browserenginehost.Guard) (browserEngineSession, error) {
					return nil, privateErr
				}
			}
			r, err := c.begin(engineConfigFixture, engineProxyFixture)
			if err != nil {
				t.Fatal("asynchronous operation was not accepted")
			}
			awaitEngineRun(t, r)
			c.finish(r)
			result := c.status()
			if result.Status.State != "start_failed" || result.Code != test.code || result.CanCancelStart || result.Status.Ready {
				t.Fatal("failure lost its fixed actionable stage")
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "PRIVATE_") || strings.Contains(string(encoded), "SYNTHETIC_") || strings.Contains(string(encoded), "proxy.example.invalid") {
				t.Fatal("private failure data exposed by async status")
			}
		})
	}
}

func TestEngineControllerPartialPublicationNeverClaimsReadyOrTerminal(t *testing.T) {
	c, session, _ := engineFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &browserEngineRun{ctx: ctx, cancel: cancel, id: strings.Repeat("b", 64), session: session, starting: true, done: make(chan struct{})}
	if result := c.snapshot(r); result.Status.State != "starting" || result.Status.Ready || result.Status.CanNavigate || !result.CanCancelStart {
		t.Fatal("unpublished returned Session claimed ready")
	}
	r.stopped, r.cancelledStart = true, true
	if err := r.closeSession(); err != nil {
		t.Fatal(err)
	}
	if result := c.snapshot(r); result.Status.State != "cancelling" || result.Status.Ready || result.CanCancelStart {
		t.Fatal("closed Session claimed terminal while startup worker still owns resources")
	}
}
