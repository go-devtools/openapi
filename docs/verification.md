# 验证记录

## 环境与仓库

- 实际平台：Darwin 27.0.0 arm64。
- 使用隔离下载并校验的精确 Go 1.27.1；Gin 固定 v1.12.0。
- 两个目标目录各自是 Git 根和独立 module；父目录未初始化 Git。
- SSH 成功认证为 Rainer-Yu；两个 private 远端创建后均通过 ls-remote，并已非强制推送首次提交。
- 核心首批提交：c28ea4b52b58b60732b80b65968ad6c7d6d9035e；适配器首批提交：86bcb463014574551129cf19ce5e1d6d0f57250f。
- 当前本地还有未提交实现增量，因此上述 SHA 不代表最终交付版本。

## 已执行的产品检查

- 核心 `GOWORK=off make dev` 退出 0：根包、CLI、compiler、contracttest、comment、validate、spec、swaggerui 测试及完整构建。
- 适配器专用 workspace 的 `make dev` 退出 0：首次或重复生成、运行时、CLI、前端、路径与完整示例测试及构建。
- `TestReadableSchemaTitles`：先观察到缺少 title 的失败，修复后退出 0。输入输出组件仍独立，展示标题相同。
- `TestFlowWriteOrdering`：内联条件读取、pending 状态、switch break、先提交状态后写 body，退出 0。
- `TestRequestContracts`：有效 ASCII/中文、字段缺失/null、太短/太长、非法 JSON/空 body，分别校验真实状态和响应 Schema；比较挂载前后完整响应，退出 0。
- `TestAllFieldTypesContract`：真实 GET 类型样本通过独立引擎；非法字符串和数字枚举被拒绝，退出 0。
- `node --test swaggerui/display-names.test.cjs swaggerui/startup.test.cjs`：Node v22.22.3，9 项通过、0 skip；验证名称隐藏、零值/复杂示例、枚举及注释、安全文本和分类恢复，拒绝任意查询配置覆盖。

## 实际浏览器

本机 `/docs/` 加载固定 swagger-ui-dist 5.32.15，展示 OAS 3.2、中文接口和 10 个示例操作（7 个标准路径）。通过内置浏览器逐次点击 Schema 标签、字段展开与模型列表，核对以下行为：

- 列表及响应模型显示 User、APIError、CreateUserRequest，不显示内部后缀。
- 字段 ID 展示紧凑 Example 1024，不显示带 #0 的 Schema 数组。
- 复制图标为高对比度双页图标，与展开箭头独立占位。
- 默认不存在 Try it out/Execute；浏览器重载网络记录仅有本机文档、脚本、CSS、图标和 data URI，没有外部 validator/CDN 请求。
- 旧深链接曾触发上游关于下划线转义的弃用日志；尚不能将浏览器控制台宣称为全历史零错误。
- Swagger UI 依赖 JavaScript Number，超过安全整数范围的显示存在上游精度限制。教学示例的六十四位数选择能准确显示且超过三十二位范围的值；核心独立精度测试仍使用 9007199254740993。

## 方法、鉴权与整体分类

- `TestHTTPMethodExamples`：13 个真实 HTTP 样本通过，包括五种方法、404、非法输入、PATCH 的 false/null、DELETE 204 空响应及 Deprecated。请求与响应使用独立 Schema 引擎。
- `TestExampleDocumentDefinitions`：all/resources/types/auth/legacy 分别包含 10/6/2/1/1 个操作，标签范围同步匹配。
- `TestDocumentGroups`、`TestInvalidGroupsLeaveRoutesUnchanged`、`TestGroupScopeIntersectionAndCache`：独立文档、ETag、HEAD、304、未知分类 404、错误配置无路由修改、全局范围求交，均通过。
- `TestExampleGroupsAndSecurity` 与 `TestEnumRequestExamples`：Bearer 的缺失/错误/正确凭据、三组命名请求及非法枚举，均通过；API Key 路由已按用户要求移除，并验证返回 404。
- 浏览器：分类下拉、刷新保留选择、Legacy 删除线及 Warning: Deprecated、Authorize 弹窗仅包含 Bearer、枚举 editor/1 和 viewer/2 实际切换均已观察。1280×900 和 390×844 页面没有横向溢出；当前独立验收页 error/warn 记录为空。未将浏览器窗口的历史日志概括为零错误。

