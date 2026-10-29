package browserengine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"
)

type fakeHTTP func(*http.Request) (*http.Response, error)

func (f fakeHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(data []byte) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}
}
func fakeManager(t *testing.T, route fakeHTTP) *Manager {
	t.Helper()
	client, err := cloneClient(&http.Client{Transport: route})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	return &Manager{directory: directory, cacheDirectory: directory, client: client, lockFile: fakeStoreLock, acquire: func(dir string, create bool) (func(), error) {
		if create {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return nil, err
			}
		}
		return func() {}, nil
	}, verifyVersion: func(string, string) error { return nil }}
}

var fakeLocks = struct {
	sync.Mutex
	entries map[string]*fakeLockState
}{entries: map[string]*fakeLockState{}}

type fakeLockState struct {
	readers int
	writer  bool
}

func fakeStoreLock(directory, name string, exclusive bool) (func(), error) {
	key := filepath.Join(directory, name)
	fakeLocks.Lock()
	state := fakeLocks.entries[key]
	if state == nil {
		state = &fakeLockState{}
		fakeLocks.entries[key] = state
	}
	if state.writer || exclusive && state.readers != 0 {
		fakeLocks.Unlock()
		return nil, ErrBusy
	}
	if exclusive {
		state.writer = true
	} else {
		state.readers++
	}
	fakeLocks.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			fakeLocks.Lock()
			defer fakeLocks.Unlock()
			if exclusive {
				state.writer = false
			} else {
				state.readers--
			}
			if !state.writer && state.readers == 0 {
				delete(fakeLocks.entries, key)
			}
		})
	}, nil
}
func fakePE() []byte {
	data := make([]byte, 96)
	copy(data, "MZ")
	binary.LittleEndian.PutUint32(data[0x3c:], 64)
	copy(data[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(data[68:], 0x8664)
	return data
}
func zipFixture(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, data := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Write(data)
	}
	if writer.Close() != nil {
		t.Fatal("zip")
	}
	return output.Bytes()
}
func chromeFixture(t *testing.T) []byte {
	return zipFixture(t, map[string][]byte{"chrome-win64/chrome.exe": fakePE(), "chrome-win64/chrome.dll": []byte("dll"), "chrome-win64/icudtl.dat": []byte("icu"), "chrome-win64/resources.pak": []byte("pak"), "chrome-win64/locales/en-US.pak": []byte("locale"), "chrome-win64/LICENSE": []byte("upstream notices retained")})
}
func chromeMetadata(version string) []byte {
	return []byte(fmt.Sprintf(`{"version":%q,"downloads":{"chrome":[{"platform":"win64","url":%q}]}}`, version, "https://storage.googleapis.com/chrome-for-testing-public/"+version+"/win64/chrome-win64.zip"))
}
func chromeRoutes(t *testing.T, archive []byte) fakeHTTP {
	return func(r *http.Request) (*http.Response, error) {
		t.Helper()
		switch r.URL.String() {
		case "https://googlechromelabs.github.io/chrome-for-testing/156.0.8078.47.json":
			return response(chromeMetadata("156.0.8078.47")), nil
		case "https://storage.googleapis.com/chrome-for-testing-public/156.0.8078.47/win64/chrome-win64.zip":
			return response(archive), nil
		default:
			t.Fatalf("unexpected network request %s", r.URL)
			return nil, ErrSource
		}
	}
}

func TestChromeDownloadReadbackAndNoExecution(t *testing.T) {
	m := fakeManager(t, chromeRoutes(t, chromeFixture(t)))
	item, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47")
	if err != nil {
		t.Fatal(err)
	}
	if !item.TreeVerified || !item.VersionVerified || item.CanLaunch || len(item.Receipt.Files) != 6 {
		t.Fatalf("%+v", item)
	}
	again, err := m.InspectInstalled(Chrome, "156.0.8078.47")
	if err != nil || again.CanLaunch {
		t.Fatalf("%+v %v", again, err)
	}
	plan, err := m.Plan(Chrome, "156.0.8078.47")
	if err != nil || plan.CanLaunch || len(plan.Missing) == 0 {
		t.Fatal(plan, err)
	}
	list, err := m.ListInstalled()
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	if _, err = m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrExists {
		t.Fatal(err)
	}
}
func TestReadbackRejectsChangedExtraAndMissingFiles(t *testing.T) {
	for _, mutation := range []string{"modified", "extra", "missing", "receipt", "version"} {
		t.Run(mutation, func(t *testing.T) {
			m := fakeManager(t, chromeRoutes(t, chromeFixture(t)))
			_, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47")
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(m.directory, Chrome, "156.0.8078.47")
			switch mutation {
			case "modified":
				err = os.WriteFile(filepath.Join(dir, "chrome.dll"), []byte("tampered"), 0600)
			case "extra":
				err = os.WriteFile(filepath.Join(dir, "extra.dll"), []byte("new"), 0600)
			case "missing":
				err = os.Remove(filepath.Join(dir, "icudtl.dat"))
			case "receipt":
				err = os.WriteFile(filepath.Join(dir, receiptName), []byte(`{"schemaVersion":1,"schemaVersion":1}`), 0600)
			case "version":
				m.verifyVersion = func(string, string) error { return ErrIntegrity }
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.InspectInstalled(Chrome, "156.0.8078.47"); err == nil {
				t.Fatal("accepted mutation")
			}
		})
	}
}
func TestArchiveNamesRejectedBeforeCreatingVersion(t *testing.T) {
	for _, name := range []string{"../outside", "chrome-win64/../../outside", "chrome-win64/C:stream", "chrome-win64/a:stream", "chrome-win64/NUL.exe", "chrome-win64/folder./x", "chrome-win64/COM¹.txt", "chrome-win64/A /x", "chrome-win64/\\remote", "chrome-win64/nonascii-中"} {
		t.Run(name, func(t *testing.T) {
			m := fakeManager(t, chromeRoutes(t, zipFixture(t, map[string][]byte{name: []byte("danger")})))
			if _, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47"); err == nil {
				t.Fatal("accepted unsafe archive")
			}
			if _, err := os.Stat(filepath.Join(m.directory, Chrome, "156.0.8078.47")); !os.IsNotExist(err) {
				t.Fatal("created version before preflight")
			}
		})
	}
}
func TestArchiveCaseCollisionsSymlinkAndBomb(t *testing.T) {
	tests := []struct {
		name    string
		entries []archiveEntry
	}{{"duplicate", []archiveEntry{{name: "A"}, {name: "a"}}}, {"parent-file", []archiveEntry{{name: "a"}, {name: "a/b"}}}, {"parent-case", []archiveEntry{{name: "A/a"}, {name: "a/b"}}}, {"receipt", []archiveEntry{{name: receiptName}}}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { // Exercise the same ZIP preflight through controlled headers.
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			for _, entry := range test.entries {
				header := &zip.FileHeader{Name: "chrome-win64/" + entry.name, Method: zip.Store}
				if entry.size > maxMember {
					header.UncompressedSize64 = uint64(entry.size)
					header.CompressedSize64 = 0
				}
				file, _ := writer.CreateHeader(header)
				file.Write([]byte("x"))
			}
			writer.Close()
			release, _ := chromeRelease(chromeDataFrom(t, chromeMetadata("156.0.8078.47")))
			if _, err := archiveEntries(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), release); err == nil {
				t.Fatal("unsafe preflight accepted")
			}
		})
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: "chrome-win64/link"}
	header.SetMode(os.ModeSymlink | 0777)
	out, _ := writer.CreateHeader(header)
	out.Write([]byte("../outside"))
	writer.Close()
	release, _ := chromeRelease(chromeDataFrom(t, chromeMetadata("156.0.8078.47")))
	if _, err := archiveEntries(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), release); err == nil {
		t.Fatal("symlink accepted")
	}
}
func chromeDataFrom(t *testing.T, data []byte) chromeData {
	t.Helper()
	var value chromeData
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestTransportAndOfficialOriginBoundaries(t *testing.T) {
	if _, err := cloneClient(nil); err != ErrTransport {
		t.Fatal(err)
	}
	if _, err := cloneClient(&http.Client{}); err != ErrTransport {
		t.Fatal(err)
	}
	fixtureUserURL := (&url.URL{Scheme: "https", Host: "archive.mozilla.org", User: url.User("fixture-user"), Path: "/pub/firefox/releases/"}).String()
	for _, raw := range []string{"http://archive.mozilla.org/pub/firefox/releases/", "https://archive.mozilla.org.evil.test/pub/firefox/releases/", fixtureUserURL, "https://storage.googleapis.com/other/x", "https://archive.mozilla.org:443/pub/firefox/releases/", "https://archive.mozilla.org/pub/firefox/releases/?secret=x"} {
		if allowedURL(raw) {
			t.Fatal(raw)
		}
	}
	calls := 0
	m := fakeManager(t, func(r *http.Request) (*http.Response, error) {
		calls++
		res := response(nil)
		res.StatusCode = 302
		res.Header.Set("Location", "https://example.invalid/redirect")
		return res, nil
	})
	if _, err := m.fetch(context.Background(), mozillaVersions, 1000); err == nil || calls != 1 {
		t.Fatal(err, calls)
	}
	for _, change := range []string{"length", "encoding", "oversize", "cancel"} {
		t.Run(change, func(t *testing.T) {
			m := fakeManager(t, func(r *http.Request) (*http.Response, error) {
				res := response([]byte("abc"))
				if change == "length" {
					res.ContentLength = 4
				}
				if change == "encoding" {
					res.Header.Set("Content-Encoding", "gzip")
				}
				return res, nil
			})
			ctx := context.Background()
			limit := int64(10)
			if change == "oversize" {
				limit = 2
			}
			if change == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := m.fetch(ctx, mozillaVersions, limit); err == nil {
				t.Fatal("unsafe response")
			}
		})
	}
}

// A minimal uncompressed 7z writer is a pure fixture, never a production
// extractor. The stub prefix tests SFX scanning without executing any PE code.
func sevenFixture(entries map[string][]byte) []byte {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sortStrings(names)
	var packed, header bytes.Buffer
	number := func(b *bytes.Buffer, n uint64) { b.WriteByte(0xff); binary.Write(b, binary.LittleEndian, n) }
	for _, name := range names {
		packed.Write(entries[name])
	}
	header.WriteByte(1)
	header.WriteByte(4)
	header.WriteByte(6)
	number(&header, 0)
	number(&header, 1)
	header.WriteByte(9)
	number(&header, uint64(packed.Len()))
	header.WriteByte(0)
	header.WriteByte(7)
	header.WriteByte(11)
	number(&header, 1)
	header.WriteByte(0)
	number(&header, 1)
	header.Write([]byte{1, 0})
	header.WriteByte(12)
	number(&header, uint64(packed.Len()))
	header.WriteByte(0)
	header.WriteByte(8)
	header.WriteByte(13)
	number(&header, uint64(len(names)))
	header.WriteByte(9)
	for _, name := range names[:len(names)-1] {
		number(&header, uint64(len(entries[name])))
	}
	header.WriteByte(0)
	header.WriteByte(0)
	header.WriteByte(5)
	number(&header, uint64(len(names)))
	var encoded bytes.Buffer
	encoded.WriteByte(0)
	for _, name := range names {
		for _, unit := range append(utf16.Encode([]rune(name)), 0) {
			binary.Write(&encoded, binary.LittleEndian, unit)
		}
	}
	header.WriteByte(17)
	number(&header, uint64(encoded.Len()))
	header.Write(encoded.Bytes())
	header.Write([]byte{0, 0})
	var start bytes.Buffer
	binary.Write(&start, binary.LittleEndian, uint64(packed.Len()))
	binary.Write(&start, binary.LittleEndian, uint64(header.Len()))
	binary.Write(&start, binary.LittleEndian, crc32.ChecksumIEEE(header.Bytes()))
	var out bytes.Buffer
	out.Write([]byte("MZ-pure-inert-fixture-not-an-executable"))
	out.Write([]byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c, 0, 4})
	binary.Write(&out, binary.LittleEndian, crc32.ChecksumIEEE(start.Bytes()))
	out.Write(start.Bytes())
	out.Write(packed.Bytes())
	out.Write(header.Bytes())
	return out.Bytes()
}
func sortStrings(values []string) {
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
func firefoxFixture(version string) []byte {
	return sevenFixture(map[string][]byte{"core/firefox.exe": fakePE(), "core/xul.dll": []byte("dll"), "core/omni.ja": []byte("omni"), "core/application.ini": []byte("[App]\nVersion=" + strings.TrimSuffix(version, "esr") + "\n"), "core/platform.ini": []byte("[Build]\n"), "setup.exe": []byte("MUST NOT EXTRACT OR RUN")})
}
func firefoxRoutes(t *testing.T, archive []byte, checksum string) fakeHTTP {
	return func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case mozillaArchive + "158.0.1/SHA512SUMS":
			return response([]byte(checksum + "  win64/en-US/Firefox Setup 158.0.1.exe\n")), nil
		case mozillaArchive + "158.0.1/win64/en-US/Firefox%20Setup%20158.0.1.exe":
			return response(archive), nil
		default:
			t.Fatalf("unexpected URL %s", r.URL)
			return nil, ErrSource
		}
	}
}
func TestFirefoxSFXCoreOnlyAndPublisherHash(t *testing.T) {
	archive := firefoxFixture("158.0.1")
	sum := sha512.Sum512(archive)
	hash := hex.EncodeToString(sum[:])
	m := fakeManager(t, firefoxRoutes(t, archive, hash))
	item, err := m.DownloadVersion(context.Background(), Firefox, "158.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if item.Receipt.PublisherSHA512 != hash || len(item.Receipt.Files) != 5 || item.CanLaunch {
		t.Fatal(item)
	}
	if _, err := os.Stat(filepath.Join(m.directory, Firefox, "158.0.1", "setup.exe")); !os.IsNotExist(err) {
		t.Fatal("installer was extracted")
	}
	if _, err := m.InspectInstalled(Firefox, "158.0.1"); err != nil {
		t.Fatal(err)
	}
	bad := fakeManager(t, firefoxRoutes(t, archive, strings.Repeat("a", 128)))
	if _, err := bad.DownloadVersion(context.Background(), Firefox, "158.0.1"); err != ErrIntegrity {
		t.Fatal(err)
	}
}
func TestFirefoxVersionMismatchRefused(t *testing.T) {
	archive := firefoxFixture("158.0.2")
	sum := sha512.Sum512(archive)
	m := fakeManager(t, firefoxRoutes(t, archive, hex.EncodeToString(sum[:])))
	if _, err := m.DownloadVersion(context.Background(), Firefox, "158.0.1"); err != ErrIntegrity {
		t.Fatal(err)
	}
	if _, err := m.InspectInstalled(Firefox, "158.0.1"); err == nil {
		t.Fatal("incomplete version accepted")
	}
}
func TestCatalogUsesBothOfficialFeeds(t *testing.T) {
	m := fakeManager(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case chromeCatalog:
			return response([]byte(`{"versions":[` + string(chromeMetadata("156.0.8078.47")) + `]}`)), nil
		case mozillaVersions:
			return response([]byte(`{"LATEST_FIREFOX_VERSION":"158.0.1","FIREFOX_ESR":"153.5.1esr"}`)), nil
		case mozillaArchive:
			return response([]byte(`<a href="158.0.1/">current</a><a href="/pub/firefox/releases/153.5.1esr/">ESR</a><a href="../">parent</a><a href="https://evil.test/159.0/">external</a>`)), nil
		default:
			t.Fatal(r.URL)
			return nil, ErrSource
		}
	})
	catalog, err := m.RefreshCatalog(context.Background())
	if err != nil || len(catalog.Releases) != 3 {
		t.Fatal(catalog, err)
	}
}

