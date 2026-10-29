package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Maoyangui/godusevpn/internal/browserengine"
	"github.com/Maoyangui/godusevpn/internal/browserenginehost"
)

type BrowserEngineVersionSummary struct {
	EngineID        string    `json:"engineId"`
	Version         string    `json:"version"`
	TreeVerified    bool      `json:"treeVerified"`
	VersionVerified bool      `json:"versionVerified"`
	CanLaunch       bool      `json:"canLaunch"`
	FileCount       int       `json:"fileCount"`
	ArchiveSHA256   string    `json:"archiveSha256"`
	DownloadedAt    time.Time `json:"downloadedAt"`
	Source          string    `json:"source"`
}

func engineSummary(result browserengine.Installed) BrowserEngineVersionSummary {
	// The cache layer deliberately reports CanLaunch=false because it cannot
	// certify the ordinary-user process/job, helper guard, and protocol
	// readback on its own. A supported Windows build can expose the launch
	// affordance once the archive is fully verified; browserenginehost.Start
	// still re-checks the lease, helper, tunnel and runtime readback before
	// allowing navigation.
	canLaunch := result.CanLaunch || (result.TreeVerified && result.VersionVerified && browserenginehost.NativeLaunchEnabled())
	return BrowserEngineVersionSummary{EngineID: result.Receipt.Release.EngineID, Version: result.Receipt.Release.Version, TreeVerified: result.TreeVerified, VersionVerified: result.VersionVerified, CanLaunch: canLaunch, FileCount: len(result.Receipt.Files), ArchiveSHA256: result.Receipt.ArchiveSHA256, DownloadedAt: result.Receipt.InstalledAt, Source: result.Receipt.Release.Source}
}

func (a *App) BrowserEngineCatalog(proxyJSON string) (browserengine.Catalog, error) {
	config, err := a.browserEngineDownloadRoute(proxyJSON)
	if err != nil {
		return browserengine.Catalog{}, err
	}
	manager, err := newBrowserEngineManager(config)
	if err != nil {
		return browserengine.Catalog{}, errors.New("内核管理需要 Windows 普通用户环境及明确代理")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	catalog, err := manager.RefreshCatalog(ctx)
	if err != nil {
		return browserengine.Catalog{}, errors.New("官方版本清单读取失败，请检查所选代理；没有改用直连")
	}
	return catalog, nil
}

type offlineEngineTransport struct{}

func (offlineEngineTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, browserengine.ErrTransport
}
func (a *App) BrowserEngineVersions() ([]BrowserEngineVersionSummary, error) {
	manager, err := browserengine.New(&http.Client{Transport: offlineEngineTransport{}})
	if err != nil {
		return nil, errors.New("本地内核清单需要 Windows 普通用户环境")
	}
	versions, err := manager.ListInstalled()
	if err != nil {
		return nil, errors.New("本地内核目录无法确认，请检查目录和文件完整性")
	}
	result := make([]BrowserEngineVersionSummary, len(versions))
	for i, version := range versions {
		result[i] = engineSummary(version)
	}
	return result, nil
}

func (a *App) BrowserEngineDownload(requestJSON string) (BrowserEngineVersionSummary, error) {
	if len(requestJSON) > 8192 {
		return BrowserEngineVersionSummary{}, browserengine.ErrInput
	}
	var request struct {
		EngineID  string `json:"engineId"`
		Version   string `json:"version"`
		ProxyJSON string `json:"proxyJSON"`
	}
	decoder := json.NewDecoder(strings.NewReader(requestJSON))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return BrowserEngineVersionSummary{}, browserengine.ErrInput
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return BrowserEngineVersionSummary{}, browserengine.ErrInput
	}
	config, err := a.browserEngineDownloadRoute(request.ProxyJSON)
	if err != nil {
		return BrowserEngineVersionSummary{}, err
	}
	manager, err := newBrowserEngineManager(config)
	if err != nil {
		return BrowserEngineVersionSummary{}, errors.New("内核管理需要 Windows 普通用户环境")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	result, err := manager.DownloadVersion(ctx, request.EngineID, request.Version)
	if errors.Is(err, browserengine.ErrExists) {
		return BrowserEngineVersionSummary{}, errors.New("该版本目录已存在，请先检查本地版本；不会覆盖已有内容")
	}
	if err != nil {
		return BrowserEngineVersionSummary{}, errors.New("内核下载或校验未完成；没有改用直连，未完成目录保留供检查")
	}
	return engineSummary(result), nil
}
