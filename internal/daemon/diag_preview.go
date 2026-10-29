package daemon

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Maoyangui/godusevpn/internal/buildinfo"
	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/paths"
	"github.com/Maoyangui/godusevpn/internal/settings"
	"github.com/Maoyangui/godusevpn/internal/state"
)

const diagnosticTTL = 5 * time.Minute

type cachedDiagnostic struct {
	preview ipc.DiagnosticPreview
	archive ipc.DiagnosticArchive
	expires time.Time
}

type diagnosticCollectors struct {
	logs, config, crashes, network func(func(string, string), func(string))
}

func diagnosticOptions(raw json.RawMessage) (ipc.DiagnosticOptions, error) {
	var options ipc.DiagnosticOptions
	if err := decodeDiagnosticRequest(raw, &options); err != nil {
		return options, err
	}
	if (options.Logs || options.Config || options.Crashes || options.Network) && !options.AcknowledgeSensitive {
		return options, errors.New("详细诊断可能包含地址、设备名称和访问记录；请先明确确认所选内容")
	}
	return options, nil
}

func decodeDiagnosticRequest(raw json.RawMessage, into any) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > 4096 {
		return errors.New("诊断请求过大")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		return errors.New("诊断选项无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("诊断选项包含额外内容")
	}
	return nil
}

// Only fixed enums and booleans enter the default report. Never call stateView:
// it contains settings, profile/node names, endpoints and free-form errors.
func minimalDiagnostic(s settings.Settings, snap state.Snapshot, settingsOK, persistedOK, guardOn, guardError bool) map[string]any {
	status := string(snap.Status)
	switch snap.Status {
	case state.Disconnected, state.Preparing, state.Starting, state.Connected, state.Degraded, state.Stopping, state.Failed:
	default:
		status = "unknown"
	}
	code := snap.Code
	switch code {
	case "", state.CodeProfileMissing, state.CodeProfileNet, state.CodeProfileURL, state.CodeProfileAuth, state.CodeProfileParse, state.CodeConfig, state.CodeTunDriver, state.CodeRouteConflict, state.CodeRuleSet, state.CodePortBusy, state.CodeCoreStart, state.CodeCoreCrash, state.CodeNodeDown, state.CodePrivacyGuard, state.CodePrivacyNIC, state.CodeUnknown:
	default:
		code = state.CodeUnknown
	}
	mode := s.Mode
	if mode != settings.ModeRule && mode != settings.ModeGlobal && mode != settings.ModeDirect {
		mode = "unknown"
	}
	return map[string]any{
		"schema": 1, "scope": "minimal_allowlist", "version": buildinfo.Version,
		"os":            runtime.GOOS + "/" + runtime.GOARCH,
		"connection":    map[string]any{"status": status, "code": code, "wanted": snap.Wanted},
		"configuration": map[string]any{"mode": mode, "tun": s.TUN, "strictRoute": s.StrictRoute, "noDirect": s.NoDirect, "lanBypass": s.LANBypass, "ipv6": s.IPv6, "disableNicIpv6": s.DisableNICIPv6, "fakeIp": s.FakeIP},
		"localEvidence": map[string]bool{"settingsReadable": settingsOK, "connectionIntentReadable": persistedOK, "guardRecorded": guardOn, "guardErrorRecorded": guardError},
		"note":          "仅为应用状态与设置摘要，不是防泄漏实测；未读取日志、原始状态、节点、配置文件或系统网络详情。",
	}
}

func (d *Daemon) minimalDiagnostic() map[string]any {
	d.mu.Lock()
	s := d.settings
	settingsOK, persistedOK, guardOn, guardError := d.settingsOK, d.persistedOK, d.guardOn, d.guardErr != ""
	d.mu.Unlock()
	return minimalDiagnostic(s, d.machine.Snapshot(), settingsOK, persistedOK, guardOn, guardError)
}

