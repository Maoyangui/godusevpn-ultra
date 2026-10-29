package browserengine

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bodgit/sevenzip"
)

const (
	maxDownload int64 = 512 << 20
	maxExpanded int64 = 2 << 30
	maxMember   int64 = 512 << 20
	maxEntries        = 20000
	receiptName       = ".ultra-engine-receipt.json"
)

type Manager struct {
	directory       string
	cacheDirectory  string // set only for the fixed cache, never BrowserInstances
	sessionInstance bool   // unique session root: never add cache lock leaves here
	client          *http.Client
	mu              sync.Mutex
	acquire         func(string, bool) (func(), error)
	verifyVersion   func(string, string) error
	verifyRoot      func(*os.Root, string) error
	lockFile        func(string, string, bool) (func(), error)
	deleteTree      func(*os.Root, string, string, Receipt) (versionDeletion, error)
}

// New selects only the current ordinary user's fixed LocalAppData cache. No
// public API accepts an executable, install directory, command line or script.
// Construction performs no network or file writes. Explicit download calls do.
func New(client *http.Client) (*Manager, error) {
	client, err := cloneClient(client)
	if err != nil {
		return nil, err
	}
	dir, err := defaultDirectory()
	if err != nil {
		return nil, err
	}
	return &Manager{directory: dir, cacheDirectory: dir, client: client, acquire: acquireStore, verifyVersion: verifyBinaryVersion, verifyRoot: verifyRootIdentity, lockFile: lockStoreFile, deleteTree: openVersionDeletion}, nil
}

type TreeFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type Receipt struct {
	SchemaVersion   int        `json:"schemaVersion"`
	Release         Release    `json:"release"`
	URL             string     `json:"url"`
	ArchiveSHA256   string     `json:"archiveSha256"`
	ArchiveSHA512   string     `json:"archiveSha512"`
	PublisherSHA512 string     `json:"publisherSha512,omitempty"`
	ChecksumSource  string     `json:"checksumSource,omitempty"`
	MetadataSHA256  string     `json:"metadataSha256"`
	InstalledAt     time.Time  `json:"installedAt"`
	Executable      string     `json:"executable"`
	Directories     []string   `json:"directories"`
	Files           []TreeFile `json:"files"`
}
type Installed struct {
	Receipt         Receipt `json:"receipt"`
	TreeVerified    bool    `json:"treeVerified"`
	VersionVerified bool    `json:"versionVerified"`
	CanLaunch       bool    `json:"canLaunch"`
	Limit           string  `json:"limit"`
}
type LaunchPlan struct {
	EngineID   string   `json:"engineId"`
	Version    string   `json:"version"`
	Executable string   `json:"executable"`
	CanLaunch  bool     `json:"canLaunch"`
	Missing    []string `json:"missing"`
}

func validSelection(id, version string) bool {
	return id == Chrome && chromeVersion.MatchString(version) || id == Firefox && foxVersion.MatchString(version)
}
func (m *Manager) openStore(create bool) (*os.Root, func(), error) {
	if m.acquire == nil {
		return nil, nil, ErrStore
	}
	release, err := m.acquire(m.directory, create)
	if err != nil {
		return nil, nil, ErrStore
	}
	root, err := os.OpenRoot(m.directory)
	if err != nil {
		release()
		return nil, nil, ErrStore
	}
	if m.verifyRoot != nil && m.verifyRoot(root, m.directory) != nil {
		root.Close()
		release()
		return nil, nil, ErrStore
	}
	if m.sessionInstance {
		// The guard permits only the session's profile and engine directory.
		// This newly generated instance has exclusive-create semantics, not
		// cache sharing; a permanent cache lock would violate that exact tree.
		var once sync.Once
		return root, func() { once.Do(func() { root.Close(); release() }) }, nil
	}
	if m.lockFile == nil {
		root.Close()
		release()
		return nil, nil, ErrStore
	}
	unlock, err := m.lockFile(m.directory, storeLockName, true)
	if err != nil {
		root.Close()
		release()
		return nil, nil, err
	}
	var once sync.Once
	return root, func() { once.Do(func() { unlock(); root.Close(); release() }) }, nil
}
func mkdir(root *os.Root, name string) error {
	if err := root.Mkdir(name, 0700); err != nil && !os.IsExist(err) {
		return ErrStore
	}
	info, err := root.Lstat(name)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrStore
	}
	return nil
}
func safePath(name string) bool {
	if name == "" || !utf8.ValidString(name) || len(name) > 1024 || strings.ContainsAny(name, `\:*?"<>|`+"\x00") || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || path.Clean(name) != name || strings.Count(name, "/") > 32 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		for _, r := range part {
			if r < 32 || r > 126 {
				return false
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" {
			return false
		}
		if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
			rest := base[3:]
			if len([]rune(rest)) == 1 && strings.Contains("123456789¹²³", rest) {
				return false
			}
		}
	}
	return true
}
func hashValid(value string, size int) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == size && strings.ToLower(value) == value
}

