// 验证公开模块边界，测试消费者始终位于本 module 目录之外。
// Verify public module boundaries using consumers outside this module directory.
package verify

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 从测试包定位本次待验收的核心 checkout。
// Locate the core checkout under test from the test package.
func rootDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// 运行有时间预算的 Go 子进程，显式关闭环境 workspace。
// Run time-bounded Go subprocesses with workspace mode explicitly disabled.
func runGo(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s 失败：%v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// 核心及其测试依赖树不能引入任何产品适配器或已知 HTTP 框架。
// Exclude product adapters and known HTTP frameworks from the core and its test dependency graph.
func TestNoFrameworkDependencies(t *testing.T) {
	out := runGo(t, rootDir(t), "list", "-deps", "-test", "-f", "{{.ImportPath}}", "./...")
	for _, forbidden := range []string{"github.com/gin-gonic/", "github.com/gofiber/", "github.com/labstack/echo", "github.com/openapi-golang/gin-swagger"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("核心反向依赖 %s", forbidden)
		}
	}
}

// 根包不加载编译器、独立测试引擎或 UI 静态资源。
// Keep compiler, independent validation engine, and UI assets out of the root package's dependencies.
func TestRuntimeDependencyBoundary(t *testing.T) {
	out := runGo(t, rootDir(t), "list", "-deps", "-f", "{{.ImportPath}}", ".")
	for _, forbidden := range []string{"golang.org/x/tools", "github.com/openapi-golang/openapi/compiler", "github.com/openapi-golang/openapi/swaggerui", "github.com/openapi-golang/openapi/contracttest", "github.com/santhosh-tekuri/jsonschema"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("运行时包含可选依赖 %s", forbidden)
		}
	}
}

// 独立 module 验证公开扩展入口；远端验收时显式指定已经同步的固定版本。
// Verify public extension APIs from an independent module, selecting a synchronized fixed version for remote acceptance.
func TestExternalFrontend(t *testing.T) {
	root, dir := rootDir(t), t.TempDir()
	version := os.Getenv("OPENAPI_TEST_CORE_VERSION")
	remote := version != ""
	if !remote {
		// 此已存在的首批版本仅用于临时开发替换，不冒充最终远端验收。
		// Use this existing initial version only for temporary development replacement, not final remote acceptance.
		version = "v0.0.0-20260905054024-c28ea4b52b58"
	}
	for _, name := range []string{"types.go", "consumer_test.go", "call_flow_test.go", "call_outcome_test.go", "runtime_conditions_test.go", "compiler_conditions_test.go"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "external", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	mod := "module example.test/consumer\n\ngo 1.27.1\n\nrequire github.com/openapi-golang/openapi " + version + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	if !remote {
		runGo(t, dir, "mod", "edit", "-replace=github.com/openapi-golang/openapi="+root)
		t.Log("开发 SDK 验证：临时 module 使用本地核心；不能据此宣称远端固定版本验收")
	}
	runGo(t, dir, "mod", "tidy")
	var module struct {
		Version string
		Replace *struct{}
	}
	if err := json.Unmarshal([]byte(runGo(t, dir, "list", "-m", "-json", "github.com/openapi-golang/openapi")), &module); err != nil {
		t.Fatal(err)
	}
	if remote && (module.Replace != nil || module.Version != version) {
		t.Fatal("远端验收存在替换或版本不匹配")
	}
	t.Log(runGo(t, dir, "test", "-count=1", "-v", "./..."))
}