func (d *Daemon) previewDiag(options ipc.DiagnosticOptions) (ipc.DiagnosticPreview, error) {
	if (options.Logs || options.Config || options.Crashes || options.Network) && !options.AcknowledgeSensitive {
		return ipc.DiagnosticPreview{}, errors.New("详细诊断需要明确确认所选敏感内容")
	}
	// Serialize optional collection too: concurrent authorized requests cannot
	// multiply the bounded collectors/ZIP buffers outside the two-entry cache.
	d.diagMu.Lock()
	defer d.diagMu.Unlock()
	if d.diagClosed {
		return ipc.DiagnosticPreview{}, errors.New("服务正在退出；没有采集诊断")
	}
	entry, err := buildDiagnostic(options, d.minimalDiagnostic(), d.diagnosticCollectors(), time.Now())
	if err != nil {
		return ipc.DiagnosticPreview{}, err
	}
	var keep []cachedDiagnostic
	for _, old := range d.diagCache {
		if old.expires.After(time.Now()) {
			keep = append(keep, old)
		}
	}
	if len(keep) >= 2 {
		keep = keep[len(keep)-1:]
	}
	d.diagCache = append(keep, entry)
	d.scheduleDiagnosticExpiryLocked()
	return cloneDiagnosticPreview(entry.preview), nil
}

func (d *Daemon) scheduleDiagnosticExpiryLocked() {
	if d.diagTimer != nil || d.diagClosed || len(d.diagCache) == 0 {
		return
	}
	after := time.Until(d.diagCache[0].expires)
	if after < 0 {
		after = 0
	}
	d.diagTimer = time.AfterFunc(after, func() { d.expireDiagnosticCache(time.Now()) })
}

func (d *Daemon) expireDiagnosticCache(now time.Time) {
	d.diagMu.Lock()
	defer d.diagMu.Unlock()
	if d.diagTimer != nil {
		d.diagTimer.Stop()
		d.diagTimer = nil
	}
	var keep []cachedDiagnostic
	for _, entry := range d.diagCache {
		if entry.expires.After(now) {
			keep = append(keep, entry)
		}
	}
	d.diagCache = keep
	d.scheduleDiagnosticExpiryLocked()
}

func (d *Daemon) clearDiagnosticCache() {
	d.diagMu.Lock()
	defer d.diagMu.Unlock()
	d.diagClosed = true
	if d.diagTimer != nil {
		d.diagTimer.Stop()
		d.diagTimer = nil
	}
	d.diagCache = nil
}

func cloneDiagnosticPreview(p ipc.DiagnosticPreview) ipc.DiagnosticPreview {
	p.Files = append([]ipc.DiagnosticFile(nil), p.Files...)
	p.Warnings = append([]string(nil), p.Warnings...)
	return p
}

func (d *Daemon) exportPreview(id string, now time.Time) (ipc.DiagnosticArchive, error) {
	d.diagMu.Lock()
	defer d.diagMu.Unlock()
	for i, entry := range d.diagCache {
		if entry.preview.ID != id {
			continue
		}
		if !entry.expires.After(now) {
			d.diagCache = append(d.diagCache[:i], d.diagCache[i+1:]...)
			break
		}
		out := entry.archive
		out.Data = append([]byte(nil), out.Data...)
		return out, nil
	}
	return ipc.DiagnosticArchive{}, errors.New("诊断预览已过期或不可用；请重新选择并预览，不会自动重新采集")
}

// The old no-argument entry remains available, with minimal content only.
// Archives are not written into a machine-wide readable directory.
func (d *Daemon) exportDiag() (ipc.DiagnosticArchive, error) {
	p, err := d.previewDiag(ipc.DiagnosticOptions{})
	if err != nil {
		return ipc.DiagnosticArchive{}, err
	}
	return d.exportPreview(p.ID, time.Now())
}

