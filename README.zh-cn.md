# openapi

[English](README.md)

框架无关的 Go 源码契约编译核心与原生 OpenAPI 3.2 工具包。

本仓库正在实现。完整验收目标见 [GOAL.md](GOAL.md)，实际证据与未完成工作见[状态](docs/status.md)和[验证记录](docs/verification.md)。当前尚未完成产品验收或发布。

## 环境要求

- 最低版本验收使用精确 Go 1.27.1。
- 核心及其测试不依赖 Gin、Fiber 或 Echo。

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

自有源码注释同时提供简体中文和英文。编译指令及上游资源保留原文；示例与 Schema 测试数据中的伴随翻译通过空行与 Go 声明注释分开，使生成说明保持原有语言。提交信息使用英文。

公开编译器支持[参数对象与网络类型 codec 扩展](docs/parameter-codec.md)，不依赖框架。

生成前端可以通过[调用返回备选](docs/call-outcomes.md)关联返回值、响应提交与后续控制流。

[有限请求条件](docs/request-conditions.md)将方法和媒体类型决策从源码生成保留到运行时链接。

目标构建条件、overlay、工作区依赖、可复现指纹及集中映射配置见[构建输入指南](docs/build-inputs.md)。
