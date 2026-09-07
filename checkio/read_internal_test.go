package checkio

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// Cancel inside a read to model an interrupt arriving between filesystem operations.
type cancelingReader struct{ cancel context.CancelFunc }

// Return partial input together with a cancellation signal from the source.
func (r cancelingReader) Read(p []byte) (int, error) { r.cancel(); return copy(p, "partial"), nil }

// Prevent partial input from being accepted when cancellation arrives during a read.
func TestCancellationDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data, err := io.ReadAll(contextReader{ctx: ctx, reader: cancelingReader{cancel: cancel}})
	if !errors.Is(err, context.Canceled) || string(data) != "partial" {
		t.Fatal(string(data), err)
	}
	reader := contextReader{ctx: ctx, reader: strings.NewReader("must not be read")}
	if n, err := reader.Read(make([]byte, 32)); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
}
