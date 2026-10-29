//go:build windows

package browserengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxDeletionNodes = 2*maxEntries + 1

type deletionInfo struct {
	directory, reparse bool
	links              uint32
	size               int64
}

// The fake implementation used by tests never calls a native filesystem API.
// A production implementation is always an already-open, pinned object.
type deletionHandle interface {
	io.Reader
	Info() (deletionInfo, error)
	Names(int) ([]string, error)
	Dispose() error
	Close() error
}

type deletionOpen func(deletionHandle, string, bool, bool) (deletionHandle, error)

type deletionNode struct {
	handle            deletionHandle
	name              string
	directory, closed bool
	size              int64
}

type handleVersionDeletion struct {
	mu              sync.Mutex
	held            []*deletionNode
	files, dirs     []*deletionNode
	started, closed bool
}

// openVersionDeletion never accepts an absolute child path. store is the
// caller's verified fixed cache root; the engine parent and every deletion
// object are opened relative to held handles without following reparse points.
func openVersionDeletion(store *os.Root, id, version string, receipt Receipt) (versionDeletion, error) {
	if store == nil || !validSelection(id, version) {
		return nil, ErrInput
	}
	file, err := store.Open(".")
	if err != nil {
		return nil, deletionNativeError(err)
	}
	anchor := &nativeDeletionHandle{file: file}
	return prepareVersionDeletion(anchor, id, version, receipt, openDeletionChild)
}

