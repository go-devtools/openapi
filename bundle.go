package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/openapi-golang/openapi/spec"
)

// 表示稳定源码模板键，不等于最终 operationId。
// Identify a source template independently of the final operationId.
type OperationKey string

// 声明当前 Bundle 协议范围。
// Declare the supported Bundle protocol version.
const BundleFormatVersion = 1

// 记录有效模块选择，不序列化本地替换目录。
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

// 保存实际加载目标、版本和可重现配置；不是运行时代码一致性的证明。
// Store actual load targets, versions, and reproducible settings without claiming runtime source equivalence.
type BuildProfile struct {
	CGOEnabled          string            `json:"cgoEnabled,omitempty"`
	GoExperiment        string            `json:"goExperiment,omitempty"`
	BuildFlagsDigest    string            `json:"buildFlagsDigest,omitempty"`
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

// 保存可匹配的源码模板与框架中立契约。
// Store a source template and its framework-neutral contract.
type Template struct {
	// 有限条件变体在路由链接时选择。
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

// 表示版本化 Bundle 的公开交换格式，构造后由 Bundle 保存不可变副本。
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

// 保存不可变的契约快照，可以安全地同时供多个框架实例链接。
// Store an immutable contract snapshot that can serve multiple framework instances.
type Bundle struct {
	data      string
	loadError string
}

// 校验协议、模板键与能力后，保存确定性快照。
// Validate the protocol, keys, and capabilities before saving a deterministic snapshot.
func NewBundle(data BundleData) (Bundle, error) {
	if data.FormatVersion != BundleFormatVersion || data.SpecVersion != "3.2.0" {
		return Bundle{}, fmt.Errorf("openapi.bundle.incompatible: 不支持格式 %d / 规范 %s", data.FormatVersion, data.SpecVersion)
	}
	for _, cap := range data.Capabilities {
		if cap != "schema2020-12" && cap != "oas32" && cap != RequestConditionsCapability {
			return Bundle{}, fmt.Errorf("openapi.bundle.capability: 未知必需能力 %s", cap)
		}
	}
	keys := map[OperationKey]bool{}
	for _, t := range data.Templates {
		if t.Key == "" || keys[t.Key] {
			return Bundle{}, fmt.Errorf("openapi.bundle.key: 模板键为空或重复：%s", t.Key)
		}
		keys[t.Key] = true
		if len(t.Variants) > 0 && !stringMember(data.Capabilities, RequestConditionsCapability) {
			return Bundle{}, fmt.Errorf("openapi.bundle.capability: 条件变体缺少声明能力")
		}
		if len(t.Variants) > 1024 {
			return Bundle{}, fmt.Errorf("openapi.condition.budget: 模板变体超过限制")
		}
		for _, variant := range t.Variants {
			_, ok, err := normalizeCondition(variant.When)
			if err != nil {
				return Bundle{}, err
			}
			if !ok {
				return Bundle{}, fmt.Errorf("openapi.condition.empty: 条件变体不可达")
			}
		}
	}
	// 先序列化隔离调用方集合，再排序，避免构造过程改变输入。
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

// 严格读取 Bundle，拒绝不认识的顶层字段和协议版本。
// Strictly decode Bundle fields and protocol versions.
func ParseBundle(raw []byte) (Bundle, error) {
	if len(raw) > 16<<20 {
		return Bundle{}, fmt.Errorf("openapi.bundle.budget: Bundle 超过体积限制")
	}
	var data BundleData
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return Bundle{}, err
	}
	if dec.InputOffset() != int64(len(bytes.TrimSpace(raw))) {
		return Bundle{}, fmt.Errorf("openapi.bundle.trailing: 存在额外 JSON 内容")
	}
	return NewBundle(data)
}

// 返回可变的独立快照，适配器无需访问核心内部包。
// Return an independent snapshot without exposing internal packages.
func (b Bundle) Snapshot() BundleData {
	var data BundleData
	_ = json.Unmarshal([]byte(b.data), &data)
	return data
}

// 返回独立的编码副本。
// Return an independent encoded copy.
func (b Bundle) JSON() []byte { return []byte(b.data) }

// 返回模板索引的独立副本，包含运行时匹配所需证据。
// Return a copied template index with runtime matching evidence.
func (b Bundle) Index() []Template { return b.Snapshot().Templates }

// 从生成器固定文本创建 Bundle，错误保留到启动层返回而不触发 panic。
// Preserve generated-data errors for startup validation instead of panicking.
func GeneratedBundle(raw string) Bundle {
	b, err := ParseBundle([]byte(raw))
	if err != nil {
		return Bundle{loadError: err.Error()}
	}
	return b
}

// 检查静态生成文本和格式兼容性，适配器可在匹配路由前调用。
// Validate generated text before adapters match routes.
func (b Bundle) Validate() error {
	if b.loadError != "" {
		return fmt.Errorf("openapi.bundle.invalid: %s", b.loadError)
	}
	_, err := ParseBundle(b.JSON())
	return err
}