标签筛选框已按用户要求在示例配置中关闭，页面介绍也移除对应说明；整体文档分类继续通过顶部选择器提供。

## 枚举注释

`TestEnumDescriptionsFromSource` 从真实常量的普通注释生成 `x-enum-descriptions`，与排序后的 enum 数组对应。字符串、数字/iota、单独常量及别名、浮点枚举经过测试。共享 UI 直接展示 `"admin" - 管理员`、`"editor" - 编辑者`、`"viewer" - 查看者`，以及 `0 - 待处理`、`1 - 执行中`、`2 - 已完成`；浏览器已在请求 Schema 页实际观察，当前验收页 error/warn 为空。说明缺失时仅显示原值；说明始终作为 React 文本，不插入 HTML。

## 独立 Schema 导出

`TestStandalonePreservesValuesAndDataReferences` 和 `TestStandaloneRootNumberAndDefinitionConflict` 先暴露已有 $defs 被覆盖及根级 9007199254740993 被舍入的问题；修复后核心 `GOWORK=off make dev` 通过。导出保留根与组件数值、既有 $defs 和示例中的业务 $ref，仅重写标准 Schema 位置的组件引用，重名定义返回诊断。

## 官方规范独立检查

已下载固定官方 OAS 3.2 `schema/2025-11-23`。独立 jsonschema/v6 v6.0.3 对实际导出文档的首次结构校验失败：`/security` 为 null、规范要求数组。

修复默认 Optional 复制后，新增“省略 / 显式 [] / 拒绝 null”回归通过；重新导出的真实文档再次通过官方结构 Schema 校验，退出 0。此项覆盖结构，不等于已经完成全部 OAS 3.2 语义、Schema 方言和高级功能矩阵。

固定四份官方 schema/schema-base/dialect/meta 资源和 Apache 2.0 许可证保存在 `contracttest/testdata/oas32`，来源与 SHA-256 见 PROVENANCE.md。`TestOfficialOpenAPI32Matrix` 用 jsonschema/v6 离线加载 schema-base，完整 fixture 正例与 14 个反例通过。`TestNative32AndPresence` 类型化完整往返及 `TestFullNative32Fixture` 自有检查通过。官方 Schema 不覆盖所有跨对象语义，不能据此宣称标准验收完成。

`TestNoFrameworkDependencies`、`TestRuntimeDependencyBoundary`、`TestExternalFrontend` 真实通过：外部临时 module 仅调用公开 SDK、执行编译/构建/契约/非 HTTP 资源消费。开发模式的临时 replace 明确记录，不作为远端固定版本证据；最终可通过 OPENAPI_TEST_CORE_VERSION 指定真实版本且禁止 replace。

## 尚未执行完毕

完整 race/vet/fuzz/benchmark、自动化浏览器跨环境回归、完整 3.2 语义矩阵、远端外部 SDK 模块、GitHub CI 和最终冷缓存远端固定版本验收尚未完成。未执行或受阻项目不记为通过。

## 示例英文文档

示例四个源文件的 OpenAPI 注释、枚举常量说明和文档配置改为英文，并重新生成 Bundle。适配器专用 workspace 下 `make dev` 退出 0；首次英文生成的指纹为 `13a98b4b4ffbf842052c738cbad98bd46f4d8a0ed741117fffc3f4ec2acf365b`；英文示例阶段后续 dev 回归指纹为 `6625363b0ebb4d44e61ae32a1069a683a053ea11477f1a00902f1b20375bfa4c`。导出的规范检查了 144 处标题、摘要、描述和枚举说明，没有中文说明残留；13 个业务及路由函数体经 Go AST 对比一致。浏览器实际显示五个英文分类、英文接口说明、`"admin" - Administrator`、`"editor" - Editor`、`"viewer" - Viewer`，以及 `0 - Pending`、`1 - Running`、`2 - Completed`。本节为当前英文版本的证据，前文中文截图观察仅代表此前版本。

## 文档内引用语义

新增 `TestLocalReferenceResources`、`TestInvalidReferenceResources`、`TestReferenceTargetRoles`、`TestReferenceDiagnosticsDeterministic` 和 `TestCallbackExtensionReferencesAreData`。先复现缺失锚点漏报、错误基准 URI、重复引用丢失位置与回调扩展误读，再验证修复。核心 `GOWORK=off make dev` 及适配器专用 workspace 下 `make dev` 均退出 0。覆盖 $self 相对基准、Schema $id、资源内锚点、递归引用、百分号与 JSON Pointer 转义、目标种类、身份冲突、每处诊断和扩展数据隔离。$dynamicRef 仅验证初始引用目标，不代表完成实例级动态作用域验证。本段之后已补齐显式预载资源 API、CLI 入口及 Build 裁剪联动；动态引用实例验证及 UI 外部资源本地呈现仍未完成。

