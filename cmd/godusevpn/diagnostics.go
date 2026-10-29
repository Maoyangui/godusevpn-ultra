package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Maoyangui/godusevpn/internal/ipc"
	"github.com/Maoyangui/godusevpn/internal/privacydata"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) PreviewDiag(options ipc.DiagnosticOptions) (ipc.DiagnosticPreview, error) {
	var out ipc.DiagnosticPreview
	err := a.call(ipc.MPreviewDiag, options, &out)
	return out, err
}

var chooseDiagnosticDestination = func(ctx context.Context, name string) (string, error) {
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: "保存已预览的诊断包（不覆盖已有文件）", DefaultFilename: name, Filters: []runtime.FileFilter{{DisplayName: "ZIP", Pattern: "*.zip"}}})
}
var writeDiagnosticArchive = privacydata.WriteNewLocal

func (a *App) ExportDiagPreview(previewID, expectedSHA256 string) (string, error) {
	if previewID == "" || len(previewID) > 128 || len(expectedSHA256) != 64 {
		return "", errors.New("请先预览诊断内容")
	}
	var archive ipc.DiagnosticArchive
	if err := a.call(ipc.MExportDiag, ipc.DiagnosticExportRequest{PreviewID: previewID}, &archive); err != nil {
		return "", err
	}
	return saveReviewedDiagnostic(a.ctx, expectedSHA256, archive, chooseDiagnosticDestination, writeDiagnosticArchive)
}

func saveReviewedDiagnostic(ctx context.Context, expectedSHA256 string, archive ipc.DiagnosticArchive, choose func(context.Context, string) (string, error), write func(string, []byte) error) (string, error) {
	if err := validateDiagnosticArchive(archive); err != nil {
		return "", err
	}
	if archive.SHA256 != expectedSHA256 {
		return "", errors.New("诊断内容与预览不一致，请重新预览")
	}
	path, err := choose(ctx, archive.Filename)
	if err != nil || path == "" {
		return "", err
	}
	if err := write(path, archive.Data); err != nil {
		return "", err
	}
	return path, nil
}

func validateDiagnosticArchive(a ipc.DiagnosticArchive) error {
	if len(a.Data) == 0 || len(a.Data) > ipc.DiagnosticMaxZIPBytes || !strings.HasPrefix(a.Filename, "godusevpn-ultra-diag-") || !strings.HasSuffix(a.Filename, ".zip") || filepath.Base(a.Filename) != a.Filename || strings.ContainsAny(a.Filename, "\\/:\x00\r\n\t") || len(a.Filename) > 100 {
		return errors.New("诊断返回内容或文件名无法确认")
	}
	h := sha256.Sum256(a.Data)
	if a.SHA256 != hex.EncodeToString(h[:]) {
		return errors.New("诊断内容校验失败，请重新预览")
	}
	return nil
}