// DownloadVersion resolves official metadata, verifies the Mozilla publisher
// checksum, and reads the archive without executing its SFX code. A version is
// never overwritten. The receipt is committed last; an interrupted/incomplete
// directory is refused by InspectInstalled and is not silently reused/deleted.
func (m *Manager) DownloadVersion(ctx context.Context, id, version string) (Installed, error) {
	if !validSelection(id, version) {
		return Installed{}, ErrInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	root, closeRoot, err := m.openStore(true)
	if err != nil {
		return Installed{}, err
	}
	defer closeRoot()
	if err := mkdir(root, id); err != nil {
		return Installed{}, err
	}
	versionDir := id + "/" + version
	if _, err := root.Lstat(versionDir); err == nil || !os.IsNotExist(err) {
		return Installed{}, ErrExists
	}
	release, err := m.resolve(ctx, id, version)
	if err != nil {
		return Installed{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Installed{}, ErrStore
	}
	temporary := ".download-" + hex.EncodeToString(random[:])
	archive, err := root.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Installed{}, ErrStore
	}
	defer func() { archive.Close(); root.Remove(temporary) }()
	sha256Hash, sha512Hash := sha256.New(), sha512.New()
	size, err := m.transfer(ctx, release.url, io.MultiWriter(archive, sha256Hash, sha512Hash), maxDownload)
	if err != nil || size == 0 {
		return Installed{}, ErrSource
	}
	sha256Value, sha512Value := hex.EncodeToString(sha256Hash.Sum(nil)), hex.EncodeToString(sha512Hash.Sum(nil))
	if release.checksum != "" && release.checksum != sha512Value {
		return Installed{}, ErrIntegrity
	}
	if archive.Sync() != nil {
		return Installed{}, ErrStore
	}
	entries, err := archiveEntries(ctx, archive, size, release)
	if err != nil {
		return Installed{}, err
	}
	if err := root.Mkdir(versionDir, 0700); err != nil {
		if os.IsExist(err) {
			return Installed{}, ErrExists
		}
		return Installed{}, ErrStore
	}
	destination, err := root.OpenRoot(versionDir)
	if err != nil {
		return Installed{}, ErrStore
	}
	defer destination.Close()
	files, dirs, err := extract(ctx, destination, entries)
	if err != nil {
		return Installed{}, err
	}
	if err := checkDistribution(destination, release); err != nil {
		return Installed{}, err
	}
	executable := filepath.Join(m.directory, filepath.FromSlash(versionDir), release.executable)
	if m.verifyVersion == nil || m.verifyVersion(executable, version) != nil {
		return Installed{}, ErrIntegrity
	}
	receipt := Receipt{SchemaVersion: 1, Release: release.Release, URL: release.url, ArchiveSHA256: sha256Value, ArchiveSHA512: sha512Value, PublisherSHA512: release.checksum, ChecksumSource: release.checksumSource, MetadataSHA256: release.metadataSHA256, InstalledAt: time.Now().UTC(), Executable: release.executable, Directories: dirs, Files: files}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return Installed{}, ErrStore
	}
	if err := writeExclusive(destination, receiptName, strings.NewReader(string(encoded)), int64(len(encoded)), nil); err != nil {
		return Installed{}, err
	}
	return installed(receipt), nil
}
func installed(receipt Receipt) Installed {
	return Installed{Receipt: receipt, TreeVerified: true, VersionVerified: true, CanLaunch: false, Limit: "Downloaded/version checked only. Ordinary-user process isolation, trusted immutable code lease, per-app no-direct enforcement, proxy/worker/configuration readback and failure-close host are not implemented by this cache."}
}