## 显式资源与模型闭包

`TestCheckExplicitOfflineResources` 覆盖官方多文档用法中的检索 URI、$self 与 $id；`TestCheckNeverFetchesResources` 使用真实 HTTP 服务计数，默认拒绝与显式预载两种检查都没有发出请求。外部原始示例、重复键、尾随 JSON、资源身份冲突、预算边界、内嵌 $id 和累计 JSON 节点均有正反例。`TestBuildPrunesWithSchemaResourceSemantics` 与 `TestBuildOfflineResourceBackReference` 验证资源 URI、锚点、子节点目标、discriminator、示例数据隔离和跨资源回指的模型闭包。

CLI 清单测试验证资源路径相对于清单目录、普通文件读取预算、错误清单、help 退出码和取消状态。新增反例先在旧逻辑下失败，再修复通过。核心 `GOWORK=off make dev` 与适配器专用 workspace 下 `make dev` 均退出 0；本轮最终适配器生成指纹为 `88fc0e996b705c41e6e750e31961a3094c4e61fed47a036d7ed376e551f50e1c`。当前预览仍使用此前已验证的英文文档构建，本轮没有声明重新验收 UI 外部引用。

本轮最后补充并修复了两项组合回归：`externalValue` 指向的已识别 Schema 资源在裁剪后仍保留；discriminator 按组件名映射到带 `$id` 的 Schema 时，按该资源身份解析，不误报跨作用域。最终核心与 Gin 的 `make dev` 均退出 0。两仓库的 `go test -race ./...` 已通过；最后引用修改后的核心根包与 `internal/validate` 另行通过 race。

`FuzzReferenceGraph` 用 `-fuzztime=30s -parallel=2` 完成 1,132,358 次执行，退出 0，验证 URI / 锚点变异输入的有界性与诊断确定性。该 fuzz 不代表所有 Schema 关键字或动态引用实例语义均已覆盖。文档中的显式清单用法另通过真实 CLI 进程执行，输出 `{"diagnostics":[]}` 并退出 0。

## Schema 合法性与源码约束分离

`TestSchemaKeywordMatrix` 和 `TestOfficialSchemaKeywordMatrix` 对同一份 52 项原始 JSON 样例分别执行自有检查与固定官方元 Schema 校验，全部通过。修复前已经观察到合法 Schema 被误报，以及非法类型、负长度、空组合、错误关键字值等漏报。两个原有的错误测试分类已转移为真实源码声明反例，原始规范层接受合法但不可满足的 Schema。

`TestSourceAnnotationConflicts` 的 15 项源码反例、合法边界与开放类型正例通过，覆盖命名字段引用、组件与字段的上下界交集、固定数组长度、无符号数下界、非法指令值、实际输出 writeOnly、伪造类型、精确大整数与指数预算。类型化解码前补充关键字值检查，防止 null 被转换为零值后丢失错误。nullable 直接类型联合重复项已修复。

组合成员的资源与锚点依然参与引用检查；`TestSchemaArrayResourceReferences` 覆盖合法资源引用和默认拒绝外部引用。`TestSchemaNumberTraits` 覆盖极端正负指数、负零、小数整数判定；`FuzzSchemaNumberTraits` 以独立有界精确有理数作对照，30 秒完成 2,070,137 次执行，退出 0。

本轮核心 `GOWORK=off make dev` 与适配器专用 workspace 的 `make dev` 都退出 0。核心首次完整 dev 被沙箱禁止临时回环端口绑定，按原实现授权提升权限后完整重跑通过。`go test -race ./compiler ./internal/validate ./contracttest` 退出 0。适配器最新 Bundle 指纹为 `487f2095471be99f75d2a6c751e32c9f59a2c08ae20e3f13328fbaed4b19ce03`。

源码四个示例文件再次确认无中文注释；业务及路由的 13 个函数体与翻译前的 Go AST 对比一致。本机预览 `/docs/openapi.json` 实际读取到 User Service；本次递归检查 152 处标题、摘要、描述及枚举说明，没有中文残留。本轮没有重新构建预览二进制，没有宣称预览已验收全部新 Schema 能力。

