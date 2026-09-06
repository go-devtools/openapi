# 公开适配器 SDK

本文件是两仓库共享接口的权威说明。当前接口处于首次实现阶段，尚未发布稳定版本。

## 运行时协议

`openapi.NewBundle(BundleData) (Bundle, error)` 创建格式 1、规范 3.2.0 的不可变快照。`ParseBundle` 校验交换格式；`GeneratedBundle` 供静态生成文件调用，解析错误延迟到 `Validate` 或 `Build` 返回，不触发 panic。

`Bundle.Index()` 和 `Snapshot()` 返回独立副本。`OperationKey` 是源码模板键，最终 operationId 默认由标准 method/path 的摘要生成。`Route` 已经是标准路径；核心不解释框架路由语法。

`Build(Bundle, []Route, Config) (*Document, error)` 无路由注册或源码读取。`Document.JSON()`、`Report()` 返回副本，`WriteFile` 原子替换输出。一个 Bundle 可以并发用于多个文档实例。

## 离线规范检查

`Check` 使用默认配置，`CheckWithOptions` 支持显式 `BaseURI`、预载 `Resources` 与 `ExampleResources`。`Config.Validation` 将相同资源配置用于 Build 的模型闭包裁剪和最终检查。两者只读取调用期间的输入，不自动读取文件、访问网络或保存调用方映射。资源默认总量为 8 MiB、64 个资源和 10000 次引用，包含内嵌 `$id`；所有 JSON 共享节点预算。详细使用方式和未完成的 UI / 独立验证联动见[离线引用指南](references.md)。

## 生成时协议

`compiler.Load(context.Context, LoadOptions) (*Project, error)` 使用真实构建条件加载 Go AST 与类型。公开视图只使用 go/ast、go/token、go/types、go/constant，不暴露第三方 SSA。视图由 Project 拥有，外部仅能读取；修改视图属于违反接口约定。

`Project.Type` 解析已加载类型及泛型实例。`Project.Schema` 返回独立投影缓存；`Projection.Standalone()` 将完整组件闭包转为独立 Schema 的 $defs，默认方言为 JSON Schema 二零二零十二。`StandaloneWithOptions` 提供显式基准 URI、离线依赖、后备方言与预算，按资源身份处理引用，保留已有方言；详见[独立 Schema 指南](standalone-schema.md)。不同方向、媒体类型、codec 和泛型身份独立缓存。每次调用的缓存不共享，Project 加载后支持并发只读投影。

`Frontend` 通过显式 Go 值注册，包含 Name、Match、Entry、Call、CallOutcomes、Return 与 CarriesEffects 回调。回调输出中立 Effect；核心调度值传播、语句顺序、分支、返回及有界 helper。ReturnContext 接收函数返回值，接口不要求特定 context 形态。

`compiler.Compile(context.Context, Options) (*Result, error)` 复用加载、注释、投影与生成。生成器只原子更新带所有权标记的 `zz_openapi.gen.go`，生成文件只导入轻量核心，不执行或导入业务 handler。`Result.Check` 重新生成并比较完整字节。

## 失败、预算和版本

缺省预算为 2048 个加载包、4096 个投影类型节点、12 层 helper、128 条分析路径和 10000 次调用。注释上限 64 KiB；输入文档上限 8 MiB、深度 128、JSON 节点 200000。超预算返回稳定错误，不将截断结果当作完整。

当前模板诊断随 Bundle 保存，只在实际选中路由时阻断 Build；全局加载、注释语法与前端冲突会阻断编译。所有回调必须确定性、不得执行待分析业务函数，不得依赖机器路径、时间或网络结果。

格式 1 读取器接受能力 oas32、schema2020-12、request-conditions-v1，拒绝未知必需能力与未来格式。首个真实可消费提交同步后，由 Go 命令解析固定伪版本供适配器消费。尚未承诺稳定 SDK SemVer 范围。

## 尚待实现与扩大验证

公开前端已有非 Gin 返回值形态的源码测试，并已通过真正临时外部 module 的公开 API 消费测试。开发测试使用隔离临时 replace；最终固定远端版本测试由 OPENAPI_TEST_CORE_VERSION 选择，届时禁止 replace。多路径 helper 参数化摘要缓存、集中兜底类型解析、闭包与 receiver 消歧、完整构建 profile 指纹、导入类型的注释和所有标准矩阵尚未验收。当前对未完成的关键行为返回诊断，不能描述为全部自动支持。

## 响应提交与明确网络表示

共享分析器已支持按路径保存响应头、立即提交状态和明确的非 JSON 响应 Schema。Gin 类型与 Renderer 规则继续留在适配器，核心只接收中立效果。`Effect.WireSchema` 用于已证明的请求或响应网络表示，`ResponseHeader` 控制提交前的头值；详见[响应效果 SDK](response-effects.md)。

## 参数对象与非 JSON 类型规则

`ParameterObject` 通过共享 Schema 投影将明确对象展开为 query/path/header/cookie 参数，保留字段约束与引用，序列化由前端的 Style/Explode 指定。可选 `WireTypeCodec` 类型回调使用同一次投影的递归预算与组件缓存，已处理结果通过复制隔离调用方。实现该接口的 codec 负责自己的标准类型与自定义方法，不继承 JSON 编解码假设；现有只实现字段选择的 WireCodec 保持旧行为。回调只能同步使用，不得保留或并发调用递归函数。`Value.Object` 保存标准库符号身份用于明确前端匹配。详细所有权、失败方式与边界见[参数 codec SDK](parameter-codec.md)。

## 逐字段请求体与编码

`RequestField` 支持不依赖 DTO 的命名请求体字段，结合 `WireSchema` 或类型/codec 投影并保留标准 `Encoding`。字段 Required 与整个请求体 Required 分开；同路径约束取交集，不同路径使用备选。普通参数读取同时保留显式网络类型、Style/Explode 和冲突诊断。所有权、路径合并及验证边界见[请求字段 SDK](request-fields.md)。

## 逐项响应与内层载荷

`ResponseItem` 将同路径、同媒体类型的连续条目合并为原生 `itemSchema`。`PayloadMediaType` 选择真实内层 codec；`TransformSchema` 在编译期包装独立的投影结果，输入输出均复制隔离，nil 或错误阻止文档发布。普通正文混写和条件分帧冲突保留诊断，HEAD/无正文状态不投影未发送内容。公开 SDK 的 NDJSON/SSE 独立校验、回调约束与剩余框架边界见[响应效果指南](response-effects.md)。
