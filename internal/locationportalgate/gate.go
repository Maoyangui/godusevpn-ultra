// Package locationportalgate owns only four fixed inbound WFP restrictions for
// Windows Device Portal. It never starts a service, configures IPListen, writes
// a credential, or changes the VPN firewall namespace.
package locationportalgate

import (
	"context"
	"errors"
	"sync"
)

const HTTPPort = 58080
const HTTPSPort = 58443

var ErrUnavailable = errors.New("LOCATION_PORTAL_GATE_UNAVAILABLE")
var ErrUnknown = errors.New("LOCATION_PORTAL_GATE_UNCONFIRMED")
var ErrServiceActive = errors.New("LOCATION_PORTAL_GATE_SERVICE_NOT_DISABLED_AND_STOPPED")
var gateMu sync.Mutex

type GateReport struct {
	Status               string `json:"status"`
	Installed            bool   `json:"installed"`
	Verified             bool   `json:"verified"`
	LoopbackGateVerified bool   `json:"loopbackGateVerified"`
	ExpectedFilters      int    `json:"expectedFilters"`
	InstalledFilters     int    `json:"installedFilters"`
	HTTPPort             int    `json:"httpPort"`
	HTTPSPort            int    `json:"httpsPort"`
	Detail               string `json:"detail"`
}

type snapshot struct {
	base  bool
	count int
	valid bool
}

func (s snapshot) absent() bool   { return s.valid && s.count == 0 }
func (s snapshot) complete() bool { return s.valid && s.base && s.count == 4 }

type engine interface {
	snapshot() (snapshot, error)
	transaction(func() error) error
	install() error
	remove() error
	serviceSafe() error
	close()
}
type Gate struct {
	open func(context.Context) (engine, error)
}

// NewGate does not probe/load native APIs or alter any system object.
func NewGate() (*Gate, error) { return newNativeGate() }

func result(s snapshot, err error) (GateReport, error) {
	r := GateReport{Status: "unknown", Installed: s.count > 0, ExpectedFilters: 4, InstalledFilters: s.count, HTTPPort: HTTPPort, HTTPSPort: HTTPSPort, Detail: "入站限制尚未完整确认；不允许继续准备或启动设备门户。"}
	if err != nil {
		return r, err
	}
	if s.complete() {
		r.Status = "filters_verified"
		r.Installed = true
		r.Verified = true
		r.LoopbackGateVerified = true
		r.Detail = "已读回本模块固定端口的四条持久非回环入站限制；真实网络仍需独立验证。"
		return r, nil
	}
	if s.absent() {
		r.Status = "absent_verified"
		r.Installed = false
		r.Verified = true
		r.Detail = "已确认本模块固定入站限制不存在；此状态不允许启动设备门户。"
		return r, nil
	}
	return r, ErrUnknown
}
func (g *Gate) enter(ctx context.Context) (engine, error) {
	if g == nil || g.open == nil || ctx == nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	return g.open(ctx)
}
func (g *Gate) Inspect(ctx context.Context) (GateReport, error) {
	gateMu.Lock()
	defer gateMu.Unlock()
	e, err := g.enter(ctx)
	if err != nil {
		return result(snapshot{}, err)
	}
	defer e.close()
	s, err := e.snapshot()
	if ctx.Err() != nil {
		return result(s, ErrUnknown)
	}
	return result(s, err)
}
func (g *Gate) Ensure(ctx context.Context) (GateReport, error) {
	gateMu.Lock()
	defer gateMu.Unlock()
	e, err := g.enter(ctx)
	if err != nil {
		return result(snapshot{}, err)
	}
	defer e.close()
	err = e.transaction(func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s, err := e.snapshot()
		if err != nil {
			return err
		}
		if s.complete() {
			return nil
		}
		if !s.absent() {
			return ErrUnknown
		}
		if err = e.install(); err != nil {
			return err
		}
		s, err = e.snapshot()
		if err != nil || !s.complete() {
			return ErrUnknown
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	})
	if err != nil {
		return result(snapshot{}, err)
	}
	s, err := e.snapshot()
	if err != nil || ctx.Err() != nil || !s.complete() {
		return result(s, ErrUnknown)
	}
	return result(s, nil)
}
func (g *Gate) Remove(ctx context.Context) (GateReport, error) {
	gateMu.Lock()
	defer gateMu.Unlock()
	e, err := g.enter(ctx)
	if err != nil {
		return result(snapshot{}, err)
	}
	defer e.close()
	hadProtection := false
	err = e.transaction(func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := e.serviceSafe(); err != nil {
			return err
		}
		s, err := e.snapshot()
		if err != nil {
			return err
		}
		if !s.absent() && !s.complete() {
			return ErrUnknown
		}
		hadProtection = s.complete()
		if s.base || s.count > 0 {
			if err = e.remove(); err != nil {
				return err
			}
		}
		s, err = e.snapshot()
		if err != nil || !s.absent() || s.base {
			return ErrUnknown
		}
		if err = e.serviceSafe(); err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil
	})
	if err != nil {
		if hadProtection {
			_ = preserveProtection(e)
		}
		return result(snapshot{}, err)
	}
	if err = e.serviceSafe(); err != nil {
		if hadProtection {
			_ = preserveProtection(e)
		}
		return result(snapshot{}, err)
	}
	s, err := e.snapshot()
	if err != nil || ctx.Err() != nil || !s.absent() || s.base {
		if hadProtection {
			_ = preserveProtection(e)
		}
		return result(s, ErrUnknown)
	}
	return result(s, nil)
}

// An uncertain removal is never treated as permission to start the service.
// If a previously verified set may have been committed away, restore that same
// fixed protection best-effort, even when the caller cancelled. Never broaden
// this into taking over an alien/partial namespace or retrying portal startup.
func preserveProtection(e engine) error {
	return e.transaction(func() error {
		s, err := e.snapshot()
		if err != nil {
			return err
		}
		if s.complete() {
			return nil
		}
		if !s.absent() {
			return ErrUnknown
		}
		if err = e.install(); err != nil {
			return err
		}
		s, err = e.snapshot()
		if err != nil || !s.complete() {
			return ErrUnknown
		}
		return nil
	})
}
