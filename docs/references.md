# 离线引用与资源预算

核心支持 OpenAPI 3.2 文档基准 `$self`、Schema 资源 `$id`、`$anchor`、`$dynamicAnchor`、`$ref` 和 `$dynamicRef` 的初始目标解析。规则依据 [OpenAPI 3.2 的基准 URI 与引用说明](https://spec.openapis.org/oas/v3.2.0.html#appendix-f-examples-of-base-uri-determination-and-reference-resolution)及 [JSON Schema 2020-12 Core](https://json-schema.org/draft/2020-12/json-schema-core)。

## Go 入口

`openapi.Check(data)` 采用默认配置。`openapi.CheckWithOptions(data, options)` 接收显式离线内容，返回同一种 `Report`。检查器不调用文件、HTTP、DNS 或重定向加载器。

```go
package main

import (
    "fmt"

    "github.com/openapi-golang/openapi"
)

// 用内存中明确提供的文档和 Schema 进行离线检查。
// Checks only documents and schemas explicitly provided in memory.
func main() {
    document := []byte(`{
      "openapi":"3.2.0",
      "$self":"https://example.test/api/openapi.json",
      "info":{"title":"Offline example","version":"1"},
      "components":{"schemas":{"Item":{"$ref":"schemas/item"}}}
    }`)
    options := openapi.CheckOptions{
        Resources: map[string][]byte{
            "https://example.test/api/schemas/item": []byte(`{
              "type":"object",
              "properties":{"name":{"type":"string"}}
            }`),
        },
    }
    report := openapi.CheckWithOptions(document, options)
    if report.HasErrors() {
        panic(report)
    }
    fmt.Println("valid")
}
```

| 字段 | 行为 |
| --- | --- |
| `BaseURI` | 主文档的绝对检索 URI；不能含片段。省略时采用仅在本次检查内使用的内部默认地址。 |
| `Resources` | 绝对检索 URI 到完整 JSON 文档字节的映射。根含 `openapi` 时按 OpenAPI 文档检查；其他对象或布尔值按 JSON Schema 检查。 |
| `ExampleResources` | `externalValue` 对应的原始示例字节。仅证明内容已明确提供，不把其中的 `$ref` 当作加载指令。 |
| `MaxBytes` | 主文档和全部预载内容的累计字节上限，默认 8 MiB。 |
| `MaxResources` | 主文档、预载内容和内嵌 `$id` 创建的资源总数，默认 64。同一资源的检索 URI 与规范自声明 URI 只计一次。 |
| `MaxReferences` | 规范中的引用次数上限，默认 10000；重复文本的引用仍逐处计数和报告。 |
| `MaxIndexBytes` | 索引路径、资源 URI、引用解析和诊断的累计文本处理量，默认 16 MiB；这不是堆分配字节的精确测量。 |

预算零值采用默认值，负值返回 `openapi.spec.options`。全部 JSON 内容共用 200000 个节点预算，每份 JSON 深度不超过 128；重复键和尾随第二个 JSON 值会被拒绝。超出资源、引用或输入预算时返回 `openapi.spec.budget`，不会将截断结果标为完整。

输入字节和映射在调用期间只读使用，调用方不得并发修改。检查返回后不保留这些输入；独立调用之间没有共享的可变资源图。

## 引用规则

相对 `$self` 相对于调用方提供的检索 URI 解析。Schema 中的相对 `$id` 相对于最近的基准解析；它定义新的资源边界，其子节点继承这个基准。`$id` 不允许非空片段。锚点属于所在资源，不能借用另一个资源的同名锚点。

JSON Pointer 会先进行 URI 片段解码，再严格处理 `~0`、`~1` 和数组索引。引用必须指向对应的标准对象种类；指向 `info` 的 Schema 引用不会仅因目标存在而通过。指针跨过目标 Schema 的 `$id` 时，返回 `openapi.spec.ref.scope`，应改用最近 `$id` 对应的 URI。

discriminator 的组件名称映射会直接识别该组件；目标有 `$id` 时使用它的资源身份，不把名称映射误判为跨资源的 JSON Pointer。

循环引用按有限的边检查，不展开递归 Schema。`$dynamicRef` 的检查只证明初始目标存在且种类正确；实例验证时的动态作用域仍由 JSON Schema 验证器执行。此入口不是实例验证器，也不证明业务 handler 实施了声明约束。

所有明确预载的规范文档都会接受结构与引用检查，包括未被主文档引用的预载条目。检索地址、自声明 URI 或锚点冲突都会报告错误。外部示例仅检查资源提供性；示例是否符合媒体类型及实例 Schema 需要另行验证。

资源根目前支持完整 OpenAPI 对象和 JSON Schema 对象/布尔值。单独的 Response Object 等碎片不能被猜成完整规范；应把它们保留在完整 OpenAPI 文档内，通过 JSON Pointer 引用。

## 构建与裁剪

`openapi.Config.Validation` 接收同一个 `CheckOptions`。`Build` 在裁剪和最终检查中使用同一份配置。裁剪按 `$id`、锚点、普通引用、`$dynamicRef` 初始目标及 discriminator 映射计算模型闭包；引用组件内部任意子节点时保留整个组件。扩展、默认值和示例数据里的 `$ref` 不会保留无关组件。

外部预载文档回指本地组件时，该组件会进入闭包。只被未选中本地模型引用的缺失资源不会污染最终文档；资源身份冲突和解析预算等全局问题仍会阻断构建。

这些选项只配置检查，不修改或内嵌外部资源，不自动把预载内容写入 `Document.JSON()`，也不会发布任何 URL。当前 Gin Mount 尚未把预载资源映射为本地 UI 资源；不能把此处的离线解析通过当作外部引用的 Swagger UI 展示已经验收。

## 命令行

```sh
openapi check --spec openapi.json \
  --base-uri https://example.test/api/openapi.json \
  --resources resources.json \
  --max-bytes 8388608 --max-resources 64 --max-references 10000 \
  --max-index-bytes 16777216
```

清单内容：

```json
[
  {
    "uri": "https://example.test/api/schemas/item",
    "file": "schemas/item.json"
  },
  {
    "uri": "https://example.test/api/examples/message.txt",
    "file": "examples/message.txt",
    "kind": "example"
  }
]
```

`kind` 省略或为 `document` 时对应 `Resources`；`example` 对应 `ExampleResources`。相对文件路径以清单所在目录为基准。命令只读取显式指定的普通文件；不会把规范里的 URI 转换为本地路径。清单自身最多 1 MiB，主文档与预载文件共用 `--max-bytes` 预算。

命令拒绝缺失文件、重复 URI、未知字段或种类和尾随 JSON 值。读取前与分块读取间检查取消状态。检查成功退出 0，规范或资源错误退出非零，并输出 JSON 诊断；`check --help` 正常退出 0。
