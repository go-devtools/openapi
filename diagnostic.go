// Provide the framework-neutral runtime for compiled Go contracts.
package openapi

import (
	"encoding/json"
	"fmt"
	"github.com/openapi-golang/openapi/internal/validate"
)

// Identify diagnostic severity.
type Severity string

// Errors block construction; warnings and notes remain in the audit report.
const (
	Error   Severity = "error"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Record relative source locations without machine-specific paths.
type Source struct {
	// Preserve the finite request condition under which this fact applies.
	When   *RequestCondition `json:"when,omitempty"`
	File   string            `json:"file,omitempty"`
	Line   int               `json:"line,omitempty"`
	Column int               `json:"column,omitempty"`
	Symbol string            `json:"symbol,omitempty"`
	Rule   string            `json:"rule,omitempty"`
	Kind   string            `json:"kind,omitempty"`
}

// Describe a stable diagnostic with an explanation and remediation.
type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix,omitempty"`
	Source   Source   `json:"source,omitempty"`
	Route    string   `json:"route,omitempty"`
	Facts    []Source `json:"facts,omitempty"`
}

// Store facts and diagnostics with defensive-copy access.
type Report struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
	Facts       []Source     `json:"facts,omitempty"`
}

// Report whether any diagnostic blocks construction.
func (r Report) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Return a stable error summary while retaining full diagnostic details.
func (r Report) Error() string {
	for _, d := range r.Diagnostics {
		if d.Severity == Error {
			return fmt.Sprintf("%s: %s; %s", d.Code, d.Message, d.Fix)
		}
	}
	return "No blocking errors"
}

// Configure offline base URIs, explicit resources, and aggregate budgets; inputs remain read-only during the call.
type CheckOptions struct {
	// Set the root document's absolute retrieval URI or use the internal default for this check.
	BaseURI string
	// Preload complete OpenAPI or JSON Schema documents keyed by absolute, fragment-free retrieval URIs.
	Resources map[string][]byte
	// Preload raw externalValue example bytes without interpreting them as schemas.
	ExampleResources map[string][]byte
	// Limit aggregate bytes across the root and all preloaded contents; zero selects eight MiB.
	MaxBytes int
	// Limit root documents, preloaded entries, and embedded $id resources; zero selects sixty-four.
	MaxResources int
	// Limit reference occurrences across the specification; zero selects ten thousand.
	MaxReferences int
	// Limits cumulative index paths, URI resolution, and diagnostic text; zero uses sixteen MiB.
	MaxIndexBytes int
}

// Accept explicit offline options without automatically retrieving resources during validation.
func CheckWithOptions(data []byte, options CheckOptions) Report {
	return issuesReport(validate.CheckWithOptions(data, options.internal()))
}

// Translate public budgets into internal options without copying or modifying caller-owned contents.
func (o CheckOptions) internal() validate.Options {
	return validate.Options{BaseURI: o.BaseURI, Resources: o.Resources, ExampleResources: o.ExampleResources, MaxBytes: o.MaxBytes, MaxResources: o.MaxResources, MaxReferences: o.MaxReferences, MaxIndexBytes: o.MaxIndexBytes}
}

// Check structure and semantics offline without fetching references.
func Check(data []byte) Report {
	return CheckWithOptions(data, CheckOptions{})
}

// Convert internal diagnostics into public reports while preserving every error location.
func issuesReport(issues []validate.Issue) Report {
	r := Report{Diagnostics: []Diagnostic{}}
	for _, i := range issues {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: i.Code, Severity: Error, Message: i.Path + ": " + i.Message, Fix: i.Fix})
	}
	return r
}

// Copy serializable public data to isolate mutable collections.
func copyJSON[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out T
	if json.Unmarshal(b, &out) != nil {
		return v
	}
	return out
}
