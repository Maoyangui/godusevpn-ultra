package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// No filesystem ACLs, accounts or services are modified by these tests.
type fakeStore struct {
	nodes          map[string]storeNode
	aliases        map[string]string
	writes         []string
	opened, closed int
	hardenErr      error
	driveErr       error
	publicACL      bool
}

func safeDirectory() storeNode {
	return storeNode{directory: true, owner: "S-1-5-18", aces: []storeACE{{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: windows.GENERIC_ALL, sid: "S-1-5-18"}}}
}

const fakeRoot = `C:\ProgramData\godusevpn-ultra`

func newFakeStore() *fakeStore {
	volume := safeDirectory()
	volume.owner = trustedInstallerSID
	parent := safeDirectory()
	// ProgramData permits creating new siblings, not replacing existing children.
	parent.aces = append(parent.aces, storeACE{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: windows.FILE_APPEND_DATA | windows.FILE_WRITE_DATA, sid: "S-1-5-32-545"})
	return &fakeStore{nodes: map[string]storeNode{`C:\`: volume, `C:\ProgramData`: parent}}
}

func (f *fakeStore) fixedDrive(string) error { return f.driveErr }
func (f *fakeStore) open(path string) (*storeNode, error) {
	n, ok := f.nodes[path]
	if !ok && f.aliases != nil {
		n, ok = f.nodes[f.aliases[path]]
	}
	if !ok {
		return nil, os.ErrNotExist
	}
	f.opened++
	n.close = func() { f.closed++ }
	return &n, nil
}
func (f *fakeStore) create(path string, readable bool) error {
	f.publicACL = f.publicACL || readable
	f.writes = append(f.writes, "create:"+path)
	if _, exists := f.nodes[path]; !exists {
		f.nodes[path] = safeDirectory()
	}
	return nil
}
func (f *fakeStore) entries(path string) ([]string, error) {
	var names []string
	for key := range f.nodes {
		if key != path && filepath.Dir(key) == path {
			names = append(names, filepath.Base(key))
		}
	}
	sort.Strings(names)
	return names, nil
}
func (f *fakeStore) harden(path string, readable bool) error {
	f.publicACL = f.publicACL || readable
	f.writes = append(f.writes, "harden:"+path)
	return f.hardenErr
}

func TestLegacyLogAndDiagnosticDescendantsBecomePrivateOnlyAfterValidation(t *testing.T) {
	f := newFakeStore()
	f.nodes[fakeRoot] = safeDirectory()
	var expected []string
	for _, sub := range []string{"logs", "diag"} {
		for _, relative := range []string{sub, filepath.Join(sub, "nested"), filepath.Join(sub, "nested", "synthetic.log")} {
			path := filepath.Join(fakeRoot, relative)
			n := safeDirectory()
			if strings.HasSuffix(path, ".log") {
				n.directory, n.links = false, 1
			}
			n.aces = append(n.aces, storeACE{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: windows.GENERIC_READ, sid: "S-1-5-32-545"})
			f.nodes[path] = n
			expected = append(expected, "harden:"+path)
		}
	}
	release, err := prepareStore(fakeRoot, f)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if f.publicACL {
		t.Fatal("new or migrated service artifacts granted public read")
	}
	for _, want := range expected {
		found := false
		for _, got := range f.writes {
			found = found || got == want
		}
		if !found {
			t.Fatalf("legacy protected descendant not tightened: %s", want)
		}
	}
}

// TEMP-only DACL operation. Ownership is intentionally unchanged; this does
// not pretend the temporary parent satisfies the privileged store preflight.
func TestNativePrivateACLRemovesLegacyPublicReadFromTemporaryFiles(t *testing.T) {
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("TEMP role-only ACL test requires an already elevated test process")
	}
	for _, directory := range []bool{true, false} {
		root := t.TempDir()
		path := filepath.Join(root, "synthetic-artifact")
		if directory {
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("synthetic log fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		legacy, err := windows.SecurityDescriptorFromString("D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GRGX;;;BU)")
		if err != nil {
			t.Fatal(err)
		}
		acl, _, _ := legacy.DACL()
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
			t.Fatal(err)
		}
		before, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		old, err := descriptorNode(before)
		if err != nil {
			t.Fatal(err)
		}
		if err := (nativeStore{}).harden(path, false); err != nil {
			t.Fatal(err)
		}
		after, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		n, err := descriptorNode(after)
		if err != nil || n.owner != old.owner {
			t.Fatal("tightening took ownership", err)
		}
		for _, ace := range n.aces {
			if ace.typ == windows.ACCESS_ALLOWED_ACE_TYPE && !trustedStoreSID(ace.sid) {
				t.Fatal("private artifact still grants another account/group access")
			}
		}
	}
}

func TestStorePrepareCreatesPrivateTreeAndPinsUntilRelease(t *testing.T) {
	f := newFakeStore()
	release, err := prepareStore(fakeRoot, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"logs", "diag", "profiles", "rulesets"} {
		if _, ok := f.nodes[filepath.Join(fakeRoot, sub)]; !ok {
			t.Fatalf("missing %s", sub)
		}
	}
	if f.closed != 0 {
		t.Fatal("directory handles released before caller finished")
	}
	release()
	release()
	if f.closed != f.opened {
		t.Fatalf("release not complete/idempotent: opened=%d closed=%d", f.opened, f.closed)
	}
}

func TestStoreRejectsUntrustedExistingRootWithoutTakingOwnership(t *testing.T) {
	f := newFakeStore()
	root := safeDirectory()
	root.owner = "S-1-5-21-123-456-789-1001"
	f.nodes[fakeRoot] = root
	if _, err := prepareStore(fakeRoot, f); err == nil {
		t.Fatal("ordinary-user owner accepted")
	}
	if len(f.writes) != 0 {
		t.Fatalf("untrusted root modified: %v", f.writes)
	}
	if f.closed != f.opened {
		t.Fatal("failure leaked pinned handles")
	}
}

func TestStoreRejectsReplaceableAncestorBeforeCreatingRoot(t *testing.T) {
	for _, mask := range []uint32{fileDeleteChild, windows.DELETE, windows.WRITE_DAC, windows.WRITE_OWNER, windows.GENERIC_WRITE} {
		t.Run(fmt.Sprintf("mask_%x", mask), func(t *testing.T) {
			f := newFakeStore()
			n := f.nodes[`C:\ProgramData`]
			n.aces = append(n.aces, storeACE{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: mask, sid: "S-1-1-0"})
			f.nodes[`C:\ProgramData`] = n
			if _, err := prepareStore(fakeRoot, f); err == nil {
				t.Fatalf("ancestor mask %#x accepted", mask)
			}
			if len(f.writes) != 0 {
				t.Fatal("created data below untrusted ancestor")
			}
		})
	}
}

func TestStoreRejectsExistingLinksAndWritableFilesBeforeHardening(t *testing.T) {
	for _, kind := range []string{"root_reparse", "ancestor_reparse", "file_reparse", "file_hardlink", "user_file_owner", "explicit_user_write", "child_directory_write", "unknown_ace"} {
		t.Run(kind, func(t *testing.T) {
			f := newFakeStore()
			f.nodes[fakeRoot] = safeDirectory()
			file := storeNode{owner: "S-1-5-18", links: 1, aces: []storeACE{{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: windows.GENERIC_ALL, sid: "S-1-5-18"}}}
			switch kind {
			case "root_reparse":
				n := f.nodes[fakeRoot]
				n.reparse = true
				f.nodes[fakeRoot] = n
			case "ancestor_reparse":
				n := f.nodes[`C:\ProgramData`]
				n.reparse = true
				f.nodes[`C:\ProgramData`] = n
			case "file_reparse":
				file.reparse = true
			case "file_hardlink":
				file.links = 2
			case "user_file_owner":
				file.owner = "S-1-5-21-123-456-789-1001"
			case "explicit_user_write":
				file.aces = append(file.aces, storeACE{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: windows.FILE_WRITE_DATA, sid: "S-1-5-32-545"})
			case "child_directory_write":
				file.directory = true
				file.aces = append(file.aces, storeACE{typ: windows.ACCESS_ALLOWED_ACE_TYPE, mask: windows.FILE_WRITE_DATA, sid: "S-1-5-32-545"})
			case "unknown_ace":
				file.aces = append(file.aces, storeACE{typ: 5}) // ACCESS_ALLOWED_OBJECT_ACE_TYPE
			}
			f.nodes[filepath.Join(fakeRoot, "settings.json")] = file
			if _, err := prepareStore(fakeRoot, f); err == nil {
				t.Fatal("untrusted entry accepted")
			}
			if len(f.writes) != 0 {
				t.Fatalf("changed an unvalidated tree: %v", f.writes)
			}
			if f.closed != f.opened {
				t.Fatal("failure leaked handles")
			}
		})
	}
}

func TestStoreHardenFailureIsFatal(t *testing.T) {
	f := newFakeStore()
	f.hardenErr = errors.New("fake denied")
	if release, err := prepareStore(fakeRoot, f); err == nil || release != nil {
		t.Fatal("harden failure ignored")
	}
	if f.closed != f.opened {
		t.Fatal("failure leaked handles")
	}
}

func TestStoreIgnoresUnconsumedOptionalPrivacyAndDataToolInputs(t *testing.T) {
	f := newFakeStore()
	f.nodes[fakeRoot] = safeDirectory()
	for _, name := range []string{"privacy-machine", "privacy-data", "future-optional-store", "unconsumed.json"} {
		n := safeDirectory()
		n.owner = "S-1-5-21-123-456-789-1001"
		n.reparse = true
		f.nodes[filepath.Join(fakeRoot, name)] = n
		// If traversal occurs this unsupported child fails too.
		f.nodes[filepath.Join(fakeRoot, name, "private.json")] = storeNode{}
	}
	release, err := prepareStore(fakeRoot, f)
	if err != nil {
		t.Fatalf("optional failure gated service: %v", err)
	}
	release()
	// 3 trusted ancestors/root plus the 4 core child directories only.
	if f.opened != 7 {
		t.Fatalf("opened optional input: %d handles", f.opened)
	}
}

func TestStoreConsumedSubdirectoryFailureStillBlocks(t *testing.T) {
	for _, sub := range []string{"logs", "diag", "profiles", "rulesets"} {
		t.Run(sub, func(t *testing.T) {
			f := newFakeStore()
			f.nodes[fakeRoot] = safeDirectory()
			f.nodes[filepath.Join(fakeRoot, sub)] = safeDirectory()
			bad := storeNode{owner: "S-1-5-21-123-456-789-1001", links: 1}
			f.nodes[filepath.Join(fakeRoot, sub, "unsafe-file")] = bad
			if _, err := prepareStore(fakeRoot, f); err == nil {
				t.Fatal("untrusted consumed subtree accepted")
			}
			if len(f.writes) != 0 {
				t.Fatal("modified store before consumed subtree check")
			}
		})
	}
}

func TestCoreDirectoryInheritanceCannotGrantFutureUntrustedWrites(t *testing.T) {
	for _, sub := range []string{"profiles", "rulesets", "logs", "diag"} {
		for _, sid := range []string{"S-1-5-32-545", "S-1-3-0"} {
			t.Run(sub+"/"+sid, func(t *testing.T) {
				f := newFakeStore()
				f.nodes[fakeRoot] = safeDirectory()
				n := safeDirectory()
				// No direct write grant on this directory: only future children.
				n.aces = append(n.aces, storeACE{typ: windows.ACCESS_ALLOWED_ACE_TYPE, flags: windows.INHERIT_ONLY_ACE | windows.OBJECT_INHERIT_ACE, mask: windows.GENERIC_WRITE, sid: sid})
				f.nodes[filepath.Join(fakeRoot, sub)] = n
				if _, err := prepareStore(fakeRoot, f); err == nil {
					t.Fatal("future child write grant accepted")
				}
				if len(f.writes) != 0 {
					t.Fatal("created/wrote data before inherited permission check")
				}
			})
		}
	}
	for _, tc := range []struct {
		sddl         string
		ancestor, ok bool
	}{
		{"O:BAG:BAD:P(A;;FA;;;BA)(A;OIIO;GW;;;BU)", false, false},
		{"O:BAG:BAD:P(A;;FA;;;BA)(A;OICIIO;GA;;;CO)", false, false},
		{"O:BAG:BAD:P(A;;FA;;;BA)(A;OICIIO;GA;;;CO)", true, true},
		{"O:BAG:BAD:P(A;;FA;;;BA)(A;OIIO;GW;;;SY)", false, true},
		{"O:BAG:BAD:P(A;;FA;;;BA)(A;OIIO;GR;;;BU)", false, true},
	} {
		sd, err := windows.SecurityDescriptorFromString(tc.sddl)
		if err != nil {
			t.Fatal(err)
		}
		n, err := descriptorNode(sd)
		if err != nil {
			t.Fatal(err)
		}
		n.directory = true
		if got := checkStoreNode(n, tc.ancestor) == nil; got != tc.ok {
			t.Errorf("future child descriptor accepted=%v expected=%v: %s", got, tc.ok, tc.sddl)
		}
	}
}

func TestStorePathRejectsUNCRelativeADSAndDeviceAliases(t *testing.T) {
	for _, path := range []string{"", `relative\data`, `C:\`, `C:\\`, `C:relative`, `\\server\share\data`, `\\?\C:\data`, `C:\data:secret`, `C:\a\..\data`, `C:\data.`, `C:\data `, `C:\NUL\data`, `C:\data/other`, `C:\bad*path`} {
		if _, err := cleanStorePath(path); err == nil {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
	f := newFakeStore()
	f.driveErr = errors.New("not fixed")
	if _, err := prepareStore(fakeRoot, f); err == nil || len(f.writes) != 0 {
		t.Fatal("unsupported drive changed")
	}
	for _, name := range []string{"COM¹", "COM².txt", "LPT³", "CONIN$", "CONOUT$.txt", "LPT1", "COM9"} {
		if _, err := cleanStorePath(`C:\` + name); err == nil {
			t.Errorf("reserved device accepted: %q", name)
		}
	}
}

func TestStoreProbesFixedPathsEvenWhenEnumerationWouldReturnDifferentNames(t *testing.T) {
	for _, tc := range []struct {
		fixed, alias string
		directory    bool
	}{{"settings.json", "settıngs.json", false}, {"profiles", "profıles", true}} {
		t.Run(tc.fixed, func(t *testing.T) {
			f := newFakeStore()
			f.nodes[fakeRoot] = safeDirectory()
			alias := filepath.Join(fakeRoot, tc.alias)
			f.nodes[alias] = storeNode{directory: tc.directory, links: 1, owner: "S-1-5-21-123-456-789-1001"}
			// Model filesystem alias resolution without creating real aliases.
			// This does not claim this Unicode pair aliases on this Windows host.
			f.aliases = map[string]string{filepath.Join(fakeRoot, tc.fixed): alias}
			if _, err := prepareStore(fakeRoot, f); err == nil {
				t.Fatal("fixed service input skipped")
			}
			if len(f.writes) != 0 {
				t.Fatal("changed data before fixed input validation")
			}
		})
	}
}

func TestStoreDescriptorPolicyWithoutFilesystem(t *testing.T) {
	for _, tc := range []struct {
		sddl         string
		ancestor, ok bool
	}{
		{sddlPrivate, false, true}, {"O:BAG:BAD:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GRGX;;;BU)", false, true}, // legacy read-only ACL is safe to tighten after validation
		{"O:BAG:BAD:P(A;;GA;;;BA)(A;;GW;;;BU)", false, false},
		{"O:BAG:BAD:P(A;IO;GA;;;CO)(A;;GA;;;BA)", false, true},
		{"O:BAG:BAD:NO_ACCESS_CONTROL", false, false},
	} {
		sd, err := windows.SecurityDescriptorFromString(tc.sddl)
		if err != nil {
			t.Fatal(err)
		}
		n, err := descriptorNode(sd)
		if err == nil {
			n.directory = true
			err = checkStoreNode(n, tc.ancestor)
		}
		if (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.sddl, err)
		}
	}
}

func TestWindowsDefaultIgnoresProgramDataEnvironment(t *testing.T) {
	t.Setenv(DataEnvironment, "")
	before := DataDir()
	t.Setenv("ProgramData", `Z:\untrusted-environment`)
	if DataDir() != before || strings.HasPrefix(DataDir(), `Z:\`) {
		t.Fatal("environment redirected default store")
	}
	t.Setenv(DataEnvironment, `D:\PrivatePortable\data`)
	if DataDir() != `D:\PrivatePortable\data` {
		t.Fatal("explicit Ultra override was lost")
	}
}
