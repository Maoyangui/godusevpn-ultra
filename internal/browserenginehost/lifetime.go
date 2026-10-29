package browserenginehost

import (
	"context"
	"sync"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
)

type ownedJob interface {
	terminateAndWait(context.Context) error
	active() (uint32, error)
	close()
}
type closeOnly interface{ Close() }

// lifetime owns a single browser tree. Revocation closes both network gates
// before killing the Job. Pipe closure and guard removal follow a verified
// empty Job; synchronous Windows pipe readers must not be closed first.
type lifetime struct {
	session               *Session
	cancel                context.CancelFunc
	frontend, backend     closeOnly
	process               ownedJob
	code, profile         closeOnly
	driver                closeOnly
	guard                 Guard
	guardAttempted        bool
	revokeOnce, closeOnce sync.Once
	finishOnce            sync.Once
	killErr, closeErr     error
}

var unresolvedLifetimes = struct {
	sync.Mutex
	items map[string]*lifetime
}{items: map[string]*lifetime{}}

func (l *lifetime) state(state, detail string) {
	l.session.mu.Lock()
	l.session.status.Ready = false
	l.session.status.CanNavigate = false
	l.session.status.State = state
	l.session.status.Detail = detail
	l.session.mu.Unlock()
}
func (l *lifetime) revoke() {
	l.revokeOnce.Do(func() {
		l.state("closing", "本会话代理已封闭，正在确认浏览器进程树退出。")
		if l.frontend != nil {
			l.frontend.Close()
		}
		if l.backend != nil {
			l.backend.Close()
		}
		if l.cancel != nil {
			l.cancel()
		}
		if l.process != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			l.killErr = l.process.terminateAndWait(ctx)
			cancel()
			if l.killErr != nil {
				l.state("closing_or_unknown", "代理已封闭；进程树退出尚未证实，保留代码句柄和防火墙限制。")
			}
		}
	})
}
func (l *lifetime) close() error {
	l.closeOnce.Do(func() {
		l.revoke()
		if l.killErr != nil {
			l.closeErr = ErrRefused
			// Retain the full object graph, including os.File pins, even when a
			// caller discards the errored Session. GC finalizers must not release
			// those handles while Job exit is unknown.
			unresolvedLifetimes.Lock()
			unresolvedLifetimes.items[l.session.id] = l
			unresolvedLifetimes.Unlock()
			go l.awaitEmpty()
			return
		}
		l.closeErr = l.finish()
	})
	return l.closeErr
}
func (l *lifetime) awaitEmpty() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		count, err := l.process.active()
		if err == nil && count == 0 {
			_ = l.finish()
			if l.session.Status().State == "closed" {
				unresolvedLifetimes.Lock()
				delete(unresolvedLifetimes.items, l.session.id)
				unresolvedLifetimes.Unlock()
			}
			return
		}
	}
}
func (l *lifetime) finish() (result error) {
	l.finishOnce.Do(func() {
		if l.driver != nil {
			l.driver.Close()
		}
		filtersUnknown := false
		if l.guardAttempted && l.guard != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			result, err := l.guard.Remove(ctx, l.session.id)
			cancel()
			if !validRemovedGuard(result, l.session.id, err) {
				l.rememberGuardDiagnostic(result, err)
				filtersUnknown = true
			}
		}
		if !filtersUnknown {
			if l.code != nil {
				l.code.Close()
			}
			if l.profile != nil {
				l.profile.Close()
			}
		}
		if l.process != nil {
			l.process.close()
		}
		if filtersUnknown {
			result = ErrRefused
			// CodeLease also owns the cross-process cache version usage lease.
			// Retain it, and its os.File objects against finalizers, while filter
			// cleanup is unknown; another manager must not delete this version.
			unresolvedLifetimes.Lock()
			unresolvedLifetimes.items[l.session.id] = l
			unresolvedLifetimes.Unlock()
			l.state("closed_filters_unknown", "进程树已确认退出；防火墙撤销结果未知，保留限制且没有重试。")
		} else {
			detail := "本会话进程树已退出，已核对本会话防火墙限制撤销。"
			if !l.guardAttempted {
				detail = "本次启动资源已关闭；规则安装未执行，本次未调用撤销。"
			}
			l.state("closed", detail)
		}
		close(l.session.done)
	})
	return result
}
func validInstalledGuard(r browserengineguard.Result, id string, err error) bool {
	return err == nil && r.SessionID == id && r.Installed && r.Verified && r.State == "filters_verified" && r.Diagnostic == nil
}
func validRemovedGuard(r browserengineguard.Result, id string, err error) bool {
	return err == nil && r.SessionID == id && !r.Installed && r.Verified && r.State == "removed_after_job_empty" && r.Diagnostic == nil
}