func buildDiagnostic(options ipc.DiagnosticOptions, summary any, sources diagnosticCollectors, now time.Time) (cachedDiagnostic, error) {
	var entry cachedDiagnostic
	if (options.Logs || options.Config || options.Crashes || options.Network) && !options.AcknowledgeSensitive {
		return entry, errors.New("详细诊断需要明确确认")
	}
	entry.preview.Selection = options
	entry.preview.Files = []ipc.DiagnosticFile{}
	entry.preview.Warnings = []string{"最小摘要只含版本、固定状态码和功能开关；不代表已完成隐私或流量验证。"}
	total := 0
	add := func(name, content string) {
		content = redactText(content)
		if len(content) > ipc.DiagnosticMaxFileBytes {
			content = boundedDiagnosticText(content, ipc.DiagnosticMaxFileBytes)
			entry.preview.Warnings = append(entry.preview.Warnings, name+" 已截断；不是完整排障记录。")
		}
		if total+len(content) > ipc.DiagnosticMaxTextBytes {
			entry.preview.Warnings = append(entry.preview.Warnings, name+" 超出总大小限制，未加入。")
			return
		}
		total += len(content)
		entry.preview.Files = append(entry.preview.Files, ipc.DiagnosticFile{Name: name, Content: content, Bytes: len(content)})
	}
	warn := func(s string) { entry.preview.Warnings = append(entry.preview.Warnings, s) }
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return entry, errors.New("无法生成最小诊断摘要")
	}
	add("summary.json", string(b))
	for _, source := range []struct {
		selected bool
		collect  func(func(string, string), func(string))
	}{{options.Logs, sources.logs}, {options.Config, sources.config}, {options.Crashes, sources.crashes}, {options.Network, sources.network}} {
		if source.selected && source.collect != nil {
			source.collect(add, warn)
		}
	}
	if options.Logs || options.Config || options.Crashes || options.Network {
		warn("所选详情可能保留 IP、MAC、主机/节点名称、路径、网络关系和访问记录；常见凭据打码不等于匿名。请逐文件审阅后再分享。")
	}
	manifest, err := json.MarshalIndent(map[string]any{"selection": options, "warnings": entry.preview.Warnings}, "", "  ")
	if err != nil {
		return entry, errors.New("无法生成诊断范围说明")
	}
	add("scope.json", string(manifest))
	var data limitedDiagnosticBuffer
	zw := zip.NewWriter(&data)
	for _, file := range entry.preview.Files {
		w, err := zw.Create(file.Name)
		if err == nil {
			_, err = io.WriteString(w, file.Content)
		}
		if err != nil {
			_ = zw.Close()
			return entry, errors.New("诊断包写入失败或超出大小限制；未创建文件")
		}
	}
	if err := zw.Close(); err != nil {
		return entry, errors.New("诊断包超出大小限制或无法完成；未创建文件")
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return entry, errors.New("无法建立诊断预览标识")
	}
	entry.preview.ID = hex.EncodeToString(random[:])
	entry.expires = now.Add(diagnosticTTL)
	entry.preview.ExpiresAt = entry.expires.UTC().Format(time.RFC3339)
	sum := sha256.Sum256(data.Bytes())
	entry.preview.SHA256 = hex.EncodeToString(sum[:])
	entry.archive = ipc.DiagnosticArchive{Filename: "godusevpn-ultra-diag-" + now.UTC().Format("20060102-150405Z") + ".zip", Data: data.Bytes(), SHA256: entry.preview.SHA256}
	return entry, nil
}

type limitedDiagnosticBuffer struct{ bytes.Buffer }

func (b *limitedDiagnosticBuffer) Write(p []byte) (int, error) {
	if len(p) > ipc.DiagnosticMaxZIPBytes-b.Len() {
		return 0, errors.New("diagnostic archive limit")
	}
	return b.Buffer.Write(p)
}

func boundedDiagnosticText(s string, max int) string {
	const note = "\n[内容已截断]\n"
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= max {
		return s
	}
	n := max - len(note)
	for n > 0 && !utf8.ValidString(s[:n]) {
		n--
	}
	return s[:n] + note
}

