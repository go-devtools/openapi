package openapi

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/openapi-golang/openapi/spec"
)

// 比较已知条件，保留未知设置诊断并允许仅补丁版本不同。
// Compare known selectors, report unknown settings, and permit patch-only toolchain differences.
func TestRuntimeBuildComparison(t *testing.T) {
	actual := BuildProfile{GoVersion: "go1.27.1", GOOS: "linux", GOARCH: "amd64", CGOEnabled: "0", Settings: map[string]string{"-tags": "beta,alpha", "GOEXPERIMENT": "", "GOAMD64": "v1"}}
	for _, tc := range []struct {
		name     string
		expected BuildProfile
		code     string
		severity Severity
	}{
		{"matching-tags", BuildProfile{Settings: map[string]string{"-tags": "alpha,beta,alpha"}}, "", ""},
		{"goos", BuildProfile{GOOS: "darwin"}, "openapi.build.mismatch", Error},
		{"goarch", BuildProfile{GOARCH: "arm64"}, "openapi.build.mismatch", Error},
		{"cgo", BuildProfile{CGOEnabled: "1"}, "openapi.build.mismatch", Error},
		{"tags", BuildProfile{Settings: map[string]string{"-tags": ""}}, "openapi.build.mismatch", Error},
		{"experiments", BuildProfile{GoExperiment: "loopvar"}, "openapi.build.mismatch", Error},
		{"architecture", BuildProfile{Settings: map[string]string{"GOAMD64": "v2"}}, "openapi.build.mismatch", Error},
		{"unknown-setting", BuildProfile{Settings: map[string]string{"FUTURE": "value"}}, "openapi.build.unknown", Warning},
		{"release-tags", BuildProfile{GoVersion: "go1.28.0"}, "openapi.build.mismatch", Error},
		{"patch", BuildProfile{GoVersion: "go1.27.0"}, "openapi.build.toolchain", Warning},
		{"unparseable", BuildProfile{GoVersion: "custom-toolchain"}, "openapi.build.unknown", Warning},
		{"legacy", BuildProfile{}, "openapi.build.unrecorded", Warning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := copyJSON(tc.expected)
			report := compareRuntimeBuild(tc.expected, actual)
			if !reflect.DeepEqual(original, tc.expected) {
				t.Fatal("modified caller-owned settings")
			}
			if tc.code == "" {
				if len(report.Diagnostics) != 0 {
					t.Fatal(report.Diagnostics)
				}
				return
			}
			if len(report.Diagnostics) != 1 || report.Diagnostics[0].Code != tc.code || report.Diagnostics[0].Severity != tc.severity {
				t.Fatalf("unexpected diagnostic: %+v", report.Diagnostics)
			}
		})
	}
	unknown := compareRuntimeBuild(BuildProfile{Settings: map[string]string{"-tags": ""}}, BuildProfile{})
	if len(unknown.Diagnostics) != 1 || unknown.Diagnostics[0].Code != "openapi.build.unknown" {
		t.Fatal("missing metadata was treated as empty", unknown)
	}
}

// 运行时校验由核心调用方选择，跨目标离线导出无需伪造宿主信息。
// Let core callers select runtime validation while retaining cross-target offline exports.
func TestBuildRuntimeValidationBeforeConfiguration(t *testing.T) {
	data := testBundle(t).Snapshot()
	data.Profile.GOOS = "linux"
	if runtime.GOOS == "linux" {
		data.Profile.GOOS = "darwin"
	}
	bundle, err := NewBundle(data)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Title: "Runtime", Version: "1"}
	if _, err := Build(bundle, nil, cfg); err != nil {
		t.Fatal("offline export rejected", err)
	}
	configured := false
	cfg.VerifyRuntimeBuild = true
	cfg.Configure = func(*spec.OpenAPI) error { configured = true; return nil }
	if _, err := Build(bundle, nil, cfg); err == nil {
		t.Fatal("runtime mismatch was accepted")
	}
	if configured {
		t.Fatal("configuration callback ran before build checks")
	}
}

// 历史 Bundle 缺少构建信息仍可使用，但诊断不能声称已验证代码同源。
// Keep legacy Bundles usable while reporting that their runtime build inputs are unrecorded.
func TestBuildRetainsRuntimeWarnings(t *testing.T) {
	doc, err := Build(testBundle(t), nil, Config{Title: "Legacy", Version: "1", VerifyRuntimeBuild: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diagnostic := range doc.Report().Diagnostics {
		if diagnostic.Code == "openapi.build.unrecorded" {
			found = true
		}
	}
	if !found {
		t.Fatal("runtime warning was discarded")
	}
}