type archiveEntry struct {
	name      string
	size      int64
	directory bool
	open      func() (io.ReadCloser, error)
}
type contextualReaderAt struct {
	ctx    context.Context
	reader io.ReaderAt
}

func (r contextualReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.ReadAt(p, off)
}
func archiveEntries(ctx context.Context, reader io.ReaderAt, size int64, release resolved) ([]archiveEntry, error) {
	var entries []archiveEntry
	var totalDecoded uint64
	r := contextualReaderAt{ctx, reader}
	switch release.Format {
	case "zip":
		archive, err := zip.NewReader(r, size)
		if err != nil || len(archive.File) > maxEntries {
			return nil, ErrArchive
		}
		for _, file := range archive.File {
			file := file
			name := strings.TrimSuffix(file.Name, "/")
			mode := file.Mode()
			totalDecoded += file.UncompressedSize64
			if totalDecoded > uint64(maxExpanded) {
				return nil, ErrArchive
			}
			if !safePath(name) || mode.Type() != 0 && !mode.IsDir() || file.ExternalAttrs&0x400 != 0 || file.UncompressedSize64 > uint64(maxMember) || !strings.HasPrefix(file.Name, release.prefix) && file.Name != strings.TrimSuffix(release.prefix, "/") {
				return nil, ErrArchive
			}
			if name == strings.TrimSuffix(release.prefix, "/") {
				if !mode.IsDir() {
					return nil, ErrArchive
				}
				continue
			}
			name = strings.TrimPrefix(name, release.prefix)
			entries = append(entries, archiveEntry{name, int64(file.UncompressedSize64), mode.IsDir(), file.Open})
		}
	case "7z-sfx-core":
		archive, err := sevenzip.NewReader(r, size)
		if err != nil || len(archive.File) > maxEntries {
			return nil, ErrArchive
		}
		for _, file := range archive.File {
			file := file
			name := strings.TrimSuffix(file.Name, "/")
			mode := file.Mode()
			totalDecoded += file.UncompressedSize
			if totalDecoded > uint64(maxExpanded) {
				return nil, ErrArchive
			}
			if !safePath(name) || mode.Type() != 0 && !mode.IsDir() || file.Attributes&0x400 != 0 || file.UncompressedSize > uint64(maxMember) {
				return nil, ErrArchive
			}
			if name == strings.TrimSuffix(release.prefix, "/") {
				if !mode.IsDir() {
					return nil, ErrArchive
				}
				continue
			}
			if !strings.HasPrefix(name, release.prefix) {
				continue
			}
			name = strings.TrimPrefix(name, release.prefix)
			entries = append(entries, archiveEntry{name, int64(file.UncompressedSize), mode.IsDir(), file.Open})
		}
	default:
		return nil, ErrArchive
	}
	if len(entries) == 0 {
		return nil, ErrArchive
	}
	seen := map[string]bool{}
	kind := map[string]bool{}
	canonical := map[string]string{}
	total := int64(0)
	for _, entry := range entries {
		key := strings.ToLower(entry.name)
		if !safePath(entry.name) || key == receiptName || seen[key] || entry.directory && entry.size != 0 {
			return nil, ErrArchive
		}
		seen[key] = true
		kind[key] = entry.directory
		if !entry.directory {
			total += entry.size
			if total > maxExpanded {
				return nil, ErrArchive
			}
		}
	}
	// A file may never also act as another entry's parent; compare case-folded
	// Windows names before any output file is opened.
	for _, entry := range entries {
		for component := entry.name; component != "."; component = path.Dir(component) {
			key := strings.ToLower(component)
			if previous := canonical[key]; previous != "" && previous != component {
				return nil, ErrArchive
			}
			canonical[key] = component
			if component != entry.name && seen[key] && !kind[key] {
				return nil, ErrArchive
			}
		}
	}
	return entries, nil
}
func extract(ctx context.Context, root *os.Root, entries []archiveEntry) ([]TreeFile, []string, error) {
	directories := map[string]bool{}
	for _, entry := range entries {
		directory := path.Dir(entry.name)
		if entry.directory {
			directory = entry.name
		}
		for directory != "." {
			directories[directory] = true
			directory = path.Dir(directory)
		}
	}
	dirs := make([]string, 0, len(directories))
	for dir := range directories {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		if err := root.Mkdir(dir, 0700); err != nil {
			return nil, nil, ErrStore
		}
	}
	var files []TreeFile
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, nil, ErrArchive
		}
		if entry.directory {
			continue
		}
		reader, err := entry.open()
		if err != nil {
			return nil, nil, ErrArchive
		}
		hash := sha256.New()
		err = writeExclusive(root, entry.name, &contextReader{ctx, reader}, entry.size, hash)
		closeErr := reader.Close()
		if err != nil || closeErr != nil {
			return nil, nil, ErrArchive
		}
		files = append(files, TreeFile{entry.name, entry.size, hex.EncodeToString(hash.Sum(nil))})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, dirs, nil
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
func writeExclusive(root *os.Root, name string, reader io.Reader, expected int64, hash io.Writer) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrStore
	}
	defer file.Close()
	var out io.Writer = file
	if hash != nil {
		out = io.MultiWriter(file, hash)
	}
	n, err := io.Copy(out, io.LimitReader(reader, expected+1))
	if err != nil || n != expected || file.Sync() != nil {
		return ErrStore
	}
	return nil
}
func checkDistribution(root *os.Root, release resolved) error {
	required := []string{"chrome.exe", "chrome.dll", "icudtl.dat", "resources.pak"}
	if release.EngineID == Firefox {
		required = []string{"firefox.exe", "xul.dll", "omni.ja", "application.ini", "platform.ini"}
	}
	for _, name := range required {
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return ErrIntegrity
		}
	}
	executable, err := root.Open(release.executable)
	if err != nil {
		return ErrIntegrity
	}
	defer executable.Close()
	image, err := pe.NewFile(executable)
	if err != nil || image.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return ErrIntegrity
	}
	image.Close()
	if release.EngineID == Firefox {
		file, err := root.Open("application.ini")
		if err != nil {
			return ErrIntegrity
		}
		data, err := io.ReadAll(io.LimitReader(file, 128<<10))
		file.Close()
		if err != nil {
			return ErrIntegrity
		}
		inApp := false
		found := 0
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				inApp = line == "[App]"
			}
			if inApp && strings.HasPrefix(line, "Version=") {
				if strings.TrimPrefix(line, "Version=") != strings.TrimSuffix(release.Version, "esr") {
					return ErrIntegrity
				}
				found++
			}
		}
		if found != 1 {
			return ErrIntegrity
		}
	}
	return nil
}

