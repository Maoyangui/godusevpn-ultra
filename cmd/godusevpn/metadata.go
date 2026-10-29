package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Maoyangui/godusevpn/internal/metadatatool"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var errMetadataSession = errors.New("图片预览已失效，请重新选择；没有修改文件")
var errMetadataBusy = errors.New("另一个图片操作正在处理")

type metadataBackend interface {
	Backend() metadatatool.BackendInfo
	Preview(context.Context, string) (metadatatool.Preview, error)
	ReadValues(context.Context, string, string, []string) (map[string]json.RawMessage, error)
	CreateCopy(context.Context, metadatatool.CopyRequest) (metadatatool.CopyResult, error)
}
type metadataController struct {
	mu             sync.Mutex
	busy           bool
	tool           metadataBackend
	id, path, hash string
	expires        time.Time
	choose         func(context.Context) (string, error)
	save           func(context.Context, string) (string, error)
}
type MetadataStatusResult struct {
	Backend metadatatool.BackendInfo `json:"backend"`
	Fields  []metadatatool.Field     `json:"fields"`
}
type MetadataPreviewResult struct {
	PreviewID string               `json:"previewId"`
	Preview   metadatatool.Preview `json:"preview"`
	Fields    []metadatatool.Field `json:"fields"`
}

func (a *App) metadataController() *metadataController {
	a.privacyMu.Lock()
	defer a.privacyMu.Unlock()
	if a.metadata == nil {
		a.metadata = &metadataController{tool: metadatatool.New(), choose: chooseImage, save: func(ctx context.Context, name string) (string, error) {
			return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: "保存所选字段已清理的新副本", DefaultFilename: name})
		}}
	}
	return a.metadata
}
func (c *metadataController) begin() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy {
		return errMetadataBusy
	}
	c.busy = true
	return nil
}
func (c *metadataController) end() { c.mu.Lock(); c.busy = false; c.mu.Unlock() }
func metadataActive(ctx context.Context) error {
	if ctx == nil {
		return errMetadataSession
	}
	return ctx.Err()
}
func metadataFields(fields []string) error {
	allowed := map[string]bool{}
	for _, field := range metadatatool.Fields() {
		allowed[field.ID] = true
	}
	if len(fields) == 0 || len(fields) > len(allowed) {
		return metadatatool.ErrSelection
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if !allowed[field] || seen[field] {
			return metadatatool.ErrSelection
		}
		seen[field] = true
	}
	return nil
}
func (c *metadataController) session(id string) (string, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if id == "" || id != c.id || time.Now().After(c.expires) {
		c.id = ""
		c.path = ""
		c.hash = ""
		return "", "", errMetadataSession
	}
	return c.path, c.hash, nil
}

func (a *App) MetadataToolStatus() (MetadataStatusResult, error) {
	c := a.metadataController()
	if err := c.begin(); err != nil {
		return MetadataStatusResult{}, err
	}
	defer c.end()
	return MetadataStatusResult{Backend: c.tool.Backend(), Fields: metadatatool.Fields()}, nil
}

// The frontend receives an expiring opaque selection, never a freely editable
// source path. Only the native user file picker establishes that selection.
func (a *App) MetadataPreviewImage() (*MetadataPreviewResult, error) {
	return a.metadataController().preview(a.ctx)
}
func (c *metadataController) preview(ctx context.Context) (*MetadataPreviewResult, error) {
	if err := metadataActive(ctx); err != nil {
		return nil, err
	}
	if err := c.begin(); err != nil {
		return nil, err
	}
	defer c.end()
	c.mu.Lock()
	c.id = ""
	c.path = ""
	c.hash = ""
	c.mu.Unlock()
	path, err := c.choose(ctx)
	if err != nil || path == "" {
		return nil, err
	}
	if err := metadataActive(ctx); err != nil {
		return nil, err
	}
	preview, err := c.tool.Preview(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := metadataActive(ctx); err != nil {
		return nil, err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, errMetadataSession
	}
	id := hex.EncodeToString(token[:])
	c.mu.Lock()
	c.id = id
	c.path = path
	c.hash = preview.SourceSHA256
	c.expires = time.Now().Add(10 * time.Minute)
	c.mu.Unlock()
	return &MetadataPreviewResult{PreviewID: id, Preview: preview, Fields: metadatatool.Fields()}, nil
}
func (a *App) MetadataReadValues(id string, fields []string) (map[string]json.RawMessage, error) {
	return a.metadataController().values(a.ctx, id, fields)
}
func (c *metadataController) values(ctx context.Context, id string, fields []string) (map[string]json.RawMessage, error) {
	if err := metadataActive(ctx); err != nil {
		return nil, err
	}
	if err := metadataFields(fields); err != nil {
		return nil, err
	}
	if err := c.begin(); err != nil {
		return nil, err
	}
	defer c.end()
	path, hash, err := c.session(id)
	if err != nil {
		return nil, err
	}
	return c.tool.ReadValues(ctx, path, hash, fields)
}
func (a *App) MetadataCreateCopy(id string, fields []string) (*metadatatool.CopyResult, error) {
	return a.metadataController().copy(a.ctx, id, fields)
}
func (c *metadataController) copy(ctx context.Context, id string, fields []string) (*metadatatool.CopyResult, error) {
	if err := metadataActive(ctx); err != nil {
		return nil, err
	}
	if err := metadataFields(fields); err != nil {
		return nil, err
	}
	if err := c.begin(); err != nil {
		return nil, err
	}
	defer c.end()
	path, hash, err := c.session(id)
	if err != nil {
		return nil, err
	}
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext) + "-selected-clean" + ext
	destination, err := c.save(ctx, name)
	if err != nil || destination == "" {
		return nil, err
	}
	// A picker can complete after application cancellation. Never start a new
	// file operation merely because it eventually returned a destination.
	if err := metadataActive(ctx); err != nil {
		return nil, err
	}
	currentPath, currentHash, err := c.session(id)
	if err != nil || path != currentPath || hash != currentHash {
		return nil, errMetadataSession
	}
	result, err := c.tool.CreateCopy(ctx, metadatatool.CopyRequest{InputPath: path, OutputPath: destination, PreviewSHA256: hash, Remove: append([]string(nil), fields...)})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
