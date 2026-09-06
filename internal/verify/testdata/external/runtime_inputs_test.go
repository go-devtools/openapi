package consumer

import "testing"

// Preserve known empty selectors so runtime checks distinguish default builds from missing metadata.
func TestRuntimeProfileRecordsDefaultSelectors(t *testing.T) {
	dir := t.TempDir()
	fingerprintFile(t, dir, "go.mod", "module example.test/runtimeinputs\n\ngo 1.27.1\n")
	fingerprintFile(t, dir, "app.go", "package runtimeinputs\nfunc H() int { return 1 }\n")
	profile := fingerprintCompile(t, fingerprintOptions(dir)).Bundle.Snapshot().Profile
	for _, key := range []string{"-tags", "GOEXPERIMENT"} {
		if value, known := profile.Settings[key]; !known || value != "" {
			t.Fatalf("default %s is not recorded: %+v", key, profile)
		}
	}
}