// InspectInstalled reopens the fixed version and recomputes every file hash and
// exact tree membership. A writable receipt is not publisher authentication or
// a protected execution lease; CanLaunch therefore remains false.
func (m *Manager) InspectInstalled(id, version string) (Installed, error) {
	if !validSelection(id, version) {
		return Installed{}, ErrInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	root, closeRoot, err := m.openStore(false)
	if err != nil {
		return Installed{}, err
	}
	defer closeRoot()
	return m.inspectStore(root, id, version)
}

// The caller owns the store operation lease. Never recursively acquire it.
func (m *Manager) inspectStore(root *os.Root, id, version string) (Installed, error) {
	parent, err := root.Lstat(id)
	if err != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 {
		return Installed{}, ErrIntegrity
	}
	dir := id + "/" + version
	info, err := root.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Installed{}, ErrIntegrity
	}
	engine, err := root.OpenRoot(dir)
	if err != nil {
		return Installed{}, ErrIntegrity
	}
	defer engine.Close()
	file, err := engine.Open(receiptName)
	if err != nil {
		return Installed{}, ErrIntegrity
	}
	data, err := io.ReadAll(io.LimitReader(file, 8<<20))
	file.Close()
	if err != nil || len(data) >= 8<<20 {
		return Installed{}, ErrIntegrity
	}
	var receipt Receipt
	if strictJSON(data, &receipt) != nil || receipt.SchemaVersion != 1 || receipt.Release.EngineID != id || receipt.Release.Version != version || receipt.Release.Platform != Platform || len(receipt.Files) == 0 || len(receipt.Files) > maxEntries || len(receipt.Directories) > maxEntries || !hashValid(receipt.ArchiveSHA256, 32) || !hashValid(receipt.ArchiveSHA512, 64) || !hashValid(receipt.MetadataSHA256, 32) {
		return Installed{}, ErrIntegrity
	}
	release := firefoxRelease(version, "archive")
	if id == Chrome {
		release = resolved{Release: Release{EngineID: Chrome, Version: version, Format: "zip"}, url: "https://storage.googleapis.com/chrome-for-testing-public/" + version + "/win64/chrome-win64.zip", executable: "chrome.exe"}
	}
	if receipt.URL != release.url || receipt.Executable != release.executable || receipt.Release.Format != release.Format || id == Firefox && (!hashValid(receipt.PublisherSHA512, 64) || receipt.PublisherSHA512 != receipt.ArchiveSHA512 || receipt.ChecksumSource != release.checksumSource) || id == Chrome && (receipt.PublisherSHA512 != "" || receipt.ChecksumSource != "") {
		return Installed{}, ErrIntegrity
	}
	expectedFiles := map[string]TreeFile{}
	expectedDirs := map[string]bool{}
	for _, entry := range receipt.Files {
		key := strings.ToLower(entry.Name)
		if !safePath(entry.Name) || key == receiptName || !hashValid(entry.SHA256, 32) || entry.Size < 0 || entry.Size > maxMember || expectedFiles[key].Name != "" {
			return Installed{}, ErrIntegrity
		}
		expectedFiles[key] = entry
	}
	for _, name := range receipt.Directories {
		key := strings.ToLower(name)
		if !safePath(name) || expectedDirs[key] || expectedFiles[key].Name != "" {
			return Installed{}, ErrIntegrity
		}
		expectedDirs[key] = true
	}
	files, dirs := 0, 0
	var total int64
	err = fs.WalkDir(engine.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return ErrIntegrity
		}
		if name == "." {
			return nil
		}
		if name == receiptName {
			if !entry.Type().IsRegular() {
				return ErrIntegrity
			}
			return nil
		}
		key := strings.ToLower(name)
		if entry.IsDir() {
			if !expectedDirs[key] {
				return ErrIntegrity
			}
			dirs++
			return nil
		}
		expected, ok := expectedFiles[key]
		if !ok || name != expected.Name || !entry.Type().IsRegular() {
			return ErrIntegrity
		}
		source, err := engine.Open(name)
		if err != nil {
			return ErrIntegrity
		}
		defer source.Close()
		stat, err := source.Stat()
		if err != nil || stat.Size() != expected.Size {
			return ErrIntegrity
		}
		total += stat.Size()
		if total > maxExpanded {
			return ErrIntegrity
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(source, expected.Size+1))
		if err != nil || n != expected.Size || hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
			return ErrIntegrity
		}
		files++
		return nil
	})
	if err != nil || files != len(expectedFiles) || dirs != len(expectedDirs) || checkDistribution(engine, release) != nil || m.verifyVersion == nil || m.verifyVersion(filepath.Join(m.directory, filepath.FromSlash(dir), receipt.Executable), version) != nil {
		return Installed{}, ErrIntegrity
	}
	return installed(receipt), nil
}
func (m *Manager) ListInstalled() ([]Installed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	root, closeRoot, err := m.openStore(false)
	if err != nil {
		return nil, err
	}
	defer closeRoot()
	var selections [][2]string
	for _, id := range []string{Chrome, Firefox} {
		info, err := root.Lstat(id)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrIntegrity
		}
		directory, err := root.Open(id)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, ErrIntegrity
		}
		entries, err := directory.ReadDir(maxEntries + 1)
		directory.Close()
		if err != nil && err != io.EOF || len(entries) > maxEntries {
			return nil, ErrIntegrity
		}
		for _, entry := range entries {
			if !entry.IsDir() || !validSelection(id, entry.Name()) {
				return nil, ErrIntegrity
			}
			selections = append(selections, [2]string{id, entry.Name()})
		}
	}
	var result []Installed
	for _, selection := range selections {
		item, err := m.inspectStore(root, selection[0], selection[1])
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
func (m *Manager) Plan(id, version string) (LaunchPlan, error) {
	item, err := m.InspectInstalled(id, version)
	if err != nil {
		return LaunchPlan{}, err
	}
	return LaunchPlan{EngineID: id, Version: version, Executable: filepath.Join(m.directory, id, version, item.Receipt.Executable), CanLaunch: false, Missing: []string{"ordinary-user process/job lifecycle and trusted immutable code lease", "per-app no-direct enforcement independent of proxy flags", "pre-navigation profile/proxy/permission/worker/readback host", "Android/iOS/macOS/Windows web-environment emulation is not a native OS or Safari engine"}}, nil
}