这些通过结果不代表完整 Schema 标准验收。示例可编码性、任意组合可满足性、引用联合的 nullable / nonnull、类型化数值边界、完整资源作用域与动态引用实例验证仍有未完成项，见 [Schema 检查与源码声明](schema-annotations.md)。最终远端固定版本、关闭 workspace 的两个独立模块验收与 CI 仍未完成。

## 独立验证器资源作用域

新增真实样本回归，先观察到父级 `$id` 丢失、共享锚点不可定位和动态递归外部资源缺失，再通过资源整理修复。contracttest.Options 已接入 BaseURI、Resources、MaxResources、MaxReferences；OpenAPI / Schema 的检索 URI、相对 self/id、别名、布尔资源和明确的元 Schema 均有测试。未知必需词汇表被拒绝。真实 HTTP 服务计数证明缺少或预载资源时都没有自动网络请求。

动态递归样本证明 Strict 约束会作用于子节点，基础 Tree 和普通引用保持原行为；普通锚点或空片段的 dynamicRef 初始目标保持静态行为。指针必须指向真正的 Schema。注解数据中的 `$ref` 名称不会被当成 Schema 指令：在隔离的旧实现中，直接数据保留断言真实失败；修复后通过。仅验证实例的测试不足以证明注解字节保留，因此另外保留了资源整理结果的断言。

固定官方方言资源随可选 contracttest 包分发，保留 Apache 2.0 许可证和 SHA-256；核心根包与 Swagger UI 不引入这些资产。独立引擎前的数字、样本深度及内容预算回归通过，包含极端指数、循环内存对象与恰好达到字节上限的样本。

两仓库完整 dev 均退出 0；完整 `go test -race ./...` 均退出 0。最后注解所属上下文修复后，核心再次通过 dev 与 contracttest / internal/validate 的 race，Gin 再次通过 dev。最新 Gin Bundle 指纹为 `c7dd2f1870492242302e4854b552e22b99ffc201918587086205a7210bc7f589`，模板 11 个。本轮未重启 UI 预览，也未宣称远端模块已更新。

资源索引与位置整理复用核心规则，实例验证算法仍由独立引擎执行。规范化后 URI 与索引内存预算、样本重复键、混合内联方言、Standalone 的完整资源语义、UI 外部资源发布以及最终远端冷缓存验收仍未完成，见 [契约验证指南](contracttest.md)。

## 资源文本与规范化预算阶段

- 已复现并修复长前缀索引、长绝对引用扩张未受预算限制的问题。
- 新增正反例覆盖检索 URI、显式资源键、选中指针、诊断文本、有效引用解析、负预算、CLI 透传和公开 contracttest 配置。
- 使用标准 encoding/json 交叉检查容器、分隔符、Unicode、控制字符与非法 UTF-8 的编码体积；恰好达到规范化上限通过，少一字节失败。
- `GOWORK=off make dev`：退出 0，包括核心全部测试、外部临时 module SDK 测试及所有包构建。
- `GOWORK=off go test -race ./internal/validate ./contracttest ./cmd/openapi`：退出 0。
- `GOWORK=off go test ./internal/validate -run '^$' -fuzz '^FuzzJSONStringBudget$' -fuzztime=15s -parallel=2`：退出 0，150943 次执行。
- 本轮外部 SDK 测试尚使用开发替换模式；真实远端固定版本与冷缓存验证另行记录，不以此替代。

当前默认分支为 main。全部命令使用既定 Go 1.27.1 工具链和任务隔离缓存；资源整理保持框架中立，适配器仍只访问公开 API。

阶段暂存区检查：98 个变更文件的凭证模式与敏感文件名检查无命中。两份原样保留的上游 Swagger UI LICENSE.txt 含行尾空格；它们保留供应商校验值，未擅自修改。排除这两份上游许可证后，暂存区 `git diff --check` 通过。UI 的 9 项 Node 展示与初始化回归全部通过。

## 双语源码注释

自有 Go、UI 扩展和构建脚本注释已补齐中英双语，生成器同步输出双语注释；第三方资产与机器指令保留原文。AST 注释清单检查覆盖当前 1316 行自然语言注释，未发现缺少对应翻译的条目（生成文件另由重新生成验证）。80 个 Go 文件的 token 对比确认仅生成器中的三段注释字符串变化；8 个示例或测试数据文件绑定到声明的文档保持原值，13 个业务及路由函数体与英文翻译前一致。

