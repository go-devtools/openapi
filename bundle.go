package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/openapi-golang/openapi/spec"
)

// Identify a source template independently of the final operationId.
type OperationKey string

// Declare the supported Bundle protocol version.
const BundleFormatVersion = 1

// Record effective module selections without serializing local replacement directories.
type ModuleProfile struct {
	Path           string `json:"path"`
	Version        string `json:"version,omitempty"`
	GoVersion      string `json:"goVersion,omitempty"`
	Main           bool   `json:"main,omitempty"`
	ReplacePath    string `json:"replacePath,omitempty"`
	ReplaceVersion string `json:"replaceVersion,omitempty"`
	GoModDigest    string `json:"goModDigest,omitempty"`
}

// Store actual load targets, versions, and reproducible settings without claiming runtime source equivalence.
type BuildProfile struct {
	CGOEnabled       string `json:"cgoEnabled,omitempty"`
	GoExperiment     string `json:"goExperiment,omitempty"`
	BuildFlagsDigest string `json:"buildFlagsDigest,omitempty"`
	// Empty map values record known default selectors; absent keys represent unknown values.
	Settings            map[string]string `json:"settings,omitempty"`
	Modules             []ModuleProfile   `json:"modules,omitempty"`
	Workspace           bool              `json:"workspace,omitempty"`
	ConfigurationDigest string            `json:"configurationDigest,omitempty"`
	Codecs              []string          `json:"codecs,omitempty"`
	GoVersion           string            `json:"goVersion,omitempty"`
	GOOS                string            `json:"goos,omitempty"`
	GOARCH              string            `json:"goarch,omitempty"`
	Tags                []string          `json:"tags,omitempty"`
	Codec               string            `json:"codec,omitempty"`
	Generator           string            `json:"generator,omitempty"`
	Frontend            string            `json:"frontend,omitempty"`
}

// Store a source template and its framework-neutral contract.
type Template struct {
	// Select finite conditional variants when linking routes.
	Variants       []OperationVariant `json:"variants,omitempty"`
	Key            OperationKey       `json:"key"`
	Symbol         string             `json:"symbol"`
	RuntimeSymbols []string           `json:"runtimeSymbols,omitempty"`
	Operation      spec.Operation     `json:"operation"`
	Source         Source             `json:"source,omitempty"`
	Diagnostics    []Diagnostic       `json:"diagnostics,omitempty"`
	Facts          []Source           `json:"facts,omitempty"`
}

// Define the public Bundle exchange format; construction saves an immutable copy.
type BundleData struct {
	FormatVersion int             `json:"formatVersion"`
	SpecVersion   string          `json:"specVersion"`
	Capabilities  []string        `json:"capabilities,omitempty"`
	Templates     []Template      `json:"templates"`
	Components    spec.Components `json:"components"`
	Profile       BuildProfile    `json:"profile"`
	Fingerprint   string          `json:"fingerprint,omitempty"`
	Diagnostics   []Diagnostic    `json:"diagnostics,omitempty"`
}

// Store an immutable contract snapshot that can serve multiple framework instances.
type Bundle struct {
	data      string
	loadError string
}

// Validate the protocol, keys, and capabilities before saving a deterministic snapshot.
func NewBundle(data BundleData) (Bundle, error) {
	if data.FormatVersion != BundleFormatVersion || data.SpecVersion != "3.2.0" {
		return Bundle{}, fmt.Errorf("openapi.bundle.incompatible: unsupported format %d / specification %s", data.FormatVersion, data.SpecVersion)
	}
	for _, cap := range data.Capabilities {
		if cap != "schema2020-12" && cap != "oas32" && cap != RequestConditionsCapability {
			return Bundle{}, fmt.Errorf("openapi.bundle.capability: unknown required capability %s", cap)
		}
	}
	keys := map[OperationKey]bool{}
	for _, t := range data.Templates {
		if t.Key == "" || keys[t.Key] {
			return Bundle{}, fmt.Errorf("openapi.bundle.key: template key is empty or duplicated: %s", t.Key)
		}
		keys[t.Key] = true
		if len(t.Variants) > 0 && !stringMember(data.Capabilities, RequestConditionsCapability) {
			return Bundle{}, fmt.Errorf("openapi.bundle.capability: conditional variants require a declared capability")
		}
		if len(t.Variants) > 1024 {
			return Bundle{}, fmt.Errorf("openapi.condition.budget: template variants exceed the limit")
		}
		for _, variant := range t.Variants {
			_, ok, err := normalizeCondition(variant.When)
			if err != nil {
				return Bundle{}, err
			}
			if !ok {
				return Bundle{}, fmt.Errorf("openapi.condition.empty: conditional variant is unreachable")
			}
		}
	}
	// Detach collections before sorting to preserve caller-owned data.
	raw, err := json.Marshal(data)
	if err != nil {
		return Bundle{}, err
	}
	var detached BundleData
	if err = json.Unmarshal(raw, &detached); err != nil {
		return Bundle{}, err
	}
	sort.Slice(detached.Templates, func(i, j int) bool { return detached.Templates[i].Key < detached.Templates[j].Key })
	sort.Strings(detached.Capabilities)
	for i := range detached.Templates {
		for j := range detached.Templates[i].Variants {
			normalized, _, _ := normalizeCondition(detached.Templates[i].Variants[j].When)
			detached.Templates[i].Variants[j].When = normalized
		}
	}
	raw, err = json.Marshal(detached)
	return Bundle{data: string(raw)}, err
}

// Strictly decode Bundle fields and protocol versions.
func ParseBundle(raw []byte) (Bundle, error) {
	if len(raw) > 16<<20 {
		return Bundle{}, fmt.Errorf("openapi.bundle.budget: Bundle exceeds the size limit")
	}
	var data BundleData
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return Bundle{}, err
	}
	if dec.InputOffset() != int64(len(bytes.TrimSpace(raw))) {
		return Bundle{}, fmt.Errorf("openapi.bundle.trailing: additional JSON content exists")
	}
	return NewBundle(data)
}

// Return an independent snapshot without exposing internal packages.
func (b Bundle) Snapshot() BundleData {
	var data BundleData
	_ = json.Unmarshal([]byte(b.data), &data)
	return data
}

// Return an independent encoded copy.
func (b Bundle) JSON() []byte { return []byte(b.data) }

// Return a copied template index with runtime matching evidence.
func (b Bundle) Index() []Template { return b.Snapshot().Templates }

// Preserve generated-data errors for startup validation instead of panicking.
func GeneratedBundle(raw string) Bundle {
	b, err := ParseBundle([]byte(raw))
	if err != nil {
		return Bundle{loadError: err.Error()}
	}
	return b
}

// Validate generated text before adapters match routes.
func (b Bundle) Validate() error {
	if b.loadError != "" {
		return fmt.Errorf("openapi.bundle.invalid: %s", b.loadError)
	}
	_, err := ParseBundle(b.JSON())
	return err
}