// Opt-in static review consumes an already downloaded official installer and
// checksum file only. It does not network or launch any browser/installer.
func TestManualOfficialFirefoxStaticExtraction(t *testing.T) {
	archivePath := os.Getenv("ULTRA_ENGINE_STATIC_REVIEW_ARCHIVE")
	if archivePath == "" {
		t.Skip("explicit cached official archive required")
	}
	sumPath := os.Getenv("ULTRA_ENGINE_STATIC_REVIEW_SUMS")
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(sumPath)
	if err != nil {
		t.Fatal(err)
	}
	checksum := ""
	for _, line := range strings.Split(string(sums), "\n") {
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) == 2 && parts[1] == "win64/en-US/Firefox Setup 158.0.1.exe" {
			checksum = parts[0]
		}
	}
	if checksum == "" {
		t.Fatal("checksum missing")
	}
	m := fakeManager(t, firefoxRoutes(t, data, checksum))
	m.verifyVersion = verifyBinaryVersion
	result, err := m.DownloadVersion(context.Background(), Firefox, "158.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.InspectInstalled(Firefox, "158.0.1"); err != nil {
		t.Fatal(err)
	}
	t.Logf("static-only official Firefox %s: files=%d dirs=%d archive_sha512=%s no execution", result.Receipt.Release.Version, len(result.Receipt.Files), len(result.Receipt.Directories), result.Receipt.ArchiveSHA512)
}