核心 `GOWORK=off make dev` 退出 0，9 项 Node 展示及初始化回归全部通过；改动文件的 `git diff --check` 通过。历史提交说明的英文改写与双语源码提交分开验证，不将过去的版本号当作新的固定依赖依据。

## Standalone resource export and public SDK

The standalone exporter now resolves projection components within their actual resource scopes, preserves declared dialects and exact numbers, embeds explicitly supplied JSON Schema resources, and checks the final document without external resources. Its SDK options and CLI share bounded offline resource handling. CLI output is replaced only after a successful export, and its trailing newline counts toward the output budget.

New regression cases first failed for nil roots/components, missing references, overwritten dialects, resource-local references incorrectly rebound to root components, identified recursive components, missing embedded dependencies, ignored embedding budgets, OpenAPI annotation mappings treated as resource loads, missing CLI flags/help handling, invalid dialect URIs, and the uncharged trailing newline. The fixes passed the complete core dev target.

The independent jsonschema/v6 engine receives only the exported document and a loader that rejects implicit access. Positive and negative instances verify dynamic recursive constraints, identified components, retrieval aliases, relative identities, boolean resources, and annotation preservation. No custom-vocabulary implementation is claimed.

With Go 1.27.1 and GOWORK=off:

- `make dev`: exit 0; all packages tested and built.
- `go test -race ./...`: exit 0.
- `go vet ./...`: exit 0.
- `go test ./compiler -run '^$' -fuzz '^FuzzStandaloneComponentIdentity$' -fuzztime=30s -parallel=2`: exit 0, 57,081 executions. This checks URI and JSON Pointer escaping against an independent engine; it is not exhaustive schema fuzz coverage.
- `go test ./internal/verify -run '^TestExternalFrontend$' -count=1 -v`: exit 0. The external module now also runs TestStandaloneSchemaSDK, validating source constraints and eight concurrent read-only exports. This development invocation uses a temporary local replacement, not a remote-version claim. The outer race invocation does not implicitly add race instrumentation to that child process.

AST review found no missing bilingual counterparts among existing comments, and all 37 newly reviewed translation pairs contain both languages. The declaration audit included untracked source and found no undocumented top-level function or declaration. Example handler source was not changed in this stage. These checks use the existing task cache and do not constitute final cold-cache or full-goal acceptance.

## Neutral response state and explicit wire schemas

The public compiler effects now preserve response headers at commit time, header source facts, unknown committed statuses, and frontend-provided response-body WireSchema values. Branches receive independent header maps; same-status constant alternatives use anyOf. Header replacement, deletion, conditional insertion for an empty current value, and writes after commitment are distinguished. HTTP bodyless statuses no longer project an unused response payload. Gin-specific renderer and method rules remain in the adapter.

The real Gin development matrix first produced 14 failures for missing renderer support, spurious 204 content, missing headers, and immediate status commits. After the neutral and adapter changes, all 14 actual response cases passed under the dedicated development workspace. A direct Writer.Write negative case first passed incorrectly; the adapter now recognizes it as an unresolved response effect. Core-only tests additionally exposed and fixed empty-header fallback and invalid HTTP header-name acceptance. Unknown committed statuses, same-status header alternatives, header provenance, and caller-owned WireSchema immutability pass without framework imports.

With the exact Go 1.27.1 toolchain and GOWORK=off, core make dev, go test -race ./..., and go vet ./... all exited zero. The external development module ran TestPublicFrontend, TestTransportNeutralResources, TestStandaloneSchemaSDK, and TestExplicitWireResponseSDK successfully; this invocation uses a temporary core replacement and is not remote-version evidence. Raw binary representation follows OpenAPI 3.2 contentMediaType semantics and does not add a JSON string or Base64 constraint.

This stage does not complete method-conditioned rendering, interim-response sequences, all binders/codecs, or full-goal acceptance. Gin's new dedicated verification package still requires execution after the synchronized core dependency is pinned.

## Parameter objects and codec type callbacks

Public WireTypeCodec callbacks now control non-JSON type representations while sharing the current projection's recursion budget and component state. TypeMapper rules remain first; field-only codecs retain their earlier default behavior. ParameterObject expands explicit projected objects into query/path/header/cookie parameters, retaining annotations, required flags, schemas, and frontend-provided serialization. Value.Object preserves lexical go/types identity through supported propagation without serializing compiler objects into Bundle.

