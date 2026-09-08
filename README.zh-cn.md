# openapi

[English](README.md)

框架无关的 Go 源码契约编译核心与原生 OpenAPI 3.2 工具包。

当前 SDK 尚处于 1.0 之前，接口可能随固定版本更新而变化。请使用公开 API，并先核对各功能指南中的支持范围。

## 安装与版本

首个版本为 **v0.0.1**。可直接引入公开 Go module，无需令牌、workspace 或本地 replace：

```sh
go get github.com/go-devtools/openapi@v0.0.1
go install github.com/go-devtools/openapi/cmd/openapi@v0.0.1
```

运行 `openapi version` 查看实际版本。下载预编译 CLI、源码和 SHA-256 校验和请访问 [GitHub Releases](https://github.com/go-devtools/openapi/releases)。库依赖和 CLI 应固定到相同版本。

main、develop、release 和 hotfix 分工见 [贡献与发版说明](CONTRIBUTING.zh-cn.md)。v0 API 仍在演进，请阅读发布说明后升级。

## 环境要求

- 开发和验证使用 Go 1.27.1。
- 核心及其测试不依赖 Gin、Fiber 或 Echo。

## 快速开始

在本仓库根目录使用 Go 1.27.1 执行：

```sh
GOWORK=off go mod download
schema_file="$(mktemp)"
GOWORK=off go run ./cmd/openapi schema --dir ./testdata/types --type Request --projection request --output "$schema_file"
cat "$schema_file"
GOWORK=off go run ./cmd/openapi check --spec ./testdata/golden/openapi32-full.json
GOWORK=off make dev
```

第一条功能命令从真实的零 tag [`Request`](testdata/types/types.go) 导出契约，包含源码注释、递归父对象、字节和整数键 map。输出为使用本地 `$defs` 引用的独立 JSON Schema 2020-12，并非 OpenAPI 文档。第二条功能命令校验仓库提供的原生 OpenAPI 3.2 文档；成功报告中没有错误诊断。临时 Schema 文件可自行查看或删除。

接入自己的项目时，将 `--dir` 和 `--type` 替换为实际包目录和 Go 类型。输出编码使用 `--projection response`。源码投影和文档检查都不执行业务 handler。等价 Go API 见[独立 Schema 指南](docs/standalone-schema.md)；不依赖框架的完整源码到 Bundle 示例见[外部返回值前端](internal/verify/testdata/external/consumer_test.go)。

| 任务 | 公开 API |
| --- | --- |
| 加载并投影 Go 源码 | `compiler.Load`、`Project.Type`、`Project.Schema` |
| 编译已注册的前端 | `compiler.Compile`、`Result.Write`、`Result.Check` |
| 将生成的 Bundle 与规范化路由链接 | `openapi.Build(bundle, routes, config)` |
| 读取或保存不可变文档 | `Document.JSON()`、`Document.Report()`、`Document.WriteFile(path)` |
| 准备共享离线 UI 资源 | `swaggerui.New`、`UI.Resource`、`Resource.Bytes` |

按需导入 `compiler`、`contracttest` 和 `swaggerui`，根运行时包不会把它们带入业务程序。升级固定模块前请核对 [SDK 与 Bundle 兼容契约](docs/adapter-sdk.md#version-compatibility)。

## 架构

Go 源码与真实编解码提供结构，普通注释提供业务语义。文档生成不修改业务 DTO tag、handler 函数体或签名，也不改变既有路由注册。

核心拥有类型投影、注释、中立效果、Bundle 和 OpenAPI 模型。框架适配器拥有框架调用语义、路径语法、handler 证据和挂载。运行时构建文档不导入编译器、不读取应用源码。

## 能力边界

- **自动推导：** 类型、支持的网络表示和已识别的源码效果，以实际测试为准。
- **显式声明：** 语义约束与高级契约；声明不等于已经证明服务端执行。
- **集中适配：** 自定义 codec 和未支持的项目 helper，通过显式 Go 扩展接入。
- **无法消歧：** 不确定或未支持行为必须返回诊断，不能猜测响应。

Fiber 和 Echo 仅为未来扩展方向，本仓库未交付或宣称支持这些适配器。

## 离线引用检查

`CheckWithOptions` 接收显式检索 URI、预载的 OpenAPI / JSON Schema 和外部示例原文。文档构建通过 `Config.Validation` 使用相同配置，命令行通过 `openapi check --resources resources.json` 读取显式清单。检查器限制全部输入，始终不自动加载 URI。资源作用域、预算与当前展示边界见[引用 API 与 CLI 指南](docs/references.md)。

## Schema 与源码约束

原始文档检查接受合法但不适用或不可满足的 JSON Schema。源码投影另外诊断注释与推导出的网络类型、边界之间的冲突，覆盖命名组件引用。已经验证的行为与未完成边界见 [Schema 检查与注释诊断](docs/schema-annotations.md)。

## 独立契约验证

可选 contracttest 包接收显式预载资源，使用独立引擎验证实际 JSON 样本，覆盖动态递归引用，不自动获取缺失资源。选项、预算和未完成边界见[契约验证指南](docs/contracttest.md)。

## 独立 JSON Schema

`Projection.StandaloneWithOptions` 按资源作用域处理引用，把显式依赖嵌入单个离线文档，保留方言并限制输出预算。`openapi schema` 提供对应命令行选项。公开 SDK、资源规则和限制见[独立 Schema 指南](docs/standalone-schema.md)。

## 中立响应效果

公开编译 SDK 支持提交时的响应头和明确的非 JSON 网络 Schema。顺序、来源与限制见[响应效果指南](docs/response-effects.md)。

## 许可证

项目新增代码采用 [MIT](LICENSE)。第三方资源保留原许可证与声明。

自有源码注释、OpenAPI 描述、诊断、CLI 帮助、示例文字和提交信息使用英文。多语言编码测试保留其真实输入数据，上游资源保留原文，本文件提供对应的中文使用说明。

公开编译器支持[参数对象与网络类型 codec 扩展](docs/parameter-codec.md)，不依赖框架。

生成前端可以通过[调用返回备选](docs/call-outcomes.md)关联返回值、响应提交与后续控制流。

[有限请求条件](docs/request-conditions.md)将方法和媒体类型决策从源码生成保留到运行时链接。

目标构建条件、overlay、工作区依赖、可复现指纹及集中映射配置见[构建输入指南](docs/build-inputs.md)。

运行时调用方可使用 `openapi.CheckRuntimeBuild(profile)`，或在链接文档时启用 `Config.VerifyRuntimeBuild`。已知构建条件不匹配时失败，缺少元数据时保留诊断。跨目标导出与验证范围见[构建输入](docs/build-inputs.md)。

公开编译器支持[逐字段请求效果](docs/request-fields.md)，保留单个请求体字段、明确编码及与完整对象投影的组合；字段存在与请求体存在分别处理。

公开响应 SDK 支持 `ResponseItem` 逐项响应、独立内层 codec 和编译期 Schema 包装隔离；NDJSON/SSE 独立验证与尚待完成的框架边界见[响应效果指南](docs/response-effects.md)。

公开值与响应快照保留装箱载荷身份和真正提交的响应头。独立流验证覆盖协议各自的行尾、UTF-8 替换和输入预算，见[契约指南](docs/contracttest.md)。

公开[回调 SDK](docs/callbacks.md) 分析已知函数值和同步回调约定，保留隔离的捕获单元与有界重复；框架回调语义继续由适配器负责。

## AI 辅助接入

从 [llms.txt](llms.txt) 查看精简文档索引，再阅读 [AI 接入指南](docs/ai-integration.md)，获取真实可执行的命令、结构化诊断说明及公开 API 边界。生成的 JSON 和来源信息可用于核对接入判断。

参见[性能指南](docs/performance.md)，运行可复现的 100／1000 路由生成、启动 Build、文档读取与内存分配基准。

参见[独立 CI 指南](docs/ci.md)，了解固定工具链、私有模块访问、离线浏览器检查、真实平台任务和固定远端版本消费验证。

参见[安全模型指南](docs/security-models.md)，了解类型化 OAuth 流程、原生 3.2 字段、存在性、验证与 UI 边界。

离线 UI 可展示原生请求与响应体示例，并在提交时保留显式序列化文本。配对值、已验证格式及剩余 UI 边界见[原生示例](docs/native-objects.md#example)。

参见[来源解释](docs/explain.md)，了解显式启用的字段、类型、接口和响应来源查询，以及声明与实施证据的边界。

Example、Discriminator、XML、Tag 和 multipart 的验证规则，以及可选布尔字段和 `Tag.Parent` 的 API 迁移方式见[原生对象与显式值](docs/native-objects.md)。

参见[离线 UI 兼容性](docs/swaggerui-compatibility.md)，了解已实测的 QUERY 提交、被省略的扩展方法与标签元数据的浏览器端诊断，以及剩余原生展示限制。
