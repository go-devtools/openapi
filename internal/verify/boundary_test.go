// Verify public module boundaries using consumers outside this module directory.
// 验证公开模块边界，测试消费者始终位于本 module 目录之外。
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

// Locate the core checkout under test from the test package.
// 从测试包定位本次待验收的核心 checkout。
func rootDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// Run time-bounded Go subprocesses with workspace mode explicitly disabled.
// 运行有时间预算的 Go 子进程，显式关闭环境 workspace。
func runGo(t *testing.T, dir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// Exclude product adapters and known HTTP frameworks from the core and its test dependency graph.
// 核心及其测试依赖树不能引入任何产品适配器或已知 HTTP 框架。
func TestNoFrameworkDependencies(t *testing.T) {
	out := runGo(t, rootDir(t), "list", "-deps", "-test", "-f", "{{.ImportPath}}", "./...")
	for _, forbidden := range []string{"github.com/gin-gonic/", "github.com/gofiber/", "github.com/labstack/echo", "github.com/openapi-golang/gin-swagger"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("core depends on %s", forbidden)
		}
	}
}

// Keep compiler, independent validation engine, and UI assets out of the root package's dependencies.
// 根包不加载编译器、独立测试引擎或 UI 静态资源。
func TestRuntimeDependencyBoundary(t *testing.T) {
	out := runGo(t, rootDir(t), "list", "-deps", "-f", "{{.ImportPath}}", ".")
	for _, forbidden := range []string{"golang.org/x/tools", "github.com/openapi-golang/openapi/compiler", "github.com/openapi-golang/openapi/swaggerui", "github.com/openapi-golang/openapi/contracttest", "github.com/santhosh-tekuri/jsonschema"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("runtime contains optional dependency %s", forbidden)
		}
	}
}

// Verify public extension APIs from an independent module, selecting a synchronized fixed version for remote acceptance.
// 独立 module 验证公开扩展入口；远端验收时显式指定已经同步的固定版本。
func TestExternalFrontend(t *testing.T) {
	root, dir := rootDir(t), t.TempDir()
	version := os.Getenv("OPENAPI_TEST_CORE_VERSION")
	remote := version != ""
	if !remote {
		// Use this existing initial version only for temporary development replacement, not final remote acceptance.
		// 此已存在的首批版本仅用于临时开发替换，不冒充最终远端验收。
		version = "v0.0.0-20260905054024-c28ea4b52b58"
	}
	for _, name := range []string{"types.go", "consumer_test.go", "call_flow_test.go", "call_outcome_test.go", "runtime_conditions_test.go", "compiler_conditions_test.go", "build_inputs_test.go", "build_inputs_boundary_test.go", "runtime_build_test.go", "runtime_inputs_test.go", "request_fields_test.go", "request_encoding_test.go", "http_response_test.go", "response_state_test.go", "response_items_test.go", "response_item_projection_test.go", "boxed_nil_test.go", "stream_boundary_test.go", "function_values_test.go", "callback_test.go", "declarations_test.go", "imported_metadata_test.go"} {
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
		t.Log("Local SDK validation uses a temporary module with a local core replacement.")
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
		t.Fatal("remote module validation found a replacement or version mismatch")
	}
	t.Log(runGo(t, dir, "test", "-count=1", "-v", "./..."))
}