The neutral codec regression first rejected a valid byte list, accepted Base64 and null for text parameters, and rejected the missing parameterObject effect. The implementation now passes independent schema validation for those values. A later regression proved that root minProperties was silently discarded; unsupported whole-object constraints and resource scopes now produce diagnostics instead of incomplete parameter lists. Callback nil values, bounded recursion, and copied-schema ownership pass.

The external development consumer now runs five public SDK tests, including TestParameterCodecSDK from a module with no framework or internal imports. Go 1.27.1 with GOWORK=off passed make dev, full race, vet, and module verification. Following the final keyword guard, make dev and race for compiler/internal/verify passed again; vet remained clean. The consumer test inside the standard verification harness is not automatically race-enabled by the outer test.

The Gin development workspace separately passed actual Query, URI, Header, explicit JSON, FormPost, and Multipart samples, seven invalid-request branches, embedded-field handling, enum projection, and a centralized custom-parameter mapper. It exposed and fixed a spurious parameter for anonymously embedded time.Time. These workspace checks are development evidence, not the pending fixed-remote-version adapter verification. Automatic method/media selection, implicit MustBind effects, complete parameter conflicts and provenance, CI, and final acceptance remain outstanding.

## 调用返回备选与控制流

公开 CallOutcomes 将返回元组与共同发生的效果关联，核心逐路径处理 helper、短路、switch、return 和后续写入。十组中立源码用例覆盖响应提交顺序、元组、nil 装箱及指针别名写入；公开备选测试覆盖忽略错误、立即返回、提交后写入与无效返回数量。关闭备选传播的变异测试确实失败，恢复后通过。独立 module 测试夹具也消费这些公开 API。

精确 Go 1.27.1、GOWORK=off 的 make dev、全模块 go test -race ./...、go vet ./...、go mod verify 均 exit 0。此处 internal/verify 的开发消费者使用临时 replace；外层 race 不表示其普通子进程自动启用 race。固定远端版本的消费者及 CLI 实证在同步后另行记录。

两个仓库最新 AST 注释审计覆盖 1765 行自然语言 Go 注释，未发现缺失中英对应；所有顶层函数和声明均有注释。完整 Goal 仍有自动方法/媒体绑定条件、完整 helper/Schema 矩阵、CI 与最终冷缓存验收待完成。

## 有限请求条件与选择性诊断

新增 RequestCondition、OperationVariant、CallOutcome.When、Route.RequestMediaTypes 与 Source.When，条件 Bundle 必须声明 request-conditions-v1。生成阶段对连续调用条件求交，再按相同条件合并完整路径；运行时仅选择已投影的数据，不包含 Gin、AST 或执行回调。未选中的 codec 诊断不影响当前路由。

序列化决策表回归最初因缺少 variants 失败，连续调用回归最初把互斥分支的诊断混到一起。实现后，方法/媒体选择、未知范围、能力门禁、条件交集与所有权、多个兼容媒体、声明来源和无效 codec 范围均通过。额外回归先暴露了复合 Schema 同级约束错误收窄另一备选，以及不同媒体的请求体 required 被静默合并；两项均修复并通过。

Gin 开发 workspace 中 17 组成功请求验证自动 JSON、Query、Form、Multipart、GET/POST/PUT/PATCH/DELETE/QUERY 方法与真实优先顺序；八组错误/体积限制请求验证 422、400、413 和不读取 GET JSON 体时不存在虚假 413。多个位置的 required 歧义明确诊断，字段约束不被静默丢弃。独立验证器进一步检查请求数字、非 null 表单数组、重复查询序列化和真实响应。

核心在精确 Go 1.27.1、GOWORK=off 下的 make dev、全模块 go test -race ./...、go vet ./...、go mod verify 均通过。外部 SDK 开发夹具新增序列化条件链接和源码条件调用测试；此处临时 module 的核心 replace 是开发检查，固定远端消费将在同步后单独验证，普通 child test 不自动继承 outer race。

两仓库 AST 审计覆盖 1879 行自然语言 Go 注释，无缺失中英对应，所有顶层声明和方法有注释。完整 Goal、最终固定版本的冷缓存验证及尚未完成的矩阵仍保留。
