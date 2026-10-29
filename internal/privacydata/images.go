// Package privacydata provides explicit, local-only tools for files selected by
// the user. It never scans directories or uploads file contents or metadata.
package privacydata

import (
	"bufio"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
)

const maxFileBytes int64 = 256 << 20
const maxPixels int64 = 40_000_000

type Block struct {
	Type  string `json:"type"`
	Bytes int64  `json:"bytes"`
}
type Report struct {
	Format         string  `json:"format"`
	MetadataBlocks []Block `json:"metadataBlocks"`
	RemovedBlocks  []Block `json:"removedBlocks"`
	HasMetadata    bool    `json:"hasMetadata"`
	Bytes          int64   `json:"bytes"`
	Output         string  `json:"output,omitempty"`
	Warning        string  `json:"warning"`
}

// Inspect reports block types and sizes only. GPS values, comments, filenames
// embedded in metadata, and other actual metadata content are never returned.
func Inspect(path string) (Report, error) {
	f, err := openSource(path)
	if err != nil {
		return Report{}, err
	}
	defer f.Close()
	return inspectOpen(f)
}

// Clean creates a new copy. It never modifies the source, overwrites an existing
// destination, or keeps a partially written destination after a processing error.
func Clean(source, destination string) (Report, error) {
	return cleanUsing(source, destination, openDestination)
}

func cleanUsing(source, destination string, create func(string) (*os.File, func() error, error)) (report Report, err error) {
	if samePath(source, destination) {
		return Report{}, errors.New("输出必须是新的独立文件，不能与源文件相同")
	}
	in, err := openSource(source)
	if err != nil {
		return Report{}, err
	}
	defer in.Close()
	report, err = inspectOpen(in)
	if err != nil {
		return Report{}, err
	}
	release, err := pinValidatedPath(destination, true)
	if err != nil {
		return Report{}, err
	}
	defer release()
	out, discard, err := create(destination)
	if err != nil {
		return Report{}, err
	}
	complete := false
	defer func() {
		if !complete {
			if cleanupErr := discard(); cleanupErr != nil {
				err = errors.Join(err, errors.New("未完成输出无法清理，请检查选定输出路径"))
			}
		}
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = errors.New("输出文件关闭失败，请核对生成副本")
		}
	}()
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return Report{}, errors.New("源文件无法重新读取")
	}
	var blocks []Block
	buffered := bufio.NewWriterSize(out, 64<<10)
	blocks, err = process(in, buffered, report.Format, report.Bytes)
	if err != nil {
		return Report{}, err
	}
	if err = buffered.Flush(); err != nil {
		return Report{}, errors.New("输出副本写入失败")
	}
	if err = out.Sync(); err != nil {
		return Report{}, errors.New("输出副本无法持久保存")
	}
	report.RemovedBlocks = blocks
	report.Output = filepath.Clean(destination)
	complete = true
	return report, nil
}

func inspectOpen(f *os.File) (Report, error) {
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return Report{}, errors.New("只支持普通本地图片文件")
	}
	if info.Size() <= 0 || info.Size() > maxFileBytes {
		return Report{}, errors.New("文件为空或超过 256 MiB 本地处理上限")
	}
	var sig [8]byte
	n, err := io.ReadFull(f, sig[:])
	if err != nil && n < 2 {
		return Report{}, errors.New("图片签名不完整")
	}
	format := ""
	if sig[0] == 0xff && sig[1] == 0xd8 {
		format = "jpeg"
	} else if n == 8 && string(sig[:]) == "\x89PNG\r\n\x1a\n" {
		format = "png"
	}
	if format == "" {
		return Report{}, errors.New("仅支持 JPEG 和 PNG；不根据扩展名猜测格式")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return Report{}, errors.New("无法读取源文件")
	}
	blocks, err := process(f, io.Discard, format, info.Size())
	if err != nil {
		return Report{}, err
	}
	if err = validatePixels(f, format); err != nil {
		return Report{}, err
	}
	return Report{Format: format, MetadataBlocks: blocks, RemovedBlocks: []Block{}, HasMetadata: len(blocks) > 0, Bytes: info.Size(), Warning: "仅移除指定元数据块，不会匿名化画面。ICC 色彩配置、其他附加块、文件名和图像中的人脸/文字仍可能包含信息；不覆盖源文件。删除 EXIF 方向信息可能改变部分照片的显示方向。"}, nil
}

func validatePixels(f *os.File, format string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return errors.New("无法校验图片")
	}
	var w, h int
	if format == "jpeg" {
		c, err := jpeg.DecodeConfig(f)
		if err != nil {
			return errors.New("JPEG 图像头损坏或编码不受支持")
		}
		w, h = c.Width, c.Height
	} else {
		c, err := png.DecodeConfig(f)
		if err != nil {
			return errors.New("PNG 图像头损坏或编码不受支持")
		}
		w, h = c.Width, c.Height
	}
	if w <= 0 || h <= 0 || int64(w) > maxPixels/int64(h) {
		return errors.New("图片尺寸超出 4000 万像素验证上限")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return errors.New("无法校验图片")
	}
	// Decode only for bounded input validation; output is always copied from the
	// original encoded segments, never recompressed or generated from these pixels.
	if format == "jpeg" {
		if _, err := jpeg.Decode(f); err != nil {
			return errors.New("JPEG 像素数据损坏或编码不受支持")
		}
	} else {
		if _, err := png.Decode(f); err != nil {
			return errors.New("PNG 像素数据损坏或编码不受支持")
		}
	}
	return nil
}

func process(r io.Reader, w io.Writer, format string, size int64) ([]Block, error) {
	if format == "jpeg" {
		return processJPEG(r, w, size)
	}
	if format == "png" {
		return processPNG(r, w, size)
	}
	return nil, fmt.Errorf("不支持的图片格式")
}
