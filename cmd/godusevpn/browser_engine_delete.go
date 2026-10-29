package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
)

// maintenance reserves the same slot as Begin before invoking filesystem I/O.
// It deliberately rejects any tracked owner, including an unpublished starting
// session or a closed process whose filter cleanup is still unknown. The mutex
// is released during I/O so Status and cancellation never wait on file hashing.
func (c *browserEngineController) deleteVersion(id, version string, remove func(context.Context, string, string) (browserengine.DeleteReceipt, error)) (browserengine.DeleteReceipt, error) {
	if remove == nil {
		return browserengine.DeleteReceipt{}, errBrowserEngineRefused
	}
	c.mu.Lock()
	if c.closed || c.current != nil || c.maintenance || c.ctx == nil || c.ctx.Err() != nil {
		c.mu.Unlock()
		return browserengine.DeleteReceipt{}, browserengine.ErrBusy
	}
	c.maintenance = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.maintenance = false; c.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Minute)
	defer cancel()
	return remove(ctx, id, version)
}

// BrowserEngineDelete accepts identity values only; no path or command comes
// from the frontend. The offline transport makes network fallback impossible.
func (a *App) BrowserEngineDelete(id, version string) (browserengine.DeleteReceipt, error) {
	c, err := a.browserEngineController()
	if err != nil {
		return browserengine.DeleteReceipt{}, err
	}
	result, err := c.deleteVersion(id, version, func(ctx context.Context, engineID, selectedVersion string) (browserengine.DeleteReceipt, error) {
		manager, err := browserengine.New(&http.Client{Transport: offlineEngineTransport{}})
		if err != nil {
			return browserengine.DeleteReceipt{}, browserengine.ErrStore
		}
		return manager.DeleteVersion(ctx, engineID, selectedVersion)
	})
	if err == nil {
		return result, nil
	}
	switch {
	case errors.Is(err, browserengine.ErrBusy):
		return result, errors.New("该内核或启动清理任务仍被占用，请完全关闭后再删除")
	case errors.Is(err, browserengine.ErrInput):
		return result, errors.New("内核或版本格式无效，未删除任何目录")
	case errors.Is(err, browserengine.ErrNotInstalled):
		return result, errors.New("该版本已不在本地缓存，请刷新版本清单")
	case errors.Is(err, browserengine.ErrDeletePartial):
		return result, errors.New("删除未能完整确认，剩余内容已保留供检查；没有扩大清理其他版本或浏览器数据")
	default:
		return result, errors.New("无法确认该版本目录和完整性，删除被拒绝；未清理浏览器配置或数据")
	}
}
