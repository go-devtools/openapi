//go:build unix

package checkio_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/go-devtools/openapi/checkio"
)

// Reject a FIFO without opening it, while allowing explicitly selected regular symlink targets.
func TestNamedPipeAndExplicitSymlink(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(pipe, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := checkio.ReadFile(context.Background(), pipe, 64); !errors.Is(err, checkio.ErrNotRegular) {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "selected")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	if got, err := checkio.ReadFile(context.Background(), link, 2); string(got) != "{}" || err != nil {
		t.Fatal(string(got), err)
	}
}
