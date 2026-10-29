package browserengine

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestArchiveDeclaredExpansionAndCorruption(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		size  uint32
	}{{"member", 1, uint32(maxMember + 1)}, {"total", 5, uint32(maxMember)}} {
		t.Run(test.name, func(t *testing.T) {
			entries := map[string][]byte{}
			for i := 0; i < test.count; i++ {
				entries[fmt.Sprintf("chrome-win64/file%d", i)] = []byte("x")
			}
			data := zipFixture(t, entries)
			for offset := 0; offset+46 <= len(data); offset++ {
				if bytes.Equal(data[offset:offset+4], []byte{0x50, 0x4b, 0x01, 0x02}) {
					binary.LittleEndian.PutUint32(data[offset+24:], test.size)
				}
			}
			release, _ := chromeRelease(chromeDataFrom(t, chromeMetadata("156.0.8078.47")))
			if _, err := archiveEntries(context.Background(), bytes.NewReader(data), int64(len(data)), release); err != ErrArchive {
				t.Fatal(err)
			}
		})
	}
	data := firefoxFixture("158.0.1")
	data[len(data)-1] ^= 0xff
	sum := sha512.Sum512(data)
	m := fakeManager(t, firefoxRoutes(t, data, hex.EncodeToString(sum[:])))
	if _, err := m.DownloadVersion(context.Background(), Firefox, "158.0.1"); err != ErrArchive {
		t.Fatal(err)
	}
}

func TestSourceJSONDuplicatesAndArbitraryDownloadRefused(t *testing.T) {
	for _, metadata := range []string{`{"version":"156.0.8078.47","version":"156.0.8078.47","downloads":{"chrome":[]}}`, string(bytes.ReplaceAll(chromeMetadata("156.0.8078.47"), []byte("https://storage.googleapis.com/"), []byte("https://attacker.invalid/")))} {
		m := fakeManager(t, func(r *http.Request) (*http.Response, error) {
			if strings.Contains(r.URL.Host, "storage") {
				t.Fatal("untrusted artifact download attempted")
			}
			return response([]byte(metadata)), nil
		})
		if _, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrSource {
			t.Fatal(err)
		}
	}
	for _, selection := range [][2]string{{Chrome, "../156"}, {Firefox, "158.0.1/evil"}, {"custom", "1.2.3.4"}, {Firefox, "158.0.1?url=x"}} {
		m := fakeManager(t, func(r *http.Request) (*http.Response, error) {
			t.Fatal("invalid selection performed network")
			return nil, ErrSource
		})
		if _, err := m.DownloadVersion(context.Background(), selection[0], selection[1]); err != ErrInput {
			t.Fatal(err)
		}
	}
}

func TestManualOfficialChromeStaticExtraction(t *testing.T) {
	archivePath := os.Getenv("ULTRA_ENGINE_STATIC_REVIEW_CHROME")
	if archivePath == "" {
		t.Skip("explicit cached official archive required")
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	m := fakeManager(t, chromeRoutes(t, data))
	m.verifyVersion = verifyBinaryVersion
	result, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.InspectInstalled(Chrome, "156.0.8078.47"); err != nil {
		t.Fatal(err)
	}
	t.Logf("static-only official CfT %s: files=%d dirs=%d archive_sha256=%s no execution; official feed supplies no archive checksum", result.Receipt.Release.Version, len(result.Receipt.Files), len(result.Receipt.Directories), result.Receipt.ArchiveSHA256)
}

func TestOpenedRootIdentityFailurePreventsNetwork(t *testing.T) {
	m := fakeManager(t, func(*http.Request) (*http.Response, error) {
		t.Fatal("network after root identity failure")
		return nil, ErrSource
	})
	m.verifyRoot = func(*os.Root, string) error { return ErrStore }
	if _, err := m.DownloadVersion(context.Background(), Chrome, "156.0.8078.47"); err != ErrStore {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(m.directory)
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}
