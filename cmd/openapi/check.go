package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/openapi-golang/openapi"
	"github.com/openapi-golang/openapi/checkio"
)

// Explicitly map retrieval URIs to local files; never load files from URIs found in the specification.
type resourceFile struct {
	URI  string `json:"uri"`
	File string `json:"file"`
	Kind string `json:"kind,omitempty"`
}

// Share the byte budget across the root and manifest entries; limit the manifest itself to one MiB.
func readCheckInputs(ctx context.Context, specFile, manifestFile string, options openapi.CheckOptions) ([]byte, openapi.CheckOptions, error) {
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 0 {
		return nil, options, fmt.Errorf("openapi.cli.budget: all budgets must be greater than zero")
	}
	raw, err := checkio.ReadFile(ctx, specFile, options.MaxBytes)
	if err != nil {
		return nil, options, err
	}
	return readResourceManifest(ctx, raw, manifestFile, options)
}

// Apply shared read budgets to explicit manifests for document checks and schema exports.
func readResourceManifest(ctx context.Context, raw []byte, manifestFile string, options openapi.CheckOptions) ([]byte, openapi.CheckOptions, error) {
	if options.MaxBytes < 1 || options.MaxResources < 1 || options.MaxReferences < 1 || options.MaxIndexBytes < 0 {
		return nil, options, fmt.Errorf("openapi.cli.budget: all budgets must be greater than zero")
	}
	if manifestFile == "" {
		return raw, options, nil
	}
	manifest, err := checkio.ReadFile(ctx, manifestFile, 1<<20)
	if err != nil {
		return nil, options, err
	}
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	var entries []resourceFile
	if err = decoder.Decode(&entries); err != nil {
		return nil, options, fmt.Errorf("openapi.cli.resources: %w", err)
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return nil, options, fmt.Errorf("openapi.cli.resources: manifest must contain exactly one JSON array")
	}
	if len(entries) >= options.MaxResources {
		return nil, options, fmt.Errorf("openapi.cli.budget: manifest and main document exceed the resource count budget")
	}
	remaining := options.MaxBytes - len(raw)
	options.Resources = map[string][]byte{}
	options.ExampleResources = map[string][]byte{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.URI == "" || entry.File == "" {
			return nil, options, fmt.Errorf("openapi.cli.resources: entries must specify uri and file")
		}
		if entry.Kind != "" && entry.Kind != "document" && entry.Kind != "example" {
			return nil, options, fmt.Errorf("openapi.cli.resources: kind must be document or example")
		}
		if seen[entry.URI] {
			return nil, options, fmt.Errorf("openapi.cli.resources: duplicate retrieval URI: %s", entry.URI)
		}
		seen[entry.URI] = true
		path := entry.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(manifestFile), path)
		}
		data, err := checkio.ReadFile(ctx, path, remaining)
		if err != nil {
			return nil, options, err
		}
		remaining -= len(data)
		if entry.Kind == "example" {
			options.ExampleResources[entry.URI] = data
		} else {
			options.Resources[entry.URI] = data
		}
	}
	return raw, options, nil
}
