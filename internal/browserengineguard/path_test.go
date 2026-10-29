package browserengineguard

import "testing"

func TestKnownFolderNamespaceRefusedBeforeDriveProbe(t *testing.T) {
	for _, path := range []string{`\\example.invalid\share\cache`, `//example.invalid/share/cache`, `//?/UNC/example.invalid/share/cache`, `\\?\C:\cache`, `\\.\C:\cache`, `C:relative`, `C:/cache`, `C:\cache:stream`, `C:\a\..\cache`, `C:\a\.\cache`, `C:\cache\`, `C:\cache.`, `C:\cache `, `C:\NUL.txt`, `C:\COM¹.txt`, `C:\folder\LPT9`, `C:\a` + "\x00" + `b`} {
		t.Run(path, func(t *testing.T) {
			called := false
			err := verifyLocalDirectory(path, func(string) error { called = true; return nil })
			if err != ErrRefused || called {
				t.Fatalf("err=%v probe=%v", err, called)
			}
		})
	}
}
func TestKnownFolderValidLocalPathRequiresConfirmedDrive(t *testing.T) {
	for _, path := range []string{`C:\Users\user\AppData\Local`, `D:\用户\本地目录`} {
		called := false
		err := verifyLocalDirectory(path, func(root string) error {
			called = true
			if root != path[:3] {
				t.Fatal(root)
			}
			return nil
		})
		if err != nil || !called {
			t.Fatal(path, err)
		}
	}
	if verifyLocalDirectory(`C:\Users\user\AppData\Local`, func(string) error { return ErrRefused }) != ErrRefused {
		t.Fatal("unknown drive accepted")
	}
}
func TestKnownFolderRejectsNetworkAndSubstDeviceMapping(t *testing.T) {
	for _, mapping := range []string{`\Device\Mup\example.invalid\share`, `\Device\LanmanRedirector\share`, `\??\C:\other`, `\Device\HarddiskVolume`, `\Device\HarddiskVolume1\other`, `\Device\HarddiskVolumeX`} {
		if fixedVolumeMapping(mapping) {
			t.Fatal(mapping)
		}
	}
	for _, mapping := range []string{`\Device\HarddiskVolume1`, `\Device\HarddiskVolume42`} {
		if !fixedVolumeMapping(mapping) {
			t.Fatal(mapping)
		}
	}
}