// Preparation is all-or-nothing and never sets a deletion disposition. The
// receipt only bounds what may be removed; every member is checked again from
// its own retained DELETE-capable handle before the first mutation.
func prepareVersionDeletion(anchor deletionHandle, id, version string, receipt Receipt, open deletionOpen) (_ versionDeletion, result error) {
	l := &handleVersionDeletion{}
	defer func() {
		if result != nil {
			l.Close()
		}
	}()
	if anchor == nil {
		return nil, ErrStore
	}
	anchorNode := &deletionNode{handle: anchor, directory: true, name: "cache-root"}
	l.held = append(l.held, anchorNode)
	if !validSelection(id, version) || open == nil || receipt.SchemaVersion != 1 || receipt.Release.EngineID != id || receipt.Release.Version != version || receipt.Release.Platform != Platform || len(receipt.Files) == 0 || len(receipt.Files) > maxEntries || len(receipt.Directories) > maxEntries || len(receipt.Files)+len(receipt.Directories)+1 > maxDeletionNodes {
		return nil, ErrIntegrity
	}
	if info, err := anchor.Info(); err != nil || !validDeletionInfo(info, true) {
		return nil, ErrStore
	}
	// Validate the entire requested membership before opening a child.
	directoryNames := map[string]bool{".": true}
	canonical := map[string]string{}
	files := make(map[string]TreeFile, len(receipt.Files))
	var total int64
	for _, name := range receipt.Directories {
		key := strings.ToLower(name)
		if !safePath(name) || canonical[key] != "" || name == receiptName {
			return nil, ErrIntegrity
		}
		canonical[key] = name
		directoryNames[name] = true
	}
	for _, file := range receipt.Files {
		key := strings.ToLower(file.Name)
		if !safePath(file.Name) || canonical[key] != "" || strings.EqualFold(file.Name, receiptName) || file.Size < 0 || file.Size > maxMember || total > maxExpanded-file.Size || !hashValid(file.SHA256, 32) {
			return nil, ErrIntegrity
		}
		canonical[key] = file.Name
		files[file.Name] = file
		total += file.Size
	}
	for name := range directoryNames {
		if name != "." && !directoryNames[path.Dir(name)] {
			return nil, ErrIntegrity
		}
	}
	for name := range files {
		if !directoryNames[path.Dir(name)] {
			return nil, ErrIntegrity
		}
	}
	add := func(parent *deletionNode, name, relative string, directory, remove bool) (*deletionNode, error) {
		h, err := open(parent.handle, name, directory, remove)
		if err != nil {
			return nil, deletionNativeError(err)
		}
		if h == nil {
			return nil, ErrStore
		}
		n := &deletionNode{handle: h, name: relative, directory: directory}
		l.held = append(l.held, n)
		info, err := h.Info()
		if err != nil || !validDeletionInfo(info, directory) {
			return nil, ErrIntegrity
		}
		n.size = info.size
		return n, nil
	}
	parent, err := add(anchorNode, id, "engine-parent", true, false)
	if err != nil {
		return nil, err
	}
	target, err := add(parent, version, ".", true, true)
	if err != nil {
		return nil, err
	}
	directories := map[string]*deletionNode{".": target}
	orderedDirs := append([]string(nil), receipt.Directories...)
	sort.Slice(orderedDirs, func(i, j int) bool {
		left, right := strings.Count(orderedDirs[i], "/"), strings.Count(orderedDirs[j], "/")
		return left < right || left == right && orderedDirs[i] < orderedDirs[j]
	})
	for _, name := range orderedDirs {
		n, err := add(directories[path.Dir(name)], path.Base(name), name, true, true)
		if err != nil {
			return nil, err
		}
		directories[name] = n
		l.dirs = append(l.dirs, n)
	}
	// The persisted receipt must still denote precisely the in-memory receipt
	// that the caller inspected, including its release and full member hashes.
	receiptNode, err := add(target, receiptName, receiptName, false, true)
	if err != nil {
		return nil, err
	}
	if receiptNode.size < 1 || receiptNode.size >= 8<<20 {
		return nil, ErrIntegrity
	}
	data, err := io.ReadAll(io.LimitReader(receiptNode.handle, (8<<20)+1))
	var onDisk Receipt
	if err != nil || int64(len(data)) != receiptNode.size || strictJSON(data, &onDisk) != nil {
		return nil, ErrIntegrity
	}
	wanted, err := json.Marshal(receipt)
	if err != nil {
		return nil, ErrIntegrity
	}
	actual, err := canonicalDeletionJSON(data)
	if err != nil {
		return nil, ErrIntegrity
	}
	wanted, err = canonicalDeletionJSON(wanted)
	if err != nil || !bytes.Equal(wanted, actual) {
		return nil, ErrIntegrity
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		want := files[name]
		n, err := add(directories[path.Dir(name)], path.Base(name), name, false, true)
		if err != nil {
			return nil, err
		}
		if n.size != want.Size {
			return nil, ErrIntegrity
		}
		hash := sha256.New()
		count, err := io.Copy(hash, io.LimitReader(n.handle, want.Size+1))
		if err != nil || count != want.Size || hex.EncodeToString(hash.Sum(nil)) != want.SHA256 {
			return nil, ErrIntegrity
		}
		l.files = append(l.files, n)
	}
	// Read each directory through its retained handle after every member is
	// pinned. A duplicate, extra, differently cased or missing entry refuses
	// preparation before any file is marked for deletion.
	expected := make(map[string]map[string]bool, len(directories))
	for name := range directories {
		expected[name] = map[string]bool{}
	}
	for name := range directories {
		if name != "." {
			expected[path.Dir(name)][path.Base(name)] = true
		}
	}
	for name := range files {
		expected[path.Dir(name)][path.Base(name)] = true
	}
	expected["."][receiptName] = true
	for name, directory := range directories {
		want := expected[name]
		got, err := directory.handle.Names(len(want) + 1)
		if err != nil && !errors.Is(err, io.EOF) || len(got) != len(want) {
			return nil, ErrIntegrity
		}
		seen := map[string]bool{}
		for _, member := range got {
			if !want[member] || seen[member] {
				return nil, ErrIntegrity
			}
			seen[member] = true
		}
	}
	// Receipt is deleted last among files, retaining recovery context on an
	// early failure. It is included in completed-file/byte counts.
	l.files = append(l.files, receiptNode)
	for i, j := 0, len(l.dirs)-1; i < j; i, j = i+1, j-1 {
		l.dirs[i], l.dirs[j] = l.dirs[j], l.dirs[i]
	}
	l.dirs = append(l.dirs, target)
	return l, nil
}

func validDeletionInfo(info deletionInfo, directory bool) bool {
	return info.directory == directory && !info.reparse && (directory || info.links == 1) && info.size >= 0
}

func canonicalDeletionJSON(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, ErrIntegrity
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrIntegrity
	}
	// Compare the complete JSON value, not just decoded known struct fields.
	// This also refuses ignored unknown fields and case-alias key spellings.
	return json.Marshal(value)
}

