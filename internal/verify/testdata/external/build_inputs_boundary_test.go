package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	. "github.com/go-devtools/openapi/compiler"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-devtools/openapi/spec"
)

// Change each build selector independently so one change cannot hide another missing input.
func TestFingerprintIndependentSelectors(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/selectors\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package selectors\nfunc H() int { return 1 }\n")
	base := fingerprintCompile(t, fingerprintOptions(dir)).Bundle.Snapshot().Fingerprint
	for _, setting := range []string{"CGO_ENABLED=1", "GOEXPERIMENT=loopvar", "GOFLAGS=-tags=custom", "GOAMD64=v2"} {
		t.Run(setting, func(t *testing.T) {
			options := fingerprintOptions(dir)
			options.Load.Env = append(options.Load.Env, setting)
			if fingerprintCompile(t, options).Bundle.Snapshot().Fingerprint == base {
				t.Fatal("build selector was omitted")
			}
		})
	}
}

// Embedded-resource changes invalidate output while unrelated output files do not enter the fingerprint.
func TestFingerprintEmbeddedResources(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/resources\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package resources\nimport _ \"embed\"\n//go:embed payload.txt\nvar payload string\nfunc H() int { return 1 }\n")
	fingerprintFile(t, dir, "payload.txt", "first")
	options := fingerprintOptions(dir)
	base := fingerprintCompile(t, options)
	fingerprintFile(t, dir, "unused.txt", "unrelated output")
	if fingerprintCompile(t, options).Bundle.Snapshot().Fingerprint != base.Bundle.Snapshot().Fingerprint {
		t.Fatal("unrelated output changed freshness")
	}
	fingerprintFile(t, dir, "payload.txt", "second")
	if fingerprintCompile(t, options).Bundle.Snapshot().Fingerprint == base.Bundle.Snapshot().Fingerprint {
		t.Fatal("embedded resource did not change freshness")
	}
}

// Include workspace dependency source without putting absolute use paths into generated output.
func TestFingerprintWorkspaceSource(t *testing.T) {
	var initial [][]byte
	for i := 0; i < 2; i++ {
		dir := t.TempDir()
		app := filepath.Join(dir, "app")
		dep := filepath.Join(dir, "dep")
		fingerprintFile(t, app, "go.mod", "module example.test/workapp\n\ngo 1.27.1\n")
		fingerprintFile(t, app, "app.go", "package workapp\nimport _ \"example.test/workdep\"\nfunc H() int { return 1 }\n")
		fingerprintFile(t, dep, "go.mod", "module example.test/workdep\n\ngo 1.27.1\n")
		fingerprintFile(t, dep, "dep.go", "package workdep\nconst Value=1\n")
		fingerprintFile(t, dir, "workspace.work", "go 1.27.1\nuse (\n"+filepath.ToSlash(app)+"\n"+filepath.ToSlash(dep)+"\n)\n")
		options := fingerprintOptions(app)
		options.Load.Env = append(options.Load.Env, "GOWORK="+filepath.Join(dir, "workspace.work"))
		base := fingerprintCompile(t, options)
		initial = append(initial, base.Bundle.JSON())
		if !base.Bundle.Snapshot().Profile.Workspace || bytes.Contains(base.Bundle.JSON(), []byte(dir)) {
			t.Fatal("wrong or machine-local workspace profile")
		}
		fingerprintFile(t, dep, "dep.go", "package workdep\nconst Value=2\n")
		if fingerprintCompile(t, options).Bundle.Snapshot().Fingerprint == base.Bundle.Snapshot().Fingerprint {
			t.Fatal("local workspace dependency was omitted")
		}
	}
	if !bytes.Equal(initial[0], initial[1]) {
		t.Fatal("workspace relocation changed output")
	}
}

// Require declared configuration for custom mappings and expose only its digest in the public profile.
func TestFingerprintCustomConfiguration(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/configured\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package configured\nfunc H() int { return 1 }\n")
	options := fingerprintOptions(dir)
	options.Mappers = []TypeMapper{func(ProjectionRequest) (*spec.Schema, bool, error) { return nil, false, nil }}
	if _, err := Compile(context.Background(), options); err == nil {
		t.Fatal("unidentified custom mapper configuration was accepted")
	}
	options.Configuration = map[string]json.RawMessage{"custom-decoding": json.RawMessage(`{"version":"first-private-setting"}`)}
	first := fingerprintCompile(t, options)
	options.Configuration["custom-decoding"] = json.RawMessage(`{"version":"second-private-setting"}`)
	second := fingerprintCompile(t, options)
	if first.Bundle.Snapshot().Fingerprint == second.Bundle.Snapshot().Fingerprint {
		t.Fatal("declared mapping input was omitted")
	}
	if bytes.Contains(first.Bundle.JSON(), []byte("first-private-setting")) {
		t.Fatal("raw configuration leaked into the bundle")
	}
}

