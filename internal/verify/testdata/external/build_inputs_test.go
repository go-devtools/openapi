package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	. "github.com/openapi-golang/openapi/compiler"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Write independent source inputs without executing their business functions.
func fingerprintFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// Expose contracts and freshness of real load inputs through a minimal return-value frontend.
func fingerprintOptions(root string) Options {
	return Options{Load: LoadOptions{Dir: root, Env: []string{"GOWORK=off", "GOFLAGS=", "GOOS=linux", "GOARCH=amd64", "GOAMD64=v1", "CGO_ENABLED=0", "GOEXPERIMENT="}}, Frontends: []Frontend{{
		Name: "fingerprint-test-v1", Match: func(f Function) bool { return f.Object.Name() == "H" },
		Return: func(c ReturnContext) ([]Effect, error) {
			return []Effect{{Kind: ResponseBody, Status: "200", MediaType: "application/json", Payload: c.Values[0], Source: c.Source}}, nil
		},
	}}}
}

// Reload actual source each time; a load error is not an acceptable stale result.
func fingerprintCompile(t *testing.T, options Options) *Result {
	t.Helper()
	result, err := Compile(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// Record the load target instead of copying the platform executing the generator.
func TestFingerprintTargetProfile(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/profile\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package profile\nfunc H() int { return 1 }\n")
	options := fingerprintOptions(dir)
	base := fingerprintCompile(t, options)
	options.Load.Env = append(options.Load.Env, "GOARCH=386", "GO386=softfloat", "GOEXPERIMENT=loopvar", "GOFLAGS=-tags=alpha,beta")
	changed := fingerprintCompile(t, options)
	profile := changed.Bundle.Snapshot().Profile
	if profile.GOOS != "linux" || profile.GOARCH != "386" {
		t.Fatalf("wrong target profile: %+v", profile)
	}
	raw, _ := json.Marshal(profile)
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if data["cgoEnabled"] != "0" || data["goExperiment"] != "loopvar" {
		t.Fatalf("missing build selectors: %s", raw)
	}
	if base.Bundle.Snapshot().Fingerprint == changed.Bundle.Snapshot().Fingerprint {
		t.Fatal("target/build changes did not affect freshness")
	}
}

// Include overlay comments and files absent on disk instead of rereading stale disk bytes.
func TestFingerprintOverlayInputs(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/overlay\n\ngo 1.27.1\n")
	source := "package overlay\n\n// Original description.\nfunc H() int { return 1 }\n"
	fingerprintFile(t, dir, "app.go", source)
	options := fingerprintOptions(dir)
	base := fingerprintCompile(t, options)
	options.Load.Overlay = map[string][]byte{filepath.Join(dir, "app.go"): []byte(strings.ReplaceAll(source, "Original", "Overlay"))}
	changed := fingerprintCompile(t, options)
	if base.Bundle.Snapshot().Fingerprint == changed.Bundle.Snapshot().Fingerprint {
		t.Fatal("overlay comment was absent from fingerprint")
	}
	if !strings.Contains(string(changed.Bundle.JSON()), "Overlay description") {
		t.Fatal("overlay was not analyzed")
	}
	options.Load.Overlay[filepath.Join(dir, "extra.go")] = []byte("package overlay\ntype Additional string\n")
	added := fingerprintCompile(t, options)
	if added.Bundle.Snapshot().Fingerprint == changed.Bundle.Snapshot().Fingerprint {
		t.Fatal("new overlay file was not fingerprinted")
	}
	if _, err := os.Stat(filepath.Join(dir, "extra.go")); !os.IsNotExist(err) {
		t.Fatal("overlay was written to business source")
	}
}

// Same-named files belong to different packages, so swapping implementations must change input identity.
func TestFingerprintPackageFileIdentity(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/identity\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package identity\nimport (_ \"example.test/identity/a\"; _ \"example.test/identity/b\")\nfunc H() int { return 1 }\n")
	left, right := "package model\nconst Value=1\n", "package model\nconst Value=2\n"
	fingerprintFile(t, dir, "a/value.go", left)
	fingerprintFile(t, dir, "b/value.go", right)
	options := fingerprintOptions(dir)
	first := fingerprintCompile(t, options)
	fingerprintFile(t, dir, "a/value.go", right)
	fingerprintFile(t, dir, "b/value.go", left)
	second := fingerprintCompile(t, options)
	if first.Bundle.Snapshot().Fingerprint == second.Bundle.Snapshot().Fingerprint {
		t.Fatal("package identities were erased before hashing")
	}
}

// Include inactive source and module selections as analysis inputs.
func TestFingerprintInactiveAndModuleInputs(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	dep := filepath.Join(dir, "dep")
	fingerprintFile(t, dep, "go.mod", "module example.test/dependency\n\ngo 1.27.1\n")
	fingerprintFile(t, dep, "value.go", "package dependency\nconst Value=1\n")
	mod := "module example.test/inputs\n\ngo 1.27.1\nrequire example.test/dependency v1.0.0\nreplace example.test/dependency => ../dep\n"
	fingerprintFile(t, app, "go.mod", mod)
	fingerprintFile(t, app, "app.go", "package inputs\nimport _ \"example.test/dependency\"\nfunc H() int { return 1 }\n")
	fingerprintFile(t, app, "inactive.go", "//go:build future\n\npackage inputs\n\n// Initial future type.\ntype Future int\n")
	options := fingerprintOptions(app)
	base := fingerprintCompile(t, options)
	fingerprintFile(t, app, "inactive.go", "//go:build future\n\npackage inputs\n\n// Changed future type.\ntype Future int\n")
	changed := fingerprintCompile(t, options)
	if base.Bundle.Snapshot().Fingerprint == changed.Bundle.Snapshot().Fingerprint {
		t.Error("inactive source was absent from fingerprint")
	}
	fingerprintFile(t, app, "go.mod", strings.Replace(mod, "v1.0.0", "v1.1.0", 1))
	versioned := fingerprintCompile(t, options)
	if changed.Bundle.Snapshot().Fingerprint == versioned.Bundle.Snapshot().Fingerprint {
		t.Error("effective module version was absent from fingerprint")
	}
}

// Relocated identical projects must generate identical bytes while module files remain read-only.
func TestFingerprintRelocationAndReadOnly(t *testing.T) {
	var bundles [][]byte
	for i := 0; i < 2; i++ {
		dir := t.TempDir()
		app := filepath.Join(dir, "app")
		dep := filepath.Join(dir, "local")
		fingerprintFile(t, dep, "go.mod", "module example.test/local\n\ngo 1.27.1\n")
		fingerprintFile(t, dep, "local.go", "package local\nconst Value=1\n")
		mod := "module example.test/relocation\n\ngo 1.27.1\nrequire example.test/local v1.0.0\nreplace example.test/local => " + filepath.ToSlash(dep) + "\n"
		fingerprintFile(t, app, "go.mod", mod)
		fingerprintFile(t, app, "app.go", "package relocation\nimport _ \"example.test/local\"\nfunc H() int { return 1 }\n")
		result := fingerprintCompile(t, fingerprintOptions(app))
		bundles = append(bundles, result.Bundle.JSON())
		if bytes.Contains(result.Bundle.JSON(), []byte(dir)) {
			t.Fatal("machine path leaked into bundle")
		}
		raw, err := os.ReadFile(filepath.Join(app, "go.mod"))
		if err != nil || string(raw) != mod {
			t.Fatal("loading changed go.mod")
		}
		if _, err = os.Stat(filepath.Join(app, "go.sum")); !os.IsNotExist(err) {
			t.Fatal("loading created go.sum")
		}
	}
	if !bytes.Equal(bundles[0], bundles[1]) {
		t.Fatal("checkout relocation changed generated bytes")
	}
}
