// Package checkio reads explicitly selected local specification files under an input budget.
// It is optional: importing the root openapi package does not enable filesystem access.
package checkio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrByteLimit identifies an invalid or exhausted input byte budget.
var ErrByteLimit = errors.New("openapi.input.budget: byte budget exceeded")

// ErrNotRegular identifies an input that is not a regular file.
var ErrNotRegular = errors.New("openapi.input.file: only regular files are accepted")

// ReadFile reads a caller-selected regular file, including an explicitly selected symlink target.
// Zero permits only an empty file. At most maxBytes+1 bytes are read to detect concurrent growth.
// Cancellation is checked between filesystem operations and reads, not inside a kernel operation.
// No filename is inferred from a document URI, and no network resource is fetched.
func ReadFile(ctx context.Context, filename string, maxBytes int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes < 0 {
		return nil, fmt.Errorf("%w: limit must not be negative", ErrByteLimit)
	}
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if err = checkFile(info, maxBytes); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Recheck the opened descriptor: the named file may have changed after the initial stat.
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if err = checkFile(info, maxBytes); err != nil {
		return nil, err
	}
	reader := contextReader{ctx: ctx, reader: file}
	data, err := io.ReadAll(io.LimitReader(reader, int64(maxBytes)))
	if err != nil {
		return nil, err
	}
	var extra [1]byte
	n, err := reader.Read(extra[:])
	if n > 0 {
		return nil, fmt.Errorf("%w: file grew beyond the limit while being read", ErrByteLimit)
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// Check the file type and known size before reading any content.
func checkFile(info os.FileInfo, maxBytes int) error {
	if !info.Mode().IsRegular() {
		return ErrNotRegular
	}
	if info.Size() > int64(maxBytes) {
		return ErrByteLimit
	}
	return nil
}

// Check cancellation at the boundary of each regular-file read.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Preserve cancellation even when it arrives during the underlying read.
func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if canceled := r.ctx.Err(); canceled != nil {
		return n, canceled
	}
	return n, err
}
