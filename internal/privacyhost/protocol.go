// Package privacyhost isolates optional privileged privacy changes from the
// ordinary desktop UI. It has no VPN lifecycle, browser launch, or shell RPC.
package privacyhost

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Maoyangui/godusevpn/internal/appfirewall"
	"github.com/Maoyangui/godusevpn/internal/browserengineguard"
	"github.com/Maoyangui/godusevpn/internal/locationportal"
	prep "github.com/Maoyangui/godusevpn/internal/locationportalprepare"
	"github.com/Maoyangui/godusevpn/internal/maintenance"
	"github.com/Maoyangui/godusevpn/internal/privacy"
)

const helperFlag = "--ultra-privacy-helper"
const protocolVersion = 1
const maxRequest = 64 << 10
const maxResponse = 1 << 20
const callTimeout = 25 * time.Second
const idleTimeout = 60 * time.Second

var ErrUnsupported = errors.New("privileged privacy helper requires 64-bit Windows")
var allowedMethods = map[string]bool{"Report": true, "Apply": true, "Restore": true, "RestoreAll": true, "FirewallReport": true, "FirewallApply": true, "FirewallRemove": true, "MaintenanceReady": true, "BrowserGuardInstall": true, "BrowserGuardReport": true, "BrowserGuardRemove": true, "BrowserGuardStatus": true, "LocationPortalPreparationReport": true, "LocationPortalPreparationPrepare": true, "LocationPortalPreparationRestore": true}
var machineIDs = []string{"windows-advertising-id", "windows-online-speech", "windows-clipboard-history", "windows-clipboard-cloud"}

