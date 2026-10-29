// Package metadatatool adapts a pinned ExifTool distribution. It never scans a
// directory, uploads a file, overwrites an original, or accepts command options.
package metadatatool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
)

const (
	Version          = "13.59"
	SourceCommit     = "2200871d9cef988051d2a99d67df3bda6cbb30a8"
	ArchiveSHA256    = "577257dd22baebe77157d905792d6ed2c5916cd03aa627f40b2175db12110ac6"
	maxImage         = 32 << 20
	maxJSON          = 128 << 10
	operationTimeout = 20 * time.Second
)

var (
	ErrUnavailable  = errors.New("metadata_backend_unavailable_or_untrusted")
	ErrInput        = errors.New("metadata_input_refused")
	ErrSelection    = errors.New("metadata_selection_refused")
	ErrChanged      = errors.New("metadata_source_changed_since_preview")
	ErrBackend      = errors.New("metadata_backend_failed")
	ErrVerification = errors.New("metadata_copy_verification_failed")
	ErrDestination  = errors.New("metadata_new_copy_destination_refused")
	ErrOutputLimit  = errors.New("metadata_backend_output_limit")
)

type Field struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Tags  []string `json:"tags"`
}

// Selection is deliberately limited to these EXIF fields. XMP, IPTC, embedded
// thumbnails, visible text, file-system timestamps and other metadata may remain.
var fields = []Field{
	{"artist", "EXIF 作者", []string{"IFD0:Artist"}},
	{"copyright", "EXIF 版权", []string{"IFD0:Copyright"}},
	{"description", "EXIF 描述", []string{"IFD0:ImageDescription"}},
	{"software", "EXIF 软件", []string{"IFD0:Software"}},
	{"camera", "EXIF 相机、镜头与序列号", []string{"IFD0:Make", "IFD0:Model", "ExifIFD:LensMake", "ExifIFD:LensModel", "ExifIFD:SerialNumber", "ExifIFD:LensSerialNumber"}},
	{"capture-time", "EXIF 拍摄与修改时间", []string{"ExifIFD:DateTimeOriginal", "ExifIFD:CreateDate", "IFD0:ModifyDate", "ExifIFD:OffsetTime", "ExifIFD:OffsetTimeOriginal", "ExifIFD:OffsetTimeDigitized", "ExifIFD:SubSecTime", "ExifIFD:SubSecTimeOriginal", "ExifIFD:SubSecTimeDigitized"}},
	{"exif-gps", "EXIF GPS 整组（不含 XMP 位置）", []string{"GPS:all"}},
}

func Fields() []Field {
	result := make([]Field, len(fields))
	for i, field := range fields {
		result[i] = field
		result[i].Tags = append([]string(nil), field.Tags...)
	}
	return result
}

type BackendInfo struct {
	Available     bool   `json:"available"`
	Reason        string `json:"reason,omitempty"`
	Version       string `json:"version"`
	SourceCommit  string `json:"sourceCommit"`
	ArchiveSHA256 string `json:"archiveSha256"`
	VMValidated   bool   `json:"vmValidated"`
}
type Preview struct {
	SourceSHA256 string                `json:"sourceSha256"`
	Format       string                `json:"format"`
	Width        int                   `json:"width"`
	Height       int                   `json:"height"`
	Bytes        int                   `json:"bytes"`
	Tags         map[string]TagSummary `json:"tags"`
}

// A normal preview reveals presence and size only, including for GPS. Values
// require a separate explicit, hash-bound ReadValues request for selected fields.
type TagSummary struct {
	Field     string `json:"field"`
	JSONBytes int    `json:"jsonBytes"`
}

func summarize(tags map[string]json.RawMessage) map[string]TagSummary {
	result := make(map[string]TagSummary, len(tags))
	for tag, value := range tags {
		result[tag] = TagSummary{Field: fieldForTag(tag), JSONBytes: len(value)}
	}
	return result
}

type CopyRequest struct {
	InputPath     string   `json:"inputPath"`
	OutputPath    string   `json:"outputPath"`
	PreviewSHA256 string   `json:"previewSha256"`
	Remove        []string `json:"remove"`
}
type CopyResult struct {
	SourceSHA256 string   `json:"sourceSha256"`
	CopySHA256   string   `json:"copySha256"`
	Removed      []string `json:"removed"`
	Selected     []string `json:"selected"`
	Verified     Preview  `json:"verified"`
}

type backendLease struct {
	executable, directory string
	release               func()
}
type runner interface {
	run(context.Context, backendLease, []string, []byte, int) ([]byte, error)
}
type inputLease interface {
	read() ([]byte, error)
	unchanged(string) error
	close()
}
type fileStore interface {
	openInput(string) (inputLease, error)
	saveNew(string, string, []byte) error
}
type Tool struct {
	runner  runner
	files   fileStore
	acquire func() (backendLease, error)
}

