package swaggerui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// Verify every embedded upstream asset against its pinned digest and preserve required notices.
// 对每个内嵌上游资源核对固定摘要，并保留必要的许可证与声明。
func TestBundledAssetLicenses(t *testing.T) {
	raw, err := os.ReadFile("checksums.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string
		SHA256  map[string]string
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != Version {
		t.Fatal("asset manifest version differs from the public UI version")
	}
	entries, err := fs.ReadDir(assets, "assets")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(manifest.SHA256) {
		t.Fatal("embedded assets and manifest entries differ")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatal("unexpected asset directory")
		}
		data, err := assets.ReadFile("assets/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != manifest.SHA256[entry.Name()] {
			t.Errorf("asset checksum mismatch: %s", entry.Name())
		}
	}
	for name, phrase := range map[string]string{"LICENSE": "Apache License", "NOTICE": "SmartBear", "swagger-ui-bundle.js.LICENSE.txt": "license", "swagger-ui-standalone-preset.js.LICENSE.txt": "license"} {
		data, err := assets.ReadFile("assets/" + name)
		if err != nil || !strings.Contains(strings.ToLower(string(data)), strings.ToLower(phrase)) {
			t.Errorf("upstream license notice is missing: %s", name)
		}
	}
	project, err := os.ReadFile("../LICENSE")
	if err != nil || !strings.Contains(string(project), "MIT License") {
		t.Fatal("project license is missing")
	}
	notice, err := os.ReadFile("NOTICE")
	if err != nil || !strings.Contains(string(notice), Version) || !strings.Contains(string(notice), "Apache-2.0") {
		t.Fatal("upstream attribution or version is missing")
	}
}
