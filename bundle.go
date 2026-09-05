package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/openapi-golang/openapi/spec"
)

// 表示稳定源码模板键，不等于最终 operationId。
type OperationKey string

// 声明当前 Bundle 协议范围。
const BundleFormatVersion = 1

// 保存生成器和构建条件，用于兼容与新鲜度检查。
type BuildProfile struct {
	GoVersion string   `json:"goVersion,omitempty"`
	GOOS      string   `json:"goos,omitempty"`
	GOARCH    string   `json:"goarch,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Codec     string   `json:"codec,omitempty"`
	Generator string   `json:"generator,omitempty"`
	Frontend  string   `json:"frontend,omitempty"`
}

// 保存可匹配的源码模板与框架中立契约。
type Template struct {
	Key            OperationKey   `json:"key"`
	Symbol         string         `json:"symbol"`
	RuntimeSymbols []string       `json:"runtimeSymbols,omitempty"`
	Operation      spec.Operation `json:"operation"`
	Source         Source         `json:"source,omitempty"`
	Diagnostics    []Diagnostic   `json:"diagnostics,omitempty"`
	Facts          []Source       `json:"facts,omitempty"`
}

// 表示版本化 Bundle 的公开交换格式，构造后由 Bundle 保存不可变副本。
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
type Bundle struct {
	data      string
	loadError string
}

// 校验协议、模板键与能力后，保存确定性快照。
func NewBundle(data BundleData) (Bundle, error) {
	if data.FormatVersion != BundleFormatVersion || data.SpecVersion != "3.2.0" {
		return Bundle{}, fmt.Errorf("openapi.bundle.incompatible: 不支持格式 %d / 规范 %s", data.FormatVersion, data.SpecVersion)
	}
	for _, cap := range data.Capabilities {
		if cap != "schema2020-12" && cap != "oas32" {
			return Bundle{}, fmt.Errorf("openapi.bundle.capability: 未知必需能力 %s", cap)
		}
	}
	keys := map[OperationKey]bool{}
	for _, t := range data.Templates {
		if t.Key == "" || keys[t.Key] {
			return Bundle{}, fmt.Errorf("openapi.bundle.key: 模板键为空或重复：%s", t.Key)
		}
		keys[t.Key] = true
	}
	// 先序列化隔离调用方集合，再排序，避免构造过程改变输入。
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
	raw, err = json.Marshal(detached)
	return Bundle{data: string(raw)}, err
}

// 严格读取 Bundle，拒绝不认识的顶层字段和协议版本。
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
func (b Bundle) Snapshot() BundleData {
	var data BundleData
	_ = json.Unmarshal([]byte(b.data), &data)
	return data
}

// 返回独立的编码副本。
func (b Bundle) JSON() []byte { return []byte(b.data) }

// 返回模板索引的独立副本，包含运行时匹配所需证据。
func (b Bundle) Index() []Template { return b.Snapshot().Templates }

// 从生成器固定文本创建 Bundle，错误保留到启动层返回而不触发 panic。
func GeneratedBundle(raw string) Bundle {
	b, err := ParseBundle([]byte(raw))
	if err != nil {
		return Bundle{loadError: err.Error()}
	}
	return b
}

// 检查静态生成文本和格式兼容性，适配器可在匹配路由前调用。
func (b Bundle) Validate() error {
	if b.loadError != "" {
		return fmt.Errorf("openapi.bundle.invalid: %s", b.loadError)
	}
	_, err := ParseBundle(b.JSON())
	return err
}