type request struct {
	Version int             `json:"version"`
	Session string          `json:"session"`
	ID      uint64          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type response struct {
	Version int             `json:"version"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}
type hello struct {
	Version int    `json:"version"`
	Session string `json:"session"`
	PID     uint32 `json:"pid"`
	Parent  uint32 `json:"parent"`
}

func strictDecode(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}
func writeFrame(w io.Writer, item any, limit int) error {
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if len(b) == 0 || len(b) > limit {
		return errors.New("privacy helper frame exceeds limit")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(b)))
	if err = writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, b)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func readFrame(r io.Reader, out any, limit int) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || n > uint32(limit) {
		return errors.New("invalid privacy helper frame size")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return strictDecode(b, out)
}
func sameSession(a, b string) bool {
	return len(a) == 64 && len(b) == 64 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func nullParams(p json.RawMessage) bool {
	return len(p) == 0 || bytes.Equal(bytes.TrimSpace(p), []byte("null"))
}
func machineID(id string) bool {
	for _, known := range machineIDs {
		if id == known {
			return true
		}
	}
	return false
}

type policyManager interface {
	Report() (privacy.Report, error)
	Apply(string) (privacy.ActionResult, error)
	Restore(string) (privacy.ActionResult, error)
}
type firewallManager interface {
	Report() (appfirewall.Report, error)
	Apply(appfirewall.Rule) (appfirewall.Result, error)
	Remove(string) (appfirewall.Result, error)
}
type dispatcher struct {
	maintenanceOnly   bool
	lease             *maintenance.Lease
	gate              func() (*maintenance.Lease, error)
	policy            policyManager
	firewall          firewallManager
	browser           browserGuardManager
	browserDiagnostic *browserengineguard.Diagnostic
	portalPreparation *prep.Manager
}

type browserGuardManager interface {
	Install(browserengineguard.InstallRequest) (browserengineguard.Result, error)
	Report(string) (browserengineguard.Result, error)
	Remove(string) (browserengineguard.Result, error)
	Close()
}

func machineReport(r privacy.Report) privacy.Report {
	out := r
	out.Controls = []privacy.ControlState{}
	out.Capabilities = []privacy.Capability{}
	for _, c := range r.Controls {
		if machineID(c.ID) && c.RequiresAdmin && c.Scope == "machine / Windows" {
			out.Controls = append(out.Controls, c)
		}
	}
	out.Warnings = append(append([]string{}, r.Warnings...), "此报告仅代表机器级策略；当前登录用户的浏览器和 HKCU 设置由普通权限界面单独检查。")
	return out
}

func (d dispatcher) call(method string, p json.RawMessage) (any, error) {
	if !allowedMethods[method] {
		return nil, errors.New("helper method is not allowed")
	}
	if d.maintenanceOnly {
		if method != "MaintenanceReady" || !nullParams(p) {
			return nil, errors.New("此助手仅初始化维护门禁，不读取或修改机器策略/WFP")
		}
		return struct {
			Ready bool   `json:"ready"`
			Scope string `json:"scope"`
		}{true, "synchronization_only"}, nil
	}
	if method == "MaintenanceReady" {
		return nil, errors.New("MaintenanceReady requires the fixed maintenance-only helper mode")
	}
	if method != "Report" && method != "FirewallReport" && method != "BrowserGuardReport" && method != "BrowserGuardStatus" {
		if d.lease != nil {
			if err := d.lease.Check(); err != nil {
				return nil, err
			}
		} else if d.gate != nil {
			l, err := d.gate()
			if err != nil {
				return nil, err
			}
			defer l.Close()
			d.lease = l
		}
		if d.lease != nil {
			if m, ok := d.policy.(*privacy.Manager); ok {
				d.policy = m.WithLease(d.lease)
			}
			if m, ok := d.firewall.(*appfirewall.Manager); ok {
				d.firewall = m.WithLease(d.lease)
			}
		}
	}
	switch method {
	case "BrowserGuardInstall", "BrowserGuardReport", "BrowserGuardRemove", "BrowserGuardStatus":
		return d.callBrowserGuard(method, p)
	case "LocationPortalPreparationReport", "LocationPortalPreparationPrepare", "LocationPortalPreparationRestore":
		if !nullParams(p) {
			return nil, errors.New("location portal preparation accepts no parameters")
		}
		if d.portalPreparation == nil {
			return prep.Report{Status: "unsupported", HTTPPort: prep.HTTPPort, HTTPSPort: prep.HTTPSPort, Detail: "固定的 Windows Device Portal 准备后端不可用；未修改系统。"}, nil
		}
		var r prep.Report
		switch method {
		case "LocationPortalPreparationReport":
			r = d.portalPreparation.Report()
		case "LocationPortalPreparationPrepare":
			r = d.portalPreparation.Prepare()
		case "LocationPortalPreparationRestore":
			r = d.portalPreparation.Restore()
		}
		var c *locationportal.Config
		if r.Ready {
			c = d.portalPreparation.Connection()
		}
		var wire *preparedLocationConfig
		if c != nil {
			wire = &preparedLocationConfig{Port: c.Port, CertificateSHA256: c.CertificateSHA256, Username: c.Username, Password: c.Password}
		}
		return struct {
			Report prep.Report             `json:"report"`
			Config *preparedLocationConfig `json:"config,omitempty"`
		}{r, wire}, nil
	case "Report":
		if !nullParams(p) {
			return nil, errors.New("Report accepts no parameters")
		}
		r, e := d.policy.Report()
		return machineReport(r), e
	case "Apply", "Restore":
		var id string
		if err := strictDecode(p, &id); err != nil {
			return nil, errors.New("expected a fixed machine policy ID")
		}
		if !machineID(id) {
			return nil, errors.New("helper rejects user-scoped or unknown privacy controls")
		}
		if method == "Apply" {
			return d.policy.Apply(id)
		}
		return d.policy.Restore(id)
	case "RestoreAll":
		if !nullParams(p) {
			return nil, errors.New("RestoreAll accepts no parameters")
		}
		// Never invoke the general manager's RestoreAll: even a malformed or
		// older journal must not turn this endpoint into an HKCU write channel.
		r, err := d.policy.Report()
		if err != nil {
			return nil, err
		}
		if !r.BackupHealthy {
			return nil, errors.New("机器隐私备份异常，不能确认已管理设置；拒绝宣称恢复完成，请保留恢复工具并检查备份")
		}
		var out []privacy.ActionResult
		var errs []error
		for _, c := range machineReport(r).Controls {
			if c.Managed {
				result, e := d.policy.Restore(c.ID)
				out = append(out, result)
				if e != nil {
					errs = append(errs, e)
				}
			}
		}
		return out, errors.Join(errs...)
	case "FirewallReport":
		if !nullParams(p) {
			return nil, errors.New("FirewallReport accepts no parameters")
		}
		return d.firewall.Report()
	case "FirewallApply":
		var rule appfirewall.Rule
		if err := strictDecode(p, &rule); err != nil {
			return nil, errors.New("invalid program rule")
		}
		return d.firewall.Apply(rule)
	case "FirewallRemove":
		var id string
		if err := strictDecode(p, &id); err != nil {
			return nil, errors.New("expected rule ID")
		}
		return d.firewall.Remove(id)
	}
	return nil, errors.New("unknown helper operation")
}

// executeElevated keeps the direct CLI path on exactly the same capability
// whitelist as pipe requests. Tests replace only the privilege/factory boundary.
func executeElevated(method string, params, out any, haveAdmin func() bool, create func() (dispatcher, error)) error {
	if !allowedMethods[method] || method == "MaintenanceReady" || browserGuardMethod(method) {
		return errors.New("helper method is not allowed")
	}
	if !haveAdmin() {
		return errors.New("此命令需要实际已提升的管理员令牌；未修改任何隐私设置或 VPN 保护")
	}
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	if len(p) > maxRequest {
		return errors.New("privacy helper parameters exceed limit")
	}
	d, err := create()
	if err != nil {
		return err
	}
	result, actionErr := d.call(method, p)
	if result != nil && out != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err = json.Unmarshal(b, out); err != nil {
			return err
		}
	}
	return actionErr
}

func serve(conn net.Conn, session string, d dispatcher, validPeer func() error) error {
	defer conn.Close()
	var lastID uint64
	for {
		if err := conn.SetReadDeadline(time.Now().Add(idleTimeout)); err != nil {
			return err
		}
		var req request
		if err := readFrame(conn, &req, maxRequest); err != nil {
			return err
		}
		if req.Version != protocolVersion || !sameSession(req.Session, session) || req.ID == 0 || req.ID <= lastID {
			return errors.New("unauthenticated or replayed helper request")
		}
		if err := validPeer(); err != nil {
			return err
		}
		lastID = req.ID
		result, err := d.call(req.Method, req.Params)
		res := response{Version: protocolVersion, ID: req.ID}
		if result != nil {
			res.Result, _ = json.Marshal(result)
		}
		if err != nil {
			res.Error = err.Error()
		}
		if err = conn.SetWriteDeadline(time.Now().Add(callTimeout)); err != nil {
			return err
		}
		if err = writeFrame(conn, res, maxResponse); err != nil {
			return err
		}
	}
}

type Client struct {
	heartbeat string
	conn      net.Conn
	session   string
	mu        sync.Mutex
	next      uint64
	done      chan struct{}
	once      sync.Once
	cleanup   func()
}

func newClient(conn net.Conn, session string, cleanup func()) *Client {
	return &Client{conn: conn, session: session, done: make(chan struct{}), cleanup: cleanup}
}

// Call serializes requests. A timeout after sending a write means the result is
// unknown; it closes this session and requires a fresh report, never auto-retry.
func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	if c == nil {
		return net.ErrClosed
	}
	if !allowedMethods[method] {
		return errors.New("helper method is not allowed")
	}
	if ctx == nil {
		return errors.New("context required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-c.done:
		return net.ErrClosed
	default:
	}
	p, err := json.Marshal(params)
	if err != nil {
		return err
	}
	c.next++
	deadline := time.Now().Add(callTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = c.conn.SetDeadline(deadline); err != nil {
		return err
	}
	cancelDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = c.conn.SetDeadline(time.Now()); close(cancelDone) })
	defer func() {
		if !stop() {
			<-cancelDone
		}
	}()
	req := request{Version: protocolVersion, Session: c.session, ID: c.next, Method: method, Params: p}
	if err = writeFrame(c.conn, req, maxRequest); err != nil {
		_ = c.Close()
		return fmt.Errorf("helper send failed; refresh to confirm any in-flight change: %w", err)
	}
	var res response
	if err = readFrame(c.conn, &res, maxResponse); err != nil {
		_ = c.Close()
		return fmt.Errorf("helper result is unknown; refresh before retrying changes: %w", err)
	}
	if res.Version != protocolVersion || res.ID != req.ID {
		_ = c.Close()
		return errors.New("helper response identity mismatch")
	}
	if out != nil && len(res.Result) > 0 {
		if err = json.Unmarshal(res.Result, out); err != nil {
			return fmt.Errorf("invalid helper result: %w", err)
		}
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.once.Do(func() {
		close(c.done)
		err = c.conn.Close()
		if c.cleanup != nil {
			c.cleanup()
		}
	})
	return err
}

// Closed reports transport lifetime only, without issuing a machine-policy or
// browser mutation. A caller must never infer successful cleanup from it.
func (c *Client) Closed() bool {
	if c == nil {
		return true
	}
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Client) watch(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = c.Close()
			return
		case <-c.done:
			return
		case <-ticker.C:
			callCtx, cancel := context.WithTimeout(ctx, callTimeout)
			method := c.heartbeat
			if method == "" {
				method = "Report"
			}
			err := c.Call(callCtx, method, nil, nil)
			cancel()
			if err != nil {
				_ = c.Close()
				return
			}
		}
	}
}
