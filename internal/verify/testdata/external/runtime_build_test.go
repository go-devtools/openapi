package consumer

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/openapi-golang/openapi"
)

// 真实加载源码并构建独立程序，证明校验依据二进制元数据而非运行环境变量。
// Load real source and build an independent executable to verify binary metadata rather than runtime environment variables.
func TestRuntimeBuildFromExecutable(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"go.mod", "go.sum"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		fingerprintFile(t, dir, name, string(raw))
	}
	fingerprintFile(t, dir, "main.go", `package main
import("encoding/json";"os";"github.com/openapi-golang/openapi")
// 返回可静态分析的载荷。
// Return a statically analyzable payload.
func H() int { return 1 }
// 输出当前程序的公开构建诊断，不执行分析器。
// Emit public build diagnostics for this executable without invoking an analyzer.
func main() {
 var profile openapi.BuildProfile
 if err := json.Unmarshal([]byte(os.Args[1]), &profile); err != nil { panic(err) }
 if err := json.NewEncoder(os.Stdout).Encode(openapi.CheckRuntimeBuild(profile)); err != nil { panic(err) }
}
`)
	options := fingerprintOptions(dir)
	options.Load.Env = append(options.Load.Env, "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
	base := fingerprintCompile(t, options).Bundle.Snapshot().Profile
	binary := filepath.Join(dir, "probe")
	run := func(program string, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, program, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=", "GOEXPERIMENT=", "CGO_ENABLED=0", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
		if program == binary {
			cmd.Env = append(cmd.Env, "GOFLAGS=-tags=spoofed_runtime_environment", "CGO_ENABLED=1")
		}
		raw, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", program, args, err, raw)
		}
		return raw
	}
	check := func(profile openapi.BuildProfile, mismatch string) {
		t.Helper()
		encoded, err := json.Marshal(profile)
		if err != nil {
			t.Fatal(err)
		}
		raw := run(binary, string(encoded))
		var report openapi.Report
		if err := json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err, string(raw))
		}
		if mismatch == "" {
			if len(report.Diagnostics) != 0 {
				t.Fatalf("matching build was not verified: %+v", report.Diagnostics)
			}
			return
		}
		for _, diagnostic := range report.Diagnostics {
			if diagnostic.Code == "openapi.build.mismatch" && diagnostic.Severity == openapi.Error && strings.HasSuffix(diagnostic.Source.Rule, "."+mismatch) {
				return
			}
		}
		t.Fatalf("missing %s mismatch: %+v", mismatch, report.Diagnostics)
	}
	run("go", "build", "-trimpath", "-ldflags=-s -w", "-o", binary, ".")
	check(base, "")
	cross := options
	otherOS := "linux"
	if runtime.GOOS == otherOS {
		otherOS = "darwin"
	}
	cross.Load.Env = append(append([]string(nil), options.Load.Env...), "GOOS="+otherOS)
	check(fingerprintCompile(t, cross).Bundle.Snapshot().Profile, "GOOS")
	run("go", "build", "-tags=runtime_extra", "-trimpath", "-ldflags=-s -w", "-o", binary, ".")
	check(base, "-tags")
	options.Load.BuildFlags = []string{"-tags=beta,alpha,beta"}
	tagged := fingerprintCompile(t, options).Bundle.Snapshot().Profile
	run("go", "build", "-tags=alpha,beta", "-o", binary, ".")
	check(tagged, "")
	check(base, "-tags")
}