func diagnosticFile(path string, tail bool, limit int) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", false, errors.New("not a regular diagnostic file")
	}
	truncated := info.Size() > int64(limit)
	if tail && truncated {
		if _, err := f.Seek(-int64(limit), io.SeekEnd); err != nil {
			return "", false, err
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	return strings.ToValidUTF8(string(b), "�"), truncated, err
}

func (d *Daemon) diagnosticCollectors() diagnosticCollectors {
	return diagnosticCollectors{
		logs: func(add func(string, string), warn func(string)) {
			for _, file := range []struct{ name, path string }{{"service.log", d.log.Path()}, {"core.log", d.coreLog.Path()}} {
				text, truncated, err := diagnosticFile(file.path, true, ipc.DiagnosticMaxFileBytes)
				if err != nil {
					warn(file.name + " 不可用，未加入。")
					continue
				}
				if truncated {
					warn(file.name + " 仅保留末尾片段。")
				}
				add(file.name, text)
			}
		},
		config: func(add func(string, string), warn func(string)) {
			text, truncated, err := diagnosticFile(paths.Config(), false, ipc.DiagnosticMaxFileBytes)
			var cfg any
			if err == nil && !truncated && json.Unmarshal([]byte(text), &cfg) == nil {
				b, _ := json.MarshalIndent(redact(cfg), "", "  ")
				add("config.redacted.json", string(b))
			} else {
				warn("配置文件不可用、过大或格式异常，未加入；没有输出原始文件。")
			}
			b, _ := json.MarshalIndent(redact(d.detailedDiagnosticState()), "", "  ")
			add("state.redacted.json", string(b))
			warn("详细应用状态保留有界错误与选择信息，不包含整份原始状态、全部订阅或设备列表。")
		},
		crashes: func(add func(string, string), warn func(string)) {
			for _, name := range []string{"crash.log", "crash.log.1"} {
				text, truncated, err := diagnosticFile(filepath.Join(paths.Logs(), name), true, 2<<20)
				if err != nil {
					warn(name + " 不可用，未加入。")
					continue
				}
				if truncated {
					warn(name + " 仅从末尾 2 MiB 提取，较早原因可能缺失。")
				}
				add(name, crashTextExcerpt(text))
			}
		},
		network: func(add func(string, string), warn func(string)) {
			warn("已主动选择本机网络详情；系统命令输出可能保留机器和局域网标识。")
			sysDiag(add)
		},
	}
}

func (d *Daemon) detailedDiagnosticState() map[string]any {
	snap := d.machine.Snapshot()
	d.mu.Lock()
	defer d.mu.Unlock()
	return map[string]any{
		"status": string(snap.Status), "code": boundedDiagnosticText(snap.Code, 256),
		"error": boundedDiagnosticText(snap.Error, 8192), "retries": snap.Retries,
		"selected":  boundedDiagnosticText(d.settings.Selected, 1024),
		"localDns":  boundedDiagnosticText(d.settings.LocalDNS, 2048),
		"remoteDns": boundedDiagnosticText(d.settings.RemoteDNS, 2048),
		"mixedPort": d.settings.MixedPort, "configuredClashPort": d.settings.ClashPort,
		"guardError": boundedDiagnosticText(d.guardErr, 8192),
	}
}

// Only the explicitly selected network collector invokes these local tools.
// Bound duration and output; never put the command's raw error/path in a report.
func cmdOut(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out diagnosticCommandBuffer
	cmd, err := diagnosticCommand(ctx, name, args...)
	if err != nil {
		return "[系统诊断工具不可用；未执行命令]"
	}
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	text := string(out.data)
	if out.truncated {
		text += "\n[系统命令输出已截断]\n"
	}
	if ctx.Err() != nil {
		text += "\n[系统命令超时；输出可能不完整]\n"
	} else if err != nil {
		text += "\n[系统命令未成功完成；输出可能不完整]\n"
	}
	return boundedDiagnosticText(text, ipc.DiagnosticMaxFileBytes)
}

type diagnosticCommandBuffer struct {
	data      []byte
	truncated bool
}

func (b *diagnosticCommandBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := ipc.DiagnosticMaxFileBytes - len(b.data)
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil
}