// Prevent executable drivers and writable module modes from escaping read-only static analysis.
func TestLoadRejectsExecutableDriversAndWritableModes(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/readonly\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package readonly\nfunc H() int { return 1 }\n")
	for _, flags := range [][]string{{"-mod=mod"}, {"-mod", "mod"}, {"-toolexec=untrusted-command"}} {
		options := fingerprintOptions(dir)
		options.Load.BuildFlags = flags
		if _, err := Compile(context.Background(), options); err == nil || !strings.Contains(err.Error(), "openapi.load.flags") {
			t.Errorf("unsafe mode did not fail before loading: %v", err)
		}
	}
	marker := filepath.Join(dir, "executed")
	driver := filepath.Join(dir, "driver.sh")
	fingerprintFile(t, dir, "driver.sh", "#!/bin/sh\nprintf invoked > '"+marker+"'\nprintf '{}'\n")
	if err := os.Chmod(driver, 0700); err != nil {
		t.Fatal(err)
	}
	options := fingerprintOptions(dir)
	options.Load.Env = append(options.Load.Env, "GOPACKAGESDRIVER="+driver)
	if _, err := Compile(context.Background(), options); err == nil || !strings.Contains(err.Error(), "openapi.load.driver") {
		t.Errorf("driver was not explicitly rejected: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("package loading executed the project driver")
	}
}

// Fail on input budget exhaustion instead of publishing a complete contract with unaccounted inputs.
func TestFingerprintInputBudget(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/budget\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package budget\nfunc H() int { return 1 }\n")
	options := fingerprintOptions(dir)
	options.Load.MaxSourceBytes = 16
	if _, err := Compile(context.Background(), options); err == nil || !strings.Contains(err.Error(), "openapi.load.input-budget") {
		t.Fatalf("missing budget diagnostic: %v", err)
	}
}

// Apply Go's last-entry-wins environment rule so an explicitly disabled driver permits loading.
func TestLoadDriverEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/override\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package override\nfunc H() int { return 1 }\n")
	options := fingerprintOptions(dir)
	options.Load.Env = append(options.Load.Env, "GOPACKAGESDRIVER=unused-driver", "GOPACKAGESDRIVER=off")
	fingerprintCompile(t, options)
}

// Linker flags may contain deployment settings; they must affect freshness without exposing values in output.
func TestFingerprintFlagsDoNotExposeValues(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/flags\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package flags\nfunc H() int { return 1 }\n")
	options := fingerprintOptions(dir)
	base := fingerprintCompile(t, options)
	options.Load.BuildFlags = []string{"-ldflags=-X example.test/flags.setting=deliberate-private-marker"}
	changed := fingerprintCompile(t, options)
	if bytes.Contains(changed.Bundle.JSON(), []byte("deliberate-private-marker")) {
		t.Fatal("linker configuration leaked into the bundle")
	}
	if base.Bundle.Snapshot().Fingerprint == changed.Bundle.Snapshot().Fingerprint {
		t.Fatal("linker input was absent from freshness")
	}
}

// Vendor mode must fingerprint selected vendor source instead of silently using module-cache source.
func TestFingerprintVendorSource(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/vendorapp\n\ngo 1.27.1\nrequire example.test/vendordep v1.0.0\n")
	fingerprintFile(t, dir, "app.go", "package vendorapp\nimport _ \"example.test/vendordep\"\nfunc H() int { return 1 }\n")
	fingerprintFile(t, dir, "vendor/modules.txt", "# example.test/vendordep v1.0.0\n## explicit; go 1.27.1\nexample.test/vendordep\n")
	fingerprintFile(t, dir, "vendor/example.test/vendordep/dep.go", "package vendordep\nconst Value=1\n")
	options := fingerprintOptions(dir)
	options.Load.BuildFlags = []string{"-mod=vendor"}
	first := fingerprintCompile(t, options)
	fingerprintFile(t, dir, "vendor/example.test/vendordep/dep.go", "package vendordep\nconst Value=2\n")
	second := fingerprintCompile(t, options)
	if first.Bundle.Snapshot().Fingerprint == second.Bundle.Snapshot().Fingerprint {
		t.Fatal("selected vendor source did not affect freshness")
	}
}
