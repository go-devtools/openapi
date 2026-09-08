package checkio_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-devtools/openapi/checkio"
)

// Verify actual local bytes, exact boundaries, public error identities, and cancellation precedence.
func TestReadFileBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.json")
	raw := []byte("{\"name\":\"星\"}\n")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := checkio.ReadFile(context.Background(), path, len(raw))
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal(string(got), err)
	}
	for _, limit := range []int{-1, 0, len(raw) - 1} {
		if got, err := checkio.ReadFile(context.Background(), path, limit); got != nil || !errors.Is(err, checkio.ErrByteLimit) {
			t.Fatal(limit, got, err)
		}
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if got, err := checkio.ReadFile(context.Background(), path, 0); len(got) != 0 || err != nil {
		t.Fatal(got, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := checkio.ReadFile(canceled, path+".missing", -1); got != nil || !errors.Is(err, context.Canceled) {
		t.Fatal(got, err)
	}
}

// Reject device/directory inputs and oversized sparse files without materializing their contents.
func TestReadFileKindsAndLargeInput(t *testing.T) {
	for _, path := range []string{t.TempDir(), os.DevNull} {
		if _, err := checkio.ReadFile(context.Background(), path, 8<<20); !errors.Is(err, checkio.ErrNotRegular) {
			t.Fatal(path, err)
		}
	}
	path := filepath.Join(t.TempDir(), "large.json")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(1 << 30); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := checkio.ReadFile(context.Background(), path, 8<<20); got != nil || !errors.Is(err, checkio.ErrByteLimit) {
		t.Fatal(len(got), err)
	}
}
