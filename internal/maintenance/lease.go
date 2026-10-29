// Package maintenance serializes optional machine privacy mutations and the
// complete cleanup/uninstall window. It is not an authorization mechanism and
// never participates in the VPN startup or tunnel path.
package maintenance

import (
	"errors"
	"sync/atomic"
)

var ErrBusy = errors.New("机器隐私维护正在进行；未执行此次修改，请稍后重试")
var ErrAbandoned = errors.New("上次维护进程意外退出；本次未执行修改，请先核对备份与规则后重试")
var ErrUnavailable = errors.New("机器隐私维护门禁未就绪；请先授权管理员助手，未影响 VPN")

type Lease struct {
	owner  uint32
	thread func() uint32
	finish func() error
	active atomic.Bool
}

func newLease(owner uint32, thread func() uint32, finish func() error) *Lease {
	l := &Lease{owner: owner, thread: thread, finish: finish}
	l.active.Store(true)
	return l
}

// A lease cannot be borrowed by another goroutine/OS thread, recursively
// acquired, fabricated by callers, or used after the complete window closes.
func (l *Lease) Check() error {
	if l == nil || !l.active.Load() || l.thread == nil || l.thread() != l.owner {
		return errors.New("维护许可已关闭或不属于当前执行线程；未执行修改")
	}
	return nil
}
func (l *Lease) Close() error {
	if l == nil || !l.active.Load() {
		return nil
	}
	if err := l.Check(); err != nil {
		return err
	}
	if l.active.CompareAndSwap(true, false) {
		return l.finish()
	}
	return nil
}

func Use(existing *Lease) (*Lease, func(), error) {
	if existing != nil {
		if err := existing.Check(); err != nil {
			return nil, nil, err
		}
		return existing, func() {}, nil
	}
	l, err := Acquire()
	if err != nil {
		return nil, nil, err
	}
	return l, func() { _ = l.Close() }, nil
}
