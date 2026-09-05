package openapi

import (
	"fmt"
	"go/version"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// 对比当前程序可读取的构建条件；不读源码、不执行分析器，也不证明源码同源。
// Compare observable executable build conditions without loading source, running analyzers, or proving source equivalence.
func CheckRuntimeBuild(expected BuildProfile) Report {
	return compareRuntimeBuild(expected, executableBuildProfile())
}

// 只提取公开目标选择，不复制可能含部署值的链接参数。
// Extract public target selectors without copying linker flags that may contain deployment values.
func executableBuildProfile() BuildProfile {
	profile := BuildProfile{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Settings: map[string]string{}}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return profile
	}
	settings := map[string]string{}
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	profile.CGOEnabled = settings["CGO_ENABLED"]
	for _, key := range []string{"-tags", "GOEXPERIMENT", "GOAMD64", "GO386", "GOARM", "GOARM64", "GOMIPS", "GOMIPS64", "GOPPC64", "GORISCV64", "GOWASM", "GOFIPS140"} {
		if value, present := settings[key]; present {
			profile.Settings[key] = value
		}
	}
	// 已验证的 Go 1.27 gc 写入器省略空 tags/实验设置和关闭的 FIPS；其他写入器保持未知。
	// The verified Go 1.27 gc writer omits empty tags, experiments, and disabled FIPS; retain unknown state for other writers.
	if settings["-compiler"] == "gc" && version.Lang(info.GoVersion) == "go1.27" &&
		settings["GOOS"] == runtime.GOOS && settings["GOARCH"] == runtime.GOARCH && profile.CGOEnabled != "" {
		for key, value := range map[string]string{"-tags": "", "GOEXPERIMENT": "", "GOFIPS140": "off"} {
			if _, present := profile.Settings[key]; !present {
				profile.Settings[key] = value
			}
		}
	}
	return profile
}

// 按稳定键顺序比较已记录输入，未知与已知空值分别诊断。
// Compare recorded inputs in stable key order and distinguish unknown values from known empty values.
func compareRuntimeBuild(expected, actual BuildProfile) Report {
	report := Report{Diagnostics: []Diagnostic{}}
	add := func(code string, severity Severity, key, message string) {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{Code: code, Severity: severity, Message: message,
			Source: Source{Kind: "derived", Rule: "runtime.build." + key},
			Fix:    "使用应用实际构建条件重新生成；缺少元数据时在 CI 中用源码检查新鲜度"})
	}
	wanted := runtimeSelectors(expected)
	available := runtimeSelectors(actual)
	keys := make([]string, 0, len(wanted))
	for key := range wanted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		want := wanted[key]
		have, known := available[key]
		if !known {
			add("openapi.build.unknown", Warning, key, "运行时没有可验证的构建设置："+key)
			continue
		}
		if key == "-tags" {
			want, have = normalizedBuildTags(want), normalizedBuildTags(have)
		}
		if want != have {
			add("openapi.build.mismatch", Error, key, fmt.Sprintf("构建设置 %s 不匹配：Bundle=%q，runtime=%q", key, want, have))
		}
	}
	if expected.GoVersion != "" {
		want, have := version.Lang(expected.GoVersion), version.Lang(actual.GoVersion)
		switch {
		case want == "" || have == "":
			add("openapi.build.unknown", Warning, "GOVERSION", "无法确认工具链的 Go 语言版本")
		case want != have:
			add("openapi.build.mismatch", Error, "GOVERSION", fmt.Sprintf("Go 语言版本不匹配：Bundle=%q，runtime=%q", want, have))
		case expected.GoVersion != actual.GoVersion:
			add("openapi.build.toolchain", Warning, "GOVERSION", fmt.Sprintf("Go 工具链版本不同：Bundle=%q，runtime=%q", expected.GoVersion, actual.GoVersion))
		}
	}
	if len(wanted) == 0 && expected.GoVersion == "" {
		add("openapi.build.unrecorded", Warning, "profile", "Bundle 未记录可校验的构建条件")
	}
	return report
}

// 将旧标量和新增显式设置统一成副本，空 map 值仍表示已知值。
// Copy legacy scalars and explicit settings into one view, preserving known empty map values.
func runtimeSelectors(profile BuildProfile) map[string]string {
	selectors := map[string]string{}
	for key, value := range profile.Settings {
		selectors[key] = value
	}
	for key, value := range map[string]string{"GOOS": profile.GOOS, "GOARCH": profile.GOARCH, "CGO_ENABLED": profile.CGOEnabled, "GOEXPERIMENT": profile.GoExperiment} {
		if value != "" {
			selectors[key] = value
		}
	}
	if len(profile.Tags) > 0 {
		selectors["-tags"] = strings.Join(profile.Tags, ",")
	}
	return selectors
}

// tags 是集合，重复项、顺序和分隔空白不改变构建条件。
// Treat build tags as a set independent of duplicates, ordering, and delimiter whitespace.
func normalizedBuildTags(value string) string {
	seen := map[string]bool{}
	for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		seen[tag] = true
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return strings.Join(tags, ",")
}
