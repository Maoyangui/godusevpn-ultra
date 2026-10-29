package cli

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

func safeManifestPath(name string) bool {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, "\\:\x00<>\"|?*\r\n") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, s := range strings.Split(name, "/") {
		if s == "" || s == "." || s == ".." || strings.HasSuffix(s, ".") || strings.HasSuffix(s, " ") {
			return false
		}
	}
	return true
}

func verifyUninstallManifest(manifest, descriptor []byte, hashes map[string]string) error {
	if len(manifest) > 4<<20 || len(descriptor) > 64<<10 {
		return errors.New("发行清单大小超限")
	}
	var product struct{ Product, Namespace string }
	if json.Unmarshal(descriptor, &product) != nil || product.Product != "GodUseVPN Ultra" || product.Namespace != "godusevpn-ultra" {
		return errors.New("固定安装目录产品标识不符")
	}
	seen := map[string]bool{}
	lines := strings.Split(strings.TrimSuffix(string(manifest), "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if len(line) < 67 || line[64:66] != "  " {
			return errors.New("发行哈希清单格式不符")
		}
		hash, name := line[:64], line[66:]
		if _, e := hex.DecodeString(hash); e != nil || !safeManifestPath(name) || strings.EqualFold(name, "SHA256SUMS") || seen[strings.ToLower(name)] {
			return errors.New("发行清单有不安全或重复项")
		}
		seen[strings.ToLower(name)] = true
		if actual, ok := hashes[name]; !ok || !strings.EqualFold(actual, hash) {
			return errors.New("发行文件哈希缺失或不匹配")
		}
	}
	if len(hashes) != len(seen)+1 {
		return errors.New("安装目录含未列入清单的文件")
	}
	if _, ok := hashes["SHA256SUMS"]; !ok {
		return errors.New("安装目录缺少固定哈希清单")
	}
	for _, required := range []string{"godusevpn.exe", "godusevpn-cli.exe", "godusevpn-svc.exe", "ULTRA_PACKAGE.json", "Install-Ultra.ps1", "Uninstall-Ultra.ps1", "browser-extension/manifest.json"} {
		if !seen[strings.ToLower(required)] {
			return errors.New("发行包缺少必要恢复文件")
		}
	}
	return nil
}
