# 独立契约验证

`contracttest` 是可选的框架无关测试包。它接收调用方提供的规范、资源和真实样本，不安装生产中间件，也不执行 handler 或业务序列化方法。实际 JSON Schema 断言由 `github.com/santhosh-tekuri/jsonschema/v6` 执行。

## 资源与指针

`Compile(document, pointer, Options)` 支持独立 JSON Schema 或完整 OpenAPI 文档。pointer 必须为空或标准绝对 JSON Pointer，并指向真正的 Schema 位置；不能选择 info、Response Object 或 examples 中的业务数据。空指针用于独立 Schema 根。

```go
validator, err := contracttest.Compile(document,
    "/components/schemas/Response",
    contracttest.Options{
        BaseURI: "https://example.test/api/openapi",
        Resources: map[string][]byte{
            "https://example.test/api/schemas/item": itemSchema,
        },
    })
if err != nil {
    return err
}
return validator.JSON(responseBody)
```

BaseURI 是检索地址；Resources 的键是明确提供的绝对检索 URI，值是对应 OpenAPI 或 JSON Schema 原文。默认基准为 `https://openapi.invalid/document.json`。这些 URI 不会自动触发网络或文件读取。编译结果不保留调用方的可变资源 map 或字节内容。

索引保留 `$self`、最近的 `$id`、检索地址别名、锚点与原始 JSON Pointer 作用域。OpenAPI 内的 Schema 整理为独立引擎认识的 `$defs`；数值不通过浮点数转换。只有真正 Schema 上的 `$ref` / `$dynamicRef` 被转换，const、default、examples、扩展及 discriminator 的映射名称仍是数据。

引用位置整理复用核心的中立资源索引；实例断言及 `$dynamicRef` 的动态作用域由独立引擎计算。这不等于有两套独立的引用索引实现，也不能替代完整 OpenAPI 文档检查。

## 动态引用与方言

动态锚点的片段名称保持不变，避免转换成 JSON Pointer 后退化为静态引用。真实样本验证了递归扩展：Strict 通过动态引用约束所有子节点，基础 Tree 和静态引用仍接受基础类型允许的字段。初始目标是普通锚点或空片段时保持静态行为。

包内固定并离线注册默认 OpenAPI 3.1 方言及固定 OpenAPI 3.2 方言；来源、SHA-256 和许可证见 [方言资源](../contracttest/dialects/PROVENANCE.md)。调用方也可以通过 Resources 提供自定义元 Schema；未知的必需词汇表会被拒绝，不会静默降级。format 和 content 断言分别由 AssertFormat / AssertContent 显式选择。

OAS 的说明、XML 和 discriminator 注解不会自动变成 JSON 实例验证规则。资源整理后的模型不是供 Swagger UI 发布的文档，不会修改 Build 的标准文档或业务路由。

## 预算与边界

- MaxBytes 默认 8 MiB，限制主文档与显式预载内容的总字节，以及 JSON 样本的原始字节。
- MaxResources 默认 64，包括输入文档与内嵌 `$id`；同一资源的检索地址和自声明地址作为别名处理。
- MaxReferences 默认 10,000；引用循环不会通过递归展开复制资源。
- MaxIndexBytes 默认 16 MiB，限制索引路径、资源元数据、URI 解析、整理后的定位文本和诊断的累计处理量。它是确定性的文本预算，不是 Go 堆内存的精确计量。
- MaxNormalizedBytes 默认 16 MiB，限制整理中间结果和最终 JSON 的编码体积，包括转义和新增的绝对 URI。先有界计量，再按引用替换差额调整；不反复编码整份文档。
- 输入规范最多 128 层和累计 200,000 个 JSON 节点，并拒绝重复键及尾随 JSON。
- 独立数值引擎前限制数字文本为 4096 字节、指数绝对值为 4096；超过限制返回 `openapi.contract.budget`，不会进行指数规模的整数分配。
- Value 对已解码样本限制 128 层、200,000 个节点，并累计计算值、键、容器和分隔符的内容大小。它不调用业务序列化方法；JSON 转义后原始字节的精确限制由 JSON 方法负责。

上述预算零值采用默认值，负值作为非法配置拒绝；超限返回包含 `openapi.spec.budget` 的错误，不返回部分 Validator。

现阶段仍需继续完善样本重复键检查、混合内联方言和完整自定义词汇表边界。复杂组合及流式协议的完整验收仍以总体目标和验证记录为准；当前回归通过不代表全部标准矩阵完成。

## 流式样本

`Validator.NDJSON` 按 LF 或 CRLF 分隔记录，拒绝记录中的裸 CR；空行忽略，末尾完整 JSON 记录也可在 EOF 验证。`ParseSSE` 按 SSE 的 CR、LF、CRLF、开头 BOM 和 UTF-8 解码规则构造传输字段对象；它不补入浏览器 EventSource 的默认事件名称或跨事件状态。`Validator.SSE` 对这些对象逐项应用 itemSchema；JSON 格式的 data 仍是字符串，需要 `AssertContent: true` 才对 contentSchema 执行内层断言。

流使用独立 `Limits`，约束总字节、单行字节和条目数。计数包含实际行尾字节；加上检查余量会溢出的配置直接报错。真实单字节分块样本覆盖最大非法 UTF-8 子序列的替换、LF/CR/CRLF 精确预算、超限拒绝和 NDJSON 裸 CR 拒绝；不会将相邻非法字节合并成一个替换字符。