// New exposes only the fixed installed backend. A portable binary, PATH lookup,
// arbitrary Perl interpreter, download at runtime, or caller runner is unsupported.
func New() *Tool {
	return &Tool{runner: nativeRunner{}, files: nativeFiles{}, acquire: acquireInstalled}
}
func (t *Tool) Backend() BackendInfo {
	result := BackendInfo{Version: Version, SourceCommit: SourceCommit, ArchiveSHA256: ArchiveSHA256}
	lease, err := t.acquire()
	if err != nil {
		result.Reason = ErrUnavailable.Error()
		return result
	}
	defer lease.release()
	result.Available = true
	// Static trust is distinct from successful execution in an isolated VM.
	return result
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func readArguments() []string {
	args := []string{"-config", "", "-j", "-G1", "-s", "-n", "-ExifToolVersion", "-FileType"}
	for _, field := range fields {
		for _, tag := range field.Tags {
			args = append(args, "-"+tag)
		}
	}
	return append(args, "-")
}
func selectedFields(ids []string) (map[string]bool, []string, error) {
	if len(ids) == 0 || len(ids) > len(fields) {
		return nil, nil, ErrSelection
	}
	selected := map[string]bool{}
	var ordered []string
	for _, id := range ids {
		if selected[id] {
			return nil, nil, ErrSelection
		}
		found := false
		for _, f := range fields {
			if id == f.ID {
				found = true
			}
		}
		if !found {
			return nil, nil, ErrSelection
		}
		selected[id] = true
	}
	for _, f := range fields {
		if selected[f.ID] {
			ordered = append(ordered, f.ID)
		}
	}
	return selected, ordered, nil
}
func writeArguments(selected map[string]bool) []string {
	args := []string{"-config", "", "-q"}
	for _, field := range fields {
		if selected[field.ID] {
			for _, tag := range field.Tags {
				args = append(args, "-"+tag+"=")
			}
		}
	}
	// Both input and output are anonymous pipes. No user path is interpreted by Perl.
	return append(args, "-o", "-", "-")
}
func fieldForTag(tag string) string {
	for _, field := range fields {
		for _, candidate := range field.Tags {
			if candidate == tag || candidate == "GPS:all" && strings.HasPrefix(tag, "GPS:") {
				return field.ID
			}
		}
	}
	return ""
}
func parseMetadata(data []byte, format string) (map[string]json.RawMessage, error) {
	if len(data) == 0 || len(data) > maxJSON {
		return nil, ErrBackend
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	// Decode tokens so duplicate keys cannot silently replace verification data.
	if token, err := dec.Token(); err != nil || token != json.Delim('[') {
		return nil, ErrBackend
	}
	if token, err := dec.Token(); err != nil || token != json.Delim('{') {
		return nil, ErrBackend
	}
	result := map[string]json.RawMessage{}
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return nil, ErrBackend
		}
		seen[key] = true
		var value json.RawMessage
		if dec.Decode(&value) != nil || len(value) > 8192 {
			return nil, ErrBackend
		}
		switch key {
		case "SourceFile":
			var name string
			if json.Unmarshal(value, &name) != nil || name != "-" {
				return nil, ErrBackend
			}
		case "ExifTool:ExifToolVersion":
			var version json.Number
			if json.Unmarshal(value, &version) != nil || version.String() != Version {
				return nil, ErrBackend
			}
		case "File:FileType":
			var typ string
			if json.Unmarshal(value, &typ) != nil || typ != format {
				return nil, ErrBackend
			}
		default:
			if fieldForTag(key) == "" {
				return nil, ErrBackend
			}
			result[key] = append(json.RawMessage(nil), value...)
		}
	}
	if token, err := dec.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrBackend
	}
	if dec.More() {
		return nil, ErrBackend
	}
	if token, err := dec.Token(); err != nil || token != json.Delim(']') {
		return nil, ErrBackend
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, ErrBackend
	}
	if !seen["SourceFile"] || !seen["ExifTool:ExifToolVersion"] || !seen["File:FileType"] {
		return nil, ErrBackend
	}
	return result, nil
}
func (t *Tool) metadata(ctx context.Context, backend backendLease, data []byte, format string) (map[string]json.RawMessage, error) {
	result, err := t.runner.run(ctx, backend, readArguments(), data, maxJSON)
	if err != nil {
		return nil, ErrBackend
	}
	return parseMetadata(result, format)
}
func (t *Tool) Preview(ctx context.Context, path string) (Preview, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	lease, err := t.files.openInput(path)
	if err != nil {
		return Preview{}, ErrInput
	}
	defer lease.close()
	data, err := lease.read()
	if err != nil {
		return Preview{}, ErrInput
	}
	info, err := inspectImage(data)
	if err != nil {
		return Preview{}, err
	}
	backend, err := t.acquire()
	if err != nil {
		return Preview{}, ErrUnavailable
	}
	defer backend.release()
	tags, err := t.metadata(ctx, backend, data, info.format)
	if err != nil {
		return Preview{}, err
	}
	hash := digest(data)
	if lease.unchanged(hash) != nil {
		return Preview{}, ErrChanged
	}
	return Preview{SourceSHA256: hash, Format: info.format, Width: info.width, Height: info.height, Bytes: len(data), Tags: summarize(tags)}, nil
}
func (t *Tool) ReadValues(ctx context.Context, path, expectedSHA256 string, reveal []string) (map[string]json.RawMessage, error) {
	selected, _, err := selectedFields(reveal)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	lease, err := t.files.openInput(path)
	if err != nil {
		return nil, ErrInput
	}
	defer lease.close()
	data, err := lease.read()
	if err != nil {
		return nil, ErrInput
	}
	hash := digest(data)
	if !strings.EqualFold(hash, expectedSHA256) {
		return nil, ErrChanged
	}
	info, err := inspectImage(data)
	if err != nil {
		return nil, err
	}
	backend, err := t.acquire()
	if err != nil {
		return nil, ErrUnavailable
	}
	defer backend.release()
	tags, err := t.metadata(ctx, backend, data, info.format)
	if err != nil {
		return nil, err
	}
	if lease.unchanged(hash) != nil {
		return nil, ErrChanged
	}
	result := map[string]json.RawMessage{}
	for tag, value := range tags {
		if selected[fieldForTag(tag)] {
			result[tag] = value
		}
	}
	return result, nil
}
func (t *Tool) CreateCopy(ctx context.Context, request CopyRequest) (CopyResult, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	selected, ordered, err := selectedFields(request.Remove)
	if err != nil {
		return CopyResult{}, err
	}
	decoded, err := hex.DecodeString(request.PreviewSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return CopyResult{}, ErrChanged
	}
	lease, err := t.files.openInput(request.InputPath)
	if err != nil {
		return CopyResult{}, ErrInput
	}
	defer lease.close()
	data, err := lease.read()
	if err != nil {
		return CopyResult{}, ErrInput
	}
	original := digest(data)
	if !strings.EqualFold(original, request.PreviewSHA256) {
		return CopyResult{}, ErrChanged
	}
	beforeImage, err := inspectImage(data)
	if err != nil {
		return CopyResult{}, err
	}
	backend, err := t.acquire()
	if err != nil {
		return CopyResult{}, ErrUnavailable
	}
	defer backend.release()
	before, err := t.metadata(ctx, backend, data, beforeImage.format)
	if err != nil {
		return CopyResult{}, err
	}
	copyData, err := t.runner.run(ctx, backend, writeArguments(selected), data, maxImage)
	if err != nil {
		return CopyResult{}, ErrBackend
	}
	afterImage, err := inspectImage(copyData)
	if err != nil || beforeImage != afterImage {
		return CopyResult{}, ErrVerification
	}
	if err := samePixels(data, copyData); err != nil {
		return CopyResult{}, ErrVerification
	}
	after, err := t.metadata(ctx, backend, copyData, afterImage.format)
	if err != nil {
		return CopyResult{}, ErrVerification
	}
	for key, value := range after {
		if selected[fieldForTag(key)] {
			return CopyResult{}, ErrVerification
		}
		if !bytes.Equal(bytes.TrimSpace(before[key]), bytes.TrimSpace(value)) {
			return CopyResult{}, ErrVerification
		}
	}
	for key := range before {
		if !selected[fieldForTag(key)] {
			if _, ok := after[key]; !ok {
				return CopyResult{}, ErrVerification
			}
		}
	}
	if lease.unchanged(original) != nil {
		return CopyResult{}, ErrChanged
	}
	if ctx.Err() != nil {
		return CopyResult{}, ErrBackend
	}
	if t.files.saveNew(request.InputPath, request.OutputPath, copyData) != nil {
		return CopyResult{}, ErrDestination
	}
	removed := make([]string, 0, len(ordered))
	for _, id := range ordered {
		for tag := range before {
			if fieldForTag(tag) == id {
				removed = append(removed, id)
				break
			}
		}
	}
	return CopyResult{SourceSHA256: original, CopySHA256: digest(copyData), Removed: removed, Selected: ordered, Verified: Preview{SourceSHA256: digest(copyData), Format: afterImage.format, Width: afterImage.width, Height: afterImage.height, Bytes: len(copyData), Tags: summarize(after)}}, nil
}

// StableTagNames supports an explicit preview without inventing absent tags.
func (p Preview) StableTagNames() []string {
	result := make([]string, 0, len(p.Tags))
	for k := range p.Tags {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