// Counts describe successful disposition+Close actions, not an independent
// proof of unlinking. The caller must still confirm the fixed version is absent
// while holding its exclusive store lease before reporting overall success.
func (l *handleVersionDeletion) Remove(ctx context.Context) (int, int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.started || ctx == nil {
		return 0, 0, ErrDeletePartial
	}
	l.started = true
	files, size := 0, int64(0)
	for _, group := range [][]*deletionNode{l.files, l.dirs} {
		for _, node := range group {
			if ctx.Err() != nil || node.closed || node.handle.Dispose() != nil {
				return files, size, ErrDeletePartial
			}
			node.closed = true
			if node.handle.Close() != nil {
				return files, size, ErrDeletePartial
			}
			if !node.directory {
				files++
				size += node.size
			}
		}
	}
	return files, size, nil
}

func (l *handleVersionDeletion) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.closed = true
	for i := len(l.held) - 1; i >= 0; i-- {
		if node := l.held[i]; !node.closed {
			node.closed = true
			_ = node.handle.Close()
		}
	}
}

type deletionOpenPolicy struct{ access, share, disposition, options, attributes uint32 }

func deletionPolicy(directory, remove bool) deletionOpenPolicy {
	p := deletionOpenPolicy{access: windows.FILE_GENERIC_READ | windows.READ_CONTROL | windows.SYNCHRONIZE, share: windows.FILE_SHARE_READ, disposition: windows.FILE_OPEN, options: windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT, attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	if directory {
		p.options |= windows.FILE_DIRECTORY_FILE
	} else {
		p.options |= windows.FILE_NON_DIRECTORY_FILE
	}
	if remove {
		p.access |= windows.DELETE
	}
	return p
}

type nativeDeletionHandle struct{ file *os.File }

func (h *nativeDeletionHandle) Read(p []byte) (int, error) { return h.file.Read(p) }
func (h *nativeDeletionHandle) Close() error               { return h.file.Close() }
func (h *nativeDeletionHandle) Names(limit int) ([]string, error) {
	return h.file.Readdirnames(limit)
}
func (h *nativeDeletionHandle) Dispose() error {
	flag := byte(1)
	return windows.SetFileInformationByHandle(windows.Handle(h.file.Fd()), windows.FileDispositionInfo, &flag, 1)
}
func (h *nativeDeletionHandle) Info() (deletionInfo, error) {
	handle := windows.Handle(h.file.Fd())
	kind, err := windows.GetFileType(handle)
	if err != nil || kind != windows.FILE_TYPE_DISK {
		return deletionInfo{}, ErrStore
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil {
		return deletionInfo{}, ErrStore
	}
	size := uint64(info.FileSizeHigh)<<32 | uint64(info.FileSizeLow)
	if size > uint64(^uint64(0)>>1) {
		return deletionInfo{}, ErrIntegrity
	}
	return deletionInfo{directory: info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0, reparse: info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, links: info.NumberOfLinks, size: int64(size)}, nil
}

func openDeletionChild(parent deletionHandle, name string, directory, remove bool) (deletionHandle, error) {
	anchor, ok := parent.(*nativeDeletionHandle)
	if !ok || anchor == nil || anchor.file == nil || !safePath(name) || strings.Contains(name, "/") {
		return nil, ErrInput
	}
	relative, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, ErrInput
	}
	p := deletionPolicy(directory, remove)
	oa := windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: windows.Handle(anchor.file.Fd()), ObjectName: relative, Attributes: p.attributes}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, p.access, &oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, p.share, p.disposition, p.options, 0, 0)
	if !validDeletionOpenResult(handle, status, err) {
		if handle != 0 && handle != windows.InvalidHandle {
			_ = windows.CloseHandle(handle)
		}
		if err != nil {
			return nil, deletionNativeError(err)
		}
		return nil, ErrStore
	}
	return &nativeDeletionHandle{file: os.NewFile(uintptr(handle), "ultra-engine-version-deletion")}, nil
}

func validDeletionOpenResult(handle windows.Handle, status windows.IO_STATUS_BLOCK, err error) bool {
	// FILE_OPEN must return FILE_OPENED (1), never create/overwrite/supersede.
	return err == nil && handle != 0 && handle != windows.InvalidHandle && status.Status == 0 && status.Information == 1
}

func deletionNativeError(err error) error {
	if errors.Is(err, ErrBusy) || errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.STATUS_SHARING_VIOLATION) || errors.Is(err, windows.STATUS_DELETE_PENDING) {
		return ErrBusy
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, windows.STATUS_OBJECT_NAME_NOT_FOUND) || errors.Is(err, windows.STATUS_OBJECT_PATH_NOT_FOUND) {
		return os.ErrNotExist
	}
	return ErrStore
}
