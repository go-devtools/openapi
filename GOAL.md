# Goal：从零交付 openapi 与 gin-swagger 两个独立 Go 工具包

> Goal ID：openapi-golang/two-repository-code-first-v1
>
> 目标：框架无关的 Go 源码契约编译核心，加上无业务侵入的 Gin 适配器；生成原生 OpenAPI 3.2 文档及离线 Swagger UI。
>
> 本文件是交给 Codex 执行的完整目标与验收规范，不表示两个仓库或实现已经创建。它独立包含必要要求，替代旧的单仓库 ginoas Goal；不要继续使用旧的模块路径、包划分和单仓库发布方式。

## 1. 最终任务与固定身份

从零创建、实现、联调、测试并交付下列两个独立 Git 仓库、两个独立 Go module。这里的“两个包”指两个可独立版本化的模块，不是限制每个模块只能有一个 Go package。

| 项目 | 核心基础包 | Gin 接入包 |
| --- | --- | --- |
| GitHub owner | `openapi-golang` | `openapi-golang` |
| 仓库名 | `openapi` | `gin-swagger` |
| 仓库地址 | `https://github.com/openapi-golang/openapi` | `https://github.com/openapi-golang/gin-swagger` |
| Go module | `github.com/openapi-golang/openapi` | `github.com/openapi-golang/gin-swagger` |
| 根 Go package | `openapi` | `ginswagger` |
| 本地目录 | `/Users/whylost/private-projects/openapi` | `/Users/whylost/private-projects/gin-swagger` |
| 最低 Go | `1.27.1` | `1.27.1` |
| Gin 依赖 | 禁止 | `github.com/gin-gonic/gin v1.12.0` 起 |
| CLI | `openapi` | `gin-swagger` |

`gin-swagger` 是仓库名和命令名；Go 源文件必须使用合法标识符 `package ginswagger`。不要使用 `package gin-swagger`，不要改为其他组织或占位 module path。

此次只交付这两个仓库。为将来的 Fiber、Echo 等接入包预留经过测试的扩展边界，但不创建这些仓库，不引入这些框架依赖，也不将其描述为已经支持。

最终交付不是设计草图、空接口、演示 JSON 或仅能跑通首页的 MVP，而是完整源码、CLI、生成器 SDK、运行时接入、实例、自动化测试、文档和真实验证记录。

## 2. 工作位置、远端操作与安全边界

### 2.1 本地工作范围

首先读取目标目录及其适用父目录中的 `AGENTS.md`，检查已有文件、Git 状态、远端和权限。

只在两个目标目录中创建产品代码。共享联调文件允许放在 `/Users/whylost/private-projects` 下，但不得把该父目录初始化成第三个仓库，不扫描或修改其他私人项目。

如果目标目录已经存在，先判断是不是预期项目。使用 `git rev-parse --show-toplevel` 核实仓库根目录恰好等于目标目录，不能误把父目录的 Git 仓库当作新项目。保留用户未提交的改动；禁止删除目录重建、强制清理或覆盖现有历史。

当前环境不是用户的 Mac、没有目标文件系统或缺少写权限时，准确报告。不得在其他机器创建一个同名 `/Users/...` 目录后宣称已经操作了用户本机。

### 2.2 GitHub 仓库创建

执行此 Goal 时，用户授权在指定组织下创建上述两个仓库，并将本次新增的项目代码通过非强制 push 同步到对应远端。先检查现有 GitHub 认证、组织访问权、仓库是否存在及其归属；不要要求用户把 token 明文发到聊天或写入仓库。

未指定仓库可见性时，采用保守默认：新建为 private。不得自行改成 public。若组织策略不允许该可见性，记录阻塞并继续可进行的本地工作，不擅自扩大公开范围。

在确认仓库不存在、目标目录属于本项目且认证有权限后，可以使用：

```bash
gh repo create openapi-golang/openapi \
  --private \
  --source=/Users/whylost/private-projects/openapi \
  --remote=origin

gh repo create openapi-golang/gin-swagger \
  --private \
  --source=/Users/whylost/private-projects/gin-swagger \
  --remote=origin
```

以上是有前置条件的命令，不是可以无条件重复执行的脚本。远端存在时检查并使用，不删除、不重建；不能把认证失败或隐藏私有仓库导致的 404 直接当成“仓库不存在”。

推送前检查暂存区、凭证和敏感文件；仅提交本任务的文件。使用已有 Git 身份，不伪造作者。保护分支使用普通功能分支与 PR，不 force-push。此 Goal 不授权更改组织权限、移除保护规则、自动公开仓库、部署公网服务或创建正式 release/tag。

### 2.3 本地联调不污染父工作区

优先使用专用工作区文件：

```text
/Users/whylost/private-projects/openapi-golang.work
```

内容为：

```go
go 1.27.1

use (
    ./openapi
    ./gin-swagger
)
```

通过单次命令的 `GOWORK` 环境变量启用它。不要覆盖父目录已有 `go.work`，不要修改其他项目的工作区设置，也不要将用户绝对路径写入产品代码或发布 go.mod。

这只是开发辅助，不是两个模块安装或构建的前提。独立验收必须关闭 workspace，具体见后文。

## 3. 不可改变的产品原则

| 维度 | 硬性要求 |
| --- | --- |
| Code-first | Go 源码、真实编解码和可识别行为提供结构；注释补充契约；OpenAPI 是生成产物 |
| 用户 DTO | 不新增、不要求 struct tag；无 tag 的示例必须覆盖完整主路径 |
| 普通注释 | 只写业务语义，不重复字段名、函数名、类型名、常量名 |
| handler | 不修改原有签名、函数体、返回值模式或控制流 |
| 路由 | 不替换或包装原有 Gin 路由注册方法 |
| 接入 | 一次源码生成，加启动层一次文档挂载 |
| 运行时 | 不读取源码，不分析 AST，不执行用户业务代码探测类型 |
| 业务请求 | 不增加文档 middleware、采样、代理、反射或校验逻辑 |
| 标准 | 原生 OpenAPI `3.2.0`；Schema 按该规范和 JSON Schema 2020-12 正确表达 |
| UI | 真正的 Swagger UI，本地嵌入，默认无外部请求 |
| 架构 | 核心到任何框架适配器的反向依赖均禁止 |
| 可用性 | 两个模块分别可测试、可构建、可通过真实版本依赖使用 |

禁止将本项目实现为 schema-first 路由框架、Gin 替代品、业务参数绑定框架或运行时校验框架。不得生成或注入 decoder、validator、MarshalJSON、业务 middleware、路由包装器，也不得修改 Gin 全局配置来迎合文档。

允许业务项目添加：语义注释、生成入口、生成文件、文档启动配置。少量无法推断的项目封装可在独立文档文件中集中适配，不能让每条正常路由重复声明 method/path/request/response。

最低版本是测试下限，不是对所有未来 Go/Gin 版本的无限兼容承诺。两个 go.mod 均使用 `go 1.27.1`；如果目标工具链或依赖不可取得，报告具体失败，不静默降低版本或用更新版本测试冒充最低版本验证。

## 4. 核心架构与职责归属

```text
业务项目
  ├─ 生成时：gin-swagger CLI / compiler 前端
  │             │ 识别 Gin 调用与绑定行为
  │             ▼
  │          openapi/compiler
  │             │ 注释、类型、通用数据流、Schema、诊断
  │             ▼
  │          静态 openapi.Bundle
  │
  └─ 启动时：ginswagger.Build / Mount
                │ 实际 Gin 路由、路径语法、handler 链接
                ▼
             openapi.Build
                │ 框架中立的路由 + Bundle
                ▼
             openapi.Document
                ├─ OpenAPI 3.2 JSON
                └─ openapi/swaggerui → Gin 文档挂载
```

### 4.1 openapi 负责

Go 包加载；普通注释与统一指令；类型身份和投影；通用分析调度与有界数据流服务；框架中立 IR 和 Bundle；OpenAPI 3.2 模型、链接及语义检查；独立 JSON Schema 导出；通用诊断；可选 Swagger UI 资源与渲染；通用契约测试；生成器/适配器的公共 SDK。

框架无关不等于只提供一组 OpenAPI struct。核心必须真正拥有编译、Schema、文档构建等公共能力。

### 4.2 gin-swagger 负责

Gin handler 候选识别；Gin 请求读取、绑定、响应和控制流效果的语义规则；Gin codec 配置适配；真实路由快照；Gin 路径解析；handler 到 Bundle 模板的证据匹配；文档作用域、框架特有诊断和 mounting；面向用户的 Gin CLI；Gin 示例及集成测试。

适配器不只是把 `http.Handler` 包装进 Gin。生成时和运行时都有框架边界，二者都必须完整实现。

### 4.3 严格单向依赖

```text
gin-swagger                  → openapi
gin-swagger/compiler         → openapi/compiler
gin-swagger/cmd/gin-swagger   → 上述生成器能力
未来其他框架适配器             → openapi 的同一公共 SDK
```

禁止以下形式：

```text
openapi → gin-swagger
openapi/compiler → Gin / Fiber / Echo
openapi CLI → 静态内置所有框架适配器
核心里 switch framework == "gin" 来执行框架专属语义
适配器跨仓库 import 核心 internal 包
复制核心源码、软链接源码、go:linkname 或 unsafe 绕过边界
```

核心读取业务 AST 时可以看到业务代码的 Gin 包标识，这是“被分析的数据”，不是允许在核心实现中硬编码 Gin 分析规则。

## 5. 两个仓库的目录与模块边界

推荐目录如下。允许在不破坏职责、公共 API 和验收命令的前提下调整内部文件拆分；不要机械地为每个名词创建一个 public package。

```text
openapi/
├── go.mod / go.sum
├── GOAL.md
├── README.md
├── bundle.go / route.go / document.go / config.go
├── spec/                       # 可供外部使用的原生 OAS 3.2 类型
├── compiler/                   # 对外的编译器与前端扩展 SDK
├── swaggerui/                  # 可选导入；共享 UI 资源与呈现
├── contracttest/                # 框架无关、测试期的契约辅助
├── cmd/openapi/                 # schema、check-spec、version
├── internal/
│   ├── comment/
│   ├── load/
│   ├── analysis/
│   ├── schema/
│   ├── link/
│   ├── validate/
│   └── verify/
├── examples/                   # 无 Gin 依赖的核心示例
├── testdata/                   # 规范 fixtures、外部前端测试 fixtures
├── docs/
└── .github/workflows/

gin-swagger/
├── go.mod / go.sum
├── GOAL.md
├── README.md
├── build.go / mount.go / config.go
├── compiler/                   # 公共 Gin 前端，供 CLI/自定义生成入口组合
├── cmd/gin-swagger/
├── internal/
│   ├── analyze/                # Gin 调用语义
│   ├── codec/                  # Gin 特有的绑定/渲染适配
│   ├── routes/                 # Gin 路径与路由快照
│   ├── resolve/                # handler 证据匹配
│   ├── integration/
│   └── verify/
├── examples/
│   └── basic/
│       ├── main.go
│       └── internal/apidoc/zz_openapi.gen.go
├── testdata/
├── docs/
└── .github/workflows/
```

两个仓库各只有一个产品根 module；临时的外部消费测试可以生成 fixture module，但不是第三个发布项目。

`openapi` 根包不得导入 `compiler`、`swaggerui`、`contracttest`。仅构建文档的用户不应被迫把分析器、UI 静态资源或第三方全量验证器打入业务二进制。

`ginswagger` 根包不得导入自己的 `compiler`。CLI 可以有开发依赖，普通业务程序不可以因调用 Mount 而依赖 AST/SSA 加载链。

严格区分“同一 module 的依赖清单”和“业务程序实际 import/链接的依赖链”。检查二者，不能仅通过根包没有直接 import 就声称依赖完全隔离。

## 6. 用户侧体验与必须实现的 API

### 6.1 无 tag、纯语义注释

```go
// 创建用户时提交的信息。
type CreateUserRequest struct {
    // 用户名。
    // @openapi required nonnull minLength=3 maxLength=32 examples=["alice"]
    Name string
}

// 返回给客户端的用户信息。
type User struct {
    // 用户编号。
    // @openapi examples=[1024]
    ID int64

    // 用户名。
    Name string
}
```

原有 handler 示例：

```go
// 创建用户
//
// 创建成功后返回用户信息。
// @openapi tags=["用户"]
func CreateUser(c *gin.Context) {
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, APIError{
            Code:    "INVALID_JSON",
            Message: "请求体格式错误",
        })
        return
    }

    n := utf8.RuneCountInString(req.Name)
    if n < 3 || n > 32 {
        c.JSON(http.StatusBadRequest, APIError{
            Code:    "INVALID_NAME",
            Message: "用户名长度必须为 3～32 个字符",
        })
        return
    }

    c.JSON(http.StatusCreated, User{ID: 1024, Name: req.Name})
}
```

这是已有业务代码的示意，不是要求生成器插入该校验。实际示例必须补齐 `APIError`、imports、入口和测试，成为完整可编译应用。

最简单的 handler 只有普通说明也必须可用；`@openapi tags` 可省略，`@openapi.operation` 不存在于必需接入条件中。

### 6.2 Gin 路由不改，启动层挂载一次

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/openapi-golang/openapi"
    ginswagger "github.com/openapi-golang/gin-swagger"
    "github.com/openapi-golang/gin-swagger/examples/basic/internal/apidoc"
)
```

```go
r := gin.New()
r.POST("/users", CreateUser) // 原有注册方式不变。

doc, err := ginswagger.Mount(r, apidoc.Bundle(), ginswagger.Config{
    OpenAPI: openapi.Config{
        Title:   "用户服务",
        Version: "1.0.0",
    },
    Path: "/docs",
})
if err != nil {
    return err
}
_ = doc
```

`OpenAPI` 配置与框架的 `Path/UI/Middlewares` 分开，不复制一份核心配置模型到适配器中。适配器可提供少量类型别名改善使用体验，不得用重复结构维持两套标准模型。

默认提供：

```text
GET /docs/
GET /docs/openapi.json
```

挂载在全部业务路由注册完成后、开始处理请求之前执行。

### 6.3 目标公共接口

核心接口：

```go
// 在已知路由与生成的契约之间完成链接并构建文档。
func Build(bundle Bundle, routes []Route, cfg Config) (*Document, error)
```

框架中立的 Route 至少包含 method、规范化 OpenAPI path、Bundle 中的模板键，以及必要的来源、声明或扩展信息。对外名称统一使用 `OperationKey` 表示模板键，不与最终 `operationId` 混为一谈。

适配器接口：

```go
// 读取已注册路由并构建文档，不注册文档路由。
func Build(r *gin.Engine, bundle openapi.Bundle, cfg Config) (*openapi.Document, error)

// 构建文档并将文档页面挂载到指定引擎。
func Mount(r *gin.Engine, bundle openapi.Bundle, cfg Config) (*openapi.Document, error)
```

文档接口至少包括：

```go
func (d *Document) JSON() []byte
func (d *Document) WriteFile(path string) error
func (d *Document) Report() Report
```

最终 `Document` 使用不可变快照或返回防御性副本；不能把共享 map/slice 暴露为可变状态。相同 Bundle 可安全用于多个 Engine，不互相污染。

生成文件的 `apidoc.Bundle()` 返回 `openapi.Bundle`；不得返回 Gin 专属结构，也不得要求应用 import 编译器。

## 7. 必须公开且稳定的适配器契约

### 7.1 运行时契约

核心提供框架中立的 Bundle、OperationKey、Route、构建配置、诊断和文档类型。

适配器从框架取得路由和 handler 证据，解析原始路径，确定匹配模板，然后交给核心。核心只处理规范化路由和通用契约，不识别 `*gin.Context`，不对 `:id` 或 `*filepath` 执行 Gin 语法解析。

Bundle 必须通过公开、只读或防御性复制的 API 暴露匹配所需的索引信息。不能把关键索引全部藏在核心 internal 中，再让 Gin 适配器依赖不可访问的数据。[S14]

框架识别字符串可以出现在来源或兼容性记录中，但不能成为核心执行某套框架逻辑的开关。

### 7.2 生成时契约

`openapi/compiler` 至少公开以下可组合能力：项目加载选项、标准类型/符号视图、前端注册、调用分析上下文、类型投影请求、框架中立事实输出、来源诊断、编译及生成写入结果。

`gin-swagger/compiler` 提供可注册的 Gin 前端。同一前端由 Gin CLI 和项目自定义生成器共同使用，不能复制两套规则。

框架前端负责识别本框架 handler 和 API 调用，向核心输出通用效果，如读取命名参数、绑定某媒体类型到某 Go 类型、设置响应状态、写出某种 payload。核心负责调度、控制流/值传播、摘要缓存、Schema 与冲突检查。

接口必须允许与 Gin 不同的 handler 形态，例如函数返回响应值或 error。不能在“通用”接口中暗含所有 handler 都是 `func(*Context)` 或都通过 `c.JSON` 输出的假设。

公开标准库类型或稳定的查询视图；避免将第三方 SSA 节点、核心内部实现细节和大而全的 context 对象变成长期 SDK。所有共享类型、所有权、线程安全、失败方式、版本兼容和资源预算写入 `openapi/docs/adapter-sdk.md`。

### 7.3 不做动态插件平台

注册使用普通 Go 代码和显式导入。不要使用 Go 动态 plugin、通过任意字符串运行可执行文件、隐式下载适配器或全局 init 自注册。

核心 `openapi` CLI 不捆绑 Gin 前端，也不能承诺仅加 `--framework=gin` 就自动识别一个没有编译进二进制的适配器。

## 8. 以测试证明跨框架，而不是只写接口

在核心测试中，从一个临时外部消费模块仅导入公开 SDK，实现一个最小非 Gin 测试前端。它应分析真实 Go fixture，使用与 Gin 不同的 handler 形式，例如 `(Request) (Response, error)`，输出中立 Bundle 并通过中立 Route 构建最终文档。

该测试必须同时证明：

- 不需要导入 Gin、Fiber、Echo 或 gin-swagger。
- 不访问核心 internal；不使用已手写完成的 OpenAPI JSON 假装分析成功。
- 不需要修改核心实现就能注册新的前端。
- 能复用注释解析、Schema、来源诊断、文档构建与 UI 资源。
- 不能以函数地址、net/http 接口或 Gin context 为唯一通用入口。

可以使用纯标准库和小型 fake API；这是 SDK 合约测试，不是第三个正式框架适配包。不要在本次范围内实现完整 Fiber/Echo 适配器。

## 9. 编译与运行的两阶段模型

### 9.1 生成阶段

由 `gin-swagger generate` 组合 Gin 前端与通用编译器，完成 package 加载、注释、类型、可分析调用和契约投影，生成：

```text
internal/apidoc/zz_openapi.gen.go
```

生产应用中的生成文件只依赖核心轻量运行时。它不 import handler 包、不执行用户函数，不产生循环 import，不通过 init 注册全局状态。

生成器不执行用户 main，不启动数据库，不调用 handler，不运行用户 go generate，也不改变业务源码。类型、注释与构建信息通过静态加载获取。

### 9.2 启动阶段

Gin 适配器先对文档作用域内的真实路由做快照与证据匹配，转为框架中立 Route；核心链接模板、填充方法路径、裁剪组件、验证并生成缓存文档。

绑定器依赖 method/media-type 的情况可以在 Bundle 中保存有限的通用条件事实，到链接时确定。条件数据必须是明确的决策表，不嵌入 Gin 对象、业务闭包或任意可执行脚本；仍有歧义就诊断。

不把整个 AST/SSA 放进 Bundle，不在启动期重新运行分析器。

### 9.3 请求阶段

只读取缓存 OpenAPI 和共享 UI 资源。业务流量不经过文档采集链；文档请求也不触发类型扫描或 Schema 重建。

## 10. 注释协议与零 tag 的精确定义

### 10.1 不要求 tag，但必须忠实于网络数据

不要求用户新增 `json`、`binding`、`validate`、`swagger`、`openapi` 等 tag，主示例和全流程测试完全零 tag。

不从 `binding/validate` tag 构建本包的契约元数据系统。若原程序确实使用它们做校验，审计报告可承认存在未分析的校验来源，不能据此宣称完整掌握了业务行为。

已经存在的 `json/form/uri/header` 等影响真实编解码的 tag，允许由对应 WireCodec 兼容层读取，目的仅是避免生成与真实协议不一致的字段信息。不能修改这些 tag，也不能把它们变成使用前提。

核心管理标准编解码规则与 Schema 投影；Gin 特有绑定、JSON 后端选择和表单规则由适配器配置或实现。没有编解码证据就不推断全局改名。默认标准 JSON 中 `Name` 不能在文档里擅自变成 `name`。

用户关闭兼容读取、但实际类型存在会改变协议的 tag 时，产生诊断，不能静默输出错误 Schema。包内部为了序列化规范对象所使用的 json tag 不属于对用户 DTO tag 的依赖。

### 10.2 普通文字只描述语义

函数第一条非空普通注释行作为 summary，后续普通内容作为 description。字段、类型、常量的普通注释用于描述。支持 Unicode、段落、Markdown、前置/尾注释和块注释，并测试合并规则。

不要求注释带符号前缀，不裁剪名称，不从第一词推断字段名。普通文字不被正则或 LLM 转换成验证规则。结构化指令不进入 description。

### 10.3 单一结构化语法

仅使用：

```go
// @openapi required nonnull minLength=3 maxLength=32 examples=["alice"]
// @openapi format="email" deprecated
// @openapi operationId="users.create" tags=["用户"]
```

值采用 JSON；裸标志等价于 true。词法扫描器负责引号、转义、嵌套、空白和定位，不以空格 split 或一个巨型正则代替。数字使用无损表示，大整数不能经过 float64。

支持 JSON Schema 原有命名的常用约束与注解，例如 examples、default、enum、const、format、pattern、长度/数值/集合边界、readOnly/writeOnly、contentEncoding/contentMediaType。上下文指令包含 required、nonnull、nullable、operationId、tags、deprecated、ignore。

`nullable` 只作为源码便利语法，生成联合 null；不输出旧版 nullable 属性。required 是属性存在，nonnull 是禁止 null，二者都不等于必须非零。default 不会使生成器向业务注入默认值。

未知指令、冲突赋值、非法 JSON、不适用于当前类型的语义必须诊断。类型上的裸 `enum` 是明确封闭已知常量值域的指令；字段枚举使用 JSON 数组，不混淆两种上下文。

```go
// 用户角色。
// @openapi enum
type Role string

const (
    // 普通用户。
    RoleUser Role = "user"

    // 管理员。
    RoleAdmin Role = "admin"
)
```

没有显式封闭声明时，若干常量的存在不代表该类型只能取这些值。

### 10.4 必要时才使用兜底

```go
// @openapi request mediaType="application/json" type="CreateUserRequest" required
// @openapi response status=201 mediaType="application/json" type="User"
// @openapi response status="default" mediaType="application/json" type="APIError"
```

类型引用必须解析到真实 Go 类型；支持当前包和明确的模块限定名称，不强迫添加仅用于注释而导致 unused import 的导入，也不因注释自动联网下载任意模块。

复杂多态、条件 Schema、安全配置等由引用真实 Go 类型的集中 Go 扩展表达。常规用户不需要独立编写第二份 schema 或每条路由的 builder。

## 11. 事实、契约声明与未知信息必须分开

禁止采用“注释无条件覆盖代码”的全局优先级。对每个关键事实记录来源、源位置、适配器和适用条件。

| 类别 | 例子 | 处理 |
| --- | --- | --- |
| derived | 编解码字段名、明确类型、可求值响应码、实际路由 | 不允许注释改成与真实输出矛盾的结果 |
| declared | 必填、业务长度、说明、枚举、安全要求 | 可以补充，但不声称已证明服务端执行 |
| unresolved | 动态状态/类型、未知 codec、闭包歧义 | 报告；不得猜测或丢弃路由隐藏错误 |

可以记录样本契约测试的覆盖证据，但样本通过不等于形式化证明。不要制造没有统计依据的“95% 可信度”。

严格模式要求选中文档范围内的结构完整、一致且无未解决的关键事实，不意味着任意业务校验都已被证明。另提供审计报告标识声明与可识别实现证据之间的缺口。

必须测试矛盾声明，例如字符串 minimum、minLength 大于 maxLength、不可编码的示例、伪造字段改名以及实际输出含有 writeOnly 字段。

`readOnly/writeOnly/ignore` 不能充当隐藏实际网络数据的手段。接口可以显式退出文档范围；字段若仍会传输，不能通过忽略字段让一个闭合 Schema 伪装成完整协议。

## 12. Schema 与真实 WireCodec 投影

Schema 的缓存和身份至少考虑：Go 类型身份、泛型参数、输入/输出、媒体类型、codec profile 和影响结构的配置。

同一个类型允许有输入、输出、query、path、header、form、multipart 等不同投影。确实等价时再复用组件，不能强制“一种 Go struct 一个 Schema”。

必须覆盖基础数值/字符串/布尔、指针、数组、slice、map、命名类型、别名、递归、嵌入、any、可确定具体实现的接口、泛型实例和显式枚举。

### 12.1 必须正确处理的细节

- 输入非指针不自动等于 required；字段缺失、null、零值与空集合分别处理。
- 输出属性是否出现、是否允许 null，由实际编码行为决定；不能用输入规则代替。
- nil 指针、slice、map 和字段省略按 codec 区分；保留不存在与显式 null 的差别。
- 整数 key map 不一概拒绝；在支持其编码的 codec 中按字符串对象键表达。
- JSON 的 `[]byte`、原始二进制 body、multipart 文件分开；JSON RawMessage 不是字符串。
- time.Time、time.Duration、json.Number、自定义文本 key 等有明确映射和反例。
- 自定义 MarshalJSON/UnmarshalJSON/MarshalText/UnmarshalText 按方向与方法集处理；未知时需要 mapper，不执行方法探测。
- 匿名嵌入和字段冲突服从 codec 的实际选择，不用简单 allOf 代替。
- 默认不输出 additionalProperties=false；忽略未知输入字段不等于拒绝它们。
- 泛型默认具体化，组件命名稳定；递归引用不等于循环对象在运行时可被编码。
- JSON Schema false、true、联合 null、零数值边界、空列表和显式 null 必须无损保留。
- Go 正则、JSON Schema pattern、字符串字节长度与字符长度不自动等价。

请求 Schema 表达客户端应遵守的规范契约，不宣称穷尽 decoder 的宽松接受形式。对 null 宽松处理、大小写匹配、固定数组宽松解码等差异写入审计说明。

标准 JSON codec 可在核心复用。Gin 的 form、uri、header、multipart、第三方 JSON codec 配置由适配器识别并显式传给核心；不能让核心为了支持 Gin binding import Gin。

未知媒体类型、未知自定义序列化或关键网络名称不确定时必须诊断。可以提供集中 TypeMapper/WireCodec API，但普通 DTO 不需要手写 mapper。

### 12.2 独立 JSON Schema

`openapi schema` 必须从 Go 类型和注释生成独立、可验证的 Schema，带正确 dialect、$defs 和完整引用。

不能直接复制仍引用 `#/components/schemas/...` 的片段当独立文件。OAS 特有注解到所选 JSON Schema dialect 的保留/转换策略必须明确；对自定义方言声明支持边界，不假定所有验证器认识所有关键词。

## 13. 通用分析器与 Gin 前端

### 13.1 分析范围和预算

核心使用类型化 AST、go/types 与按需有界分析。Gin 前端识别项目范围内的候选 handler；可分析辅助函数建立参数化摘要，支持类型、常量、状态码和泛型实例代入。

使用完整包路径与 types 对象识别调用，不能按 `JSON`、`Success` 等函数名字碰巧相同就应用规则。

对跨函数深度、递归、循环、摘要数量、类型图、注释长度和引用展开设定预算。达到预算产生明确诊断，不截断后标成完整。不要第一版默认构建无界全程序 points-to 分析。

全局注释语法错误与使用相关的不确定事实分别处理。尚未注册的候选函数不应因为缺少 summary 或响应就导致另一个 Engine 的文档链接失败；最终完整性取决于实际选中的路由及其可达类型。

### 13.2 Gin 请求与参数语义

识别明确的 JSON、Query、URI、Header、Form、Multipart 绑定，含可识别的 ShouldBindWith/ShouldBindBodyWith 等显式绑定器。

支持 Param、Query、GetQuery、DefaultQuery、QueryArray、GetHeader、Cookie、PostForm、FormFile 等读取模式。参数名来自代码常量或明确绑定，不从注释自然语言猜测。

自动选择绑定器的 ShouldBind/Bind 保留方法和媒体类型条件，到链接阶段或明确配置后确定；不默认当 JSON。

请求体 required 的推导要考虑是否允许空体、绑定错误后是否继续成功路径等。字段 required 不等于 requestBody required。

ParseInt/Atoi 结果被使用不代表非法输入被拒绝；必须区分检查错误和忽略错误。`len(string)` 不直接生成 JSON Schema minLength。框架前端输出绑定/读取的通用事实，不在核心硬编码 Gin 方法名。

### 13.3 Gin 响应与控制流

覆盖常见 JSON/IndentedJSON/AsciiJSON/PureJSON、String、Data、DataFromReader、Status、Redirect、明确 Render，以及可分析的文件和流式输出。

其他渲染方法按具体媒体类型和 WireCodec 处理；不能把 JSON 的投影规则搬给 XML/YAML/TOML/ProtoBuf 后宣称支持。不能确定的自定义 Renderer 必须报告并可通过集中适配补足。

关键规则：

- 状态码、媒体类型、响应类型、已知响应头要保留来源。
- 同状态/媒体类型的重叠备选使用 anyOf；oneOf 需要已证明互斥或经过验证的显式声明。
- gin.H 的可分析字面量和有限修改可生成对象；动态 map/any 不能凭空补字段。
- Abort 不等于当前 Go 函数 return；Bind 与 ShouldBind 的副作用不同。
- 记录写入和状态提交顺序；连续写两个 JSON 不等于两个合法的备选响应。
- 无 body 与未知 body 区分；按 HTTP/Gin 规则处理 HEAD、204、304。
- 未知状态码不能伪装成 default；default 只来自明确兜底契约。
- SecureJSON 前缀、JSONP、混合字节输出不自动视为普通 JSON。
- 文件的真实类型、range、错误响应不能仅靠一次 c.File 调用全部猜出。

如需要对框架支持但前端不能完整自动分析的调用作出判断，输出专用诊断并提供集中规则，不以空 Schema 或固定 default 隐藏缺口。

### 13.4 Helper、middleware 与外部错误处理

常见 Success(c, value)、Created(c, value)、Response[T] 应尽量通过摘要识别。无法静态分析的项目 helper 可用 Go 扩展一次注册，不改业务调用。

不能因为 middleware 名称包含 Auth 就自动生成 bearerAuth/401/403。独立配置允许按路由作用域声明安全和公共错误，明确标为 declared。

不要把 Gin 的中间件模型固化到核心。未来框架可能通过返回 error 和全局 error handler 生成响应；SDK 必须能表达外部错误处理来源及不确定性，而非强迫全部转换成 Gin 模式。

## 14. Gin 路由快照、标识与链接

最终方法和路径来自 `Engine.Routes()`。静态注册分析只提供辅助线索，不能取代运行时动态前缀和配置开关的真实结果。[S8]

Gin 路由快照只直接暴露链中的末位 handler，不等于已经取得整条中间件链。对常规末位业务 handler 直接支持；复杂链结合可靠静态证据或集中绑定，不能伪称所有 middleware 行为已被观察。

### 14.1 标识规则

区分三种身份：

| 身份 | 用途 |
| --- | --- |
| 稳定源码符号与分析模板键 OperationKey | 查找生成阶段的契约摘要 |
| 运行时 handler 证据 | 判断某条注册路由对应哪个摘要 |
| 最终 operationId | OpenAPI 使用者引用接口的稳定标识 |

operationId 默认基于 method 与规范化路径，稳定处理碰撞；不依赖源码行号、遍历顺序或随机后缀。显式 operationId 冲突必须报告。函数改名不应无故改变默认对外 ID。

函数代码地址、运行时名称、源码位置均是证据，不是任意闭包或接收者实例的唯一身份。[S7]必须测试同工厂不同捕获值、同类型不同 receiver 状态、method value、别名、泛型、trimpath 和符号裁剪。

禁止简单删除 `.func1`、`-fm` 或泛型后缀就认定匹配成功。只有模板能覆盖实际行为且消歧可靠时链接。否则用集中路由绑定或项目 WrapperAnalyzer；不改路由注册语句。

### 14.2 路径与作用域

Gin 路径解析归适配器。正确处理普通参数、catch-all、转义/静态冒号、冲突、编码、框架初始化前后的路径表示差异。不得为读取路径而调用私有 API 或发送一个业务请求来初始化引擎。

catch-all 转为 OpenAPI path 时标识不可无损表示的跨斜杠语义和客户端限制；不能对 path 参数加 allowReserved 来假装解决。

支持动态 Group、多 Engine、同 handler 多路由、声明式文档 include/exclude、已生成但未注册的函数。未使用候选和另一个 Engine 的路由不能污染当前文档。

文档过滤只是呈现策略，不是接口鉴权。输出路径和 schema 默认不包含绝对本地源码路径、无关业务模型或调试数据。

### 14.3 Mount 生命周期与失败语义

Build 无路由注册副作用。Mount 先完成构建、验证、UI 资源准备与路径冲突预检查，再集中注册文档入口。

Gin 没有被本包控制的通用事务式路由回滚。不得把 recover(panic) 当成已经回滚；需要设计可证明的预检查和最小注册方案，并准确说明保证范围。可预判冲突必须在改变目标 Engine 前返回错误。

不覆盖业务路由，不修改全局 middleware，不暗中利用 NoRoute 截获其他路径。文档可添加自己的认证，但仍服从 Engine 已继承的 middleware。

挂载后新增路由不自动反映到已生成文档。禁止在服务中并发重建 Gin 路由树或加全局注册 hook；显式重建另一个未启动实例可以作为用户控制的工作流。

## 15. 原生 OpenAPI 3.2 规范能力归核心所有

`/docs/openapi.json` 是完整 OpenAPI 文档，不是单个 JSON Schema。不能把 3.1 模型的版本号改为 3.2 就宣称完成。

核心维护 `docs/openapi32-matrix.md`，按规范项记录“模型/表达入口/序列化/语义验证/独立验证/UI展示”。Gin 仓库维护自身自动推导矩阵，不复制一套规范模型或规则。

必须有可用模型及测试的范围：

| 范围 | 必须覆盖 |
| --- | --- |
| 根与引用 | 3.2.0、$self、jsonSchemaDialect、相对引用与基准 URI |
| HTTP 方法 | QUERY、additionalOperations、重复/冲突检查 |
| 参数 | path/query/header/cookie/querystring、序列化、schema/content 限制 |
| 媒体类型 | itemSchema、可复用 mediaTypes、正确引用 |
| multipart | encoding、prefixEncoding、itemEncoding 及组合限制 |
| 标签 | summary、parent、kind 与层级错误 |
| 示例 | Schema examples；Example Object 的 value/dataValue/serializedValue/externalValue |
| 多态 | discriminator/mapping/defaultMapping |
| XML | 3.2 XML Object 与 nodeType |
| 安全 | Security Scheme 的 deprecated、oauth2MetadataUrl、deviceAuthorization 及相关字段 |
| 高级对象 | callbacks、webhooks、links、servers、扩展 |
| Schema | 布尔、联合、组合、条件、$defs/$id/$anchor/$dynamicRef 等 |

OpenAPI 3.2 指定的默认 Schema 方言仍是：[S1]

```text
https://spec.openapis.org/oas/3.1/dialect/base
```

不能根据版本号拼出另一个地址。

Reference Object 与 Schema 的 $ref 分开建模。显式 false、null、0、空 security 等不能被默认省略丢失；先定义存在性表示，再写序列化。

复杂功能允许通过公开、类型化的 Go 扩展表达，且引用现有 Go 类型。只给一个 RawMessage 容器不算实现高级能力；每项应有构造入口、正例和反例。规范可表达不等于 Gin 前端必须自动推导所有高级行为，两份能力矩阵必须分别说明。

### 15.1 流式数据

NDJSON 每项通常是一个 JSON 值；SSE 每项是解析后的事件，data 是字符串。内含 JSON 的 SSE data 使用内容媒体类型和内容 Schema 表达，不把业务 DTO 当成整个事件。

用真实流样本验证事件名、id、retry、多行 data、注释和字符串 payload。未发送的事件字段不能凭空 required。框架流 API 的行为由适配器分析，共同的文档表示与验证归核心。

## 16. Bundle、版本兼容与首次生成

### 16.1 一个共享 Bundle 协议

只定义一种核心 Bundle 格式。至少记录格式版本、规范版本、生成器与前端版本、能力需求、稳定模板索引、Schema、来源/诊断、构建 profile 与源码指纹。

Bundle 中不存 Gin 对象、指针地址、原始 AST、可执行闭包或机器专属路径。框架相关信息以命名空间数据/来源或有限的中立条件表达，不要求核心 import 适配器才能解码。

核心与适配器分别版本化；同时记录 SDK 兼容范围和 Bundle reader/writer 兼容范围。遇到不支持的必需能力或新格式要清晰失败；不能静默丢弃字段、自动当成旧版文档。

测试同一兼容范围内的生成器/运行时组合，以及确定不兼容时的错误。不要只判断两边版本字符串是否完全相同，也不能无条件接收任意未来格式。

### 16.2 确定性与新鲜度

同源码、配置和依赖生成字节一致。排序、组件名与模板键不能依赖绝对路径、时间戳、随机数或遍历顺序。

指纹包含所有影响分析的输入，包括已跟踪源码、注释、类型依赖、有效模块解析、构建条件和 codec 配置。工作区引用的本地源码也要纳入，不仅记录 go.mod 声明版本。

排除生成文件自身和无关输出，避免自引用变化。只原子更新本工具拥有的文件，失败不留下半份产物。

CI 有源码时重新编译检查新鲜度；运行时没有源码，不能只凭 Bundle 自带 hash 就证明当前代码与文档一致。

### 16.3 首次生成可成功

当业务应用已经 import apidoc 而生成目录不存在时，仍需完成首次生成。使用经过验证的 overlay 或可恢复的引导策略；失败恢复现场，不能留下空 Bundle 冒充成功。

go/packages 按项目实际构建条件加载；默认不改 go.mod/go.sum、不运行项目脚本、不执行用户函数。生成输出路径相对项目目录解析，明确多 package、workspace、replace、build tags、GOOS/GOARCH、GOEXPERIMENT 的处理规则。

测试删除本工具生成目录后再次完整生成，包含 Gin CLI 从全新 checkout 的构建路径。

## 17. 两个 CLI，复用同一个编译引擎

### 17.1 核心命令

```bash
openapi schema --dir . \
  --type 'example.com/project/api.User' \
  --projection response \
  --output user.schema.json

openapi check --spec ./openapi.json
openapi version
```

核心 CLI 可执行纯类型 Schema、规范校验和通用报告功能，不内置 Gin 分析。命令中示例业务 module path 是用户项目的类型引用，不是本项目的 module path。

### 17.2 Gin 命令

```bash
gin-swagger generate --dir . --output ./internal/apidoc
gin-swagger check --dir . --output ./internal/apidoc
gin-swagger check --spec ./openapi.json
gin-swagger explain --dir . --symbol 'example.com/project/api.CreateUser'
gin-swagger version
```

适配 CLI 组合核心 SDK；注释词法、Schema 算法、诊断格式和规范校验不能各写一遍。允许小量命令入口/flag 编排代码不同，但不复制公共业务逻辑。

`check --dir` 只检查分析与生成物；不假装知道只有应用运行时才能确定的完整路由。最终文档通过用户构造的 Router + Build 导出，再执行 check --spec。

对自定义 helper 提供普通 Go 生成入口示例，显式组合核心编译器、Gin 前端和项目规则。不可为使用自定义规则而要求修改每个业务 handler。

两个 CLI 都提供准确 help、退出码、机器可读 JSON 诊断、可取消执行和资源上限。version 输出自己的版本、关联核心/前端版本及 Bundle 能力，不能硬编码永远显示 dev 或假定运行的就是源码版本。

## 18. 共享 Swagger UI 与默认安全策略

共享资源、页面/config 生成、常规安全处理归 `openapi/swaggerui`，Gin 只负责生命周期和框架挂载。以后 Fiber/Echo 接入包不得复制 UI 分发目录和页面模板。

该子包按需导入。直接 import `openapi` 构建文档时不嵌入 UI。

### 18.1 不绑定单一 HTTP 框架

可以提供 net/http 便利 handler，但必须同时提供框架中立的只读资源及渲染结果访问方式。未来不是 net/http 的框架也应能复用同一份静态资源、页面配置和安全元数据，而不被迫把业务请求改为 net/http。

先用小型非 net/http 消费测试验证资源 API，再锁定最小接口。不为未来框架预先制造复杂传输抽象或第二套 HTTP 服务器。

### 18.2 固定资源与安全默认值

实施时核对真实 Swagger UI 版本及 OpenAPI 3.2 能力，固定经过验收的资源版本与校验和，保留上游许可证。不得将旧 Goal 的版本字符串当成无需核验的最新版本，也不得使用 CDN latest。

生产 Go 应用不依赖 Node、npm、外网或源码；Node/浏览器仅用于开发与验收。

默认至少为：

```text
validatorUrl: null
persistAuthorization: false
queryConfigEnabled: false
supportedSubmitMethods: []
```

仅设置 tryItOutEnabled=false 不算禁用实际提交。[S9]开启调试必须显式配置并通过浏览器测试；QUERY/扩展方法的可提交性依据所选 UI 实测，不强行宣称支持。

文档和静态资源处理前缀、尾斜杠、反向代理、OAuth redirect、MIME、ETag、缓存和路径编码。禁止 remote validator、查询串覆盖配置和未授权远端文档加载。

不把不可信内容拼进脚本；使用正确编码，测试 CSP、nosniff、Markdown/HTML 注入和路径遍历。CSP 要与真实 UI 行为兼容，不能为了看起来严格而使页面不可用。

不能为迎合 UI 修改规范版本或删除有效 3.2 字段。分别记录标准支持、核心表达、Gin 推导、UI 展示、实际提交能力。

### 18.3 验证器与网络安全

规范检查默认禁止自动获取外部 $ref/externalValue；可显式配置允许列表，并限制重定向、文件访问、循环和体积，避免读取本地敏感文件或发起任意网络请求。

外部验证库是交叉检查后端，不是核心公共数据模型。锁版本，使用真实 fixtures 验证支持范围；规范正文要求不能因为库不支持就被略过。

## 19. 诊断与可解释性

诊断必须包含稳定 code、severity、相对源位置、类型/函数/路由、来源事实、原因和修复方式。核心标准诊断与 Gin 特有诊断使用清晰命名空间，共用格式与聚合机制。

至少覆盖：非法注释、未知指令、类型不匹配、未知 codec、关键响应未解决、非法状态组合、handler 歧义、路径冲突、缺失响应、重复 operationId、Bundle 不兼容、生成物过期、构建条件不匹配、UI 展示缺口。

`explain` 应回答某个字段/响应/接口从哪里来、用了哪个规则、哪些声明尚未得到实施证据，以及怎样集中补足。不要只输出“解析失败”。

严格模式不能以开放空 Schema、虚构 default 或丢弃接口掩盖未知。明确声明 any 或开放对象则是合法契约，不应与意外丢失类型混淆。

未知高级业务模式“清晰诊断 + 集中适配 + 验证用例”是允许的产品边界；“任意 Go 程序都能全自动理解”不是交付承诺。

## 20. 测试与两个模块的独立完成标准

### 20.1 核心测试

| 测试组 | 必须证明 |
| --- | --- |
| 注释 | 普通语义无名称前缀；单一 DSL；定位、转义、Unicode、精度、错误处理 |
| 类型 | 无 tag 的字段命名、泛型、递归、别名、枚举、嵌入、null、map key、bytes |
| 编解码 | 输入/输出投影、自定义方法集、codec profile、矛盾声明 |
| 标准 | OAS 3.2 功能矩阵逐项正反例，独立 Schema 引用有效 |
| Bundle | 确定性、格式兼容、能力检查、不可变性、多次链接 |
| 文档 | 中立 Route 构建；引用裁剪、稳定命名、扩展字段和存在性 |
| 跨框架 | 外部模块通过公开 SDK 实现非 Gin 前端，不修改核心 |
| UI | 纯核心消费和非 net/http 资源消费均可工作 |
| 安全 | 外部引用默认拒绝、资源预算、路径遍历、配置注入 |
| 边界 | 核心 module 和测试中无 Gin/Fiber/Echo/适配器依赖 |

核心必须在 Gin 仓库完全不在磁盘上时仍能完成自身完整测试。禁止核心 CI 为常规测试反向 checkout Gin 适配器。

### 20.2 Gin 接入测试

| 测试组 | 必须证明 |
| --- | --- |
| 最小体验 | 零 tag、无 Operation marker、原 handler 和路由方式不变 |
| 生成 | 从源码生成 Bundle，首次无 apidoc 也能完成，生成后编译成功 |
| 请求 | JSON/Query/URI/Header/Cookie/Form/Multipart；绑定条件和错误分支 |
| 响应 | JSON/String/Data/状态/重定向/文件能力边界/流式 |
| 控制流 | Abort 后继续、Bind 隐含写入、状态提交、连续 body 写入 |
| 合并 | 重叠备选不被错误 oneOf 拒绝；未知状态不自动 default |
| helper | 普通和泛型 helper、集中适配、分析预算超限 |
| 身份 | 普通/未导出函数、方法、别名、泛型、同地址不同捕获 |
| 路径 | 动态 Group、多 Engine、多路由、catch-all、转义、冲突 |
| 无侵入 | 挂载前后相同业务请求的状态/头/body/副作用保持一致 |
| 挂载 | 预判冲突无修改，文档鉴权不影响其他路径，继承 middleware 如实反映 |
| 构建 | 真实 Go1.27.1/Gin1.12.0、trimpath、符号裁剪、已声明平台 |
| UI | 浏览器真实加载，中文 summary 和模型可见，无外部网络 |

“无侵入”测试还应验证生成器未改写用户 handler/路由 AST；测试时分别构造等价实例，避免重复业务请求自己引入的状态变化污染比较结果。

### 20.3 黑盒链路

必须存在以下完整链路，不能仅做 golden JSON 快照：

```text
真实 Go fixture
 → gin-swagger generate
 → 编译生成的应用
 → 构造真实 Gin Router
 → Build 导出 OpenAPI
 → 独立规范与 Schema 检查
 → httptest 业务响应/流式解析验证
 → 离线 Swagger UI 浏览器验证
```

独立验证库覆盖不足的规范项用明确的自有规则与反例补足；不能大量 skip 后宣称全支持。浏览器确实访问本地示例，测试实际提交默认关闭及显式开启两种状态，禁止页面外连。

### 20.4 contracttest

框架无关测试工具接收调用方提供的请求/响应样本、文档和必要解码配置，不要求生产 middleware。提供标准 net/http 辅助，但底层样本模型可用于其他服务器模型。

不主动扫描生产环境、不自动调用破坏性接口、不把未覆盖的状态标为已验证。SSE/NDJSON 先按真实格式解析再验证对应 itemSchema。

### 20.5 可复现、性能与资源限制

重复生成字节一致；源码目录移动不带来机器路径噪声；修改相关注释或本地依赖源码会触发过期检查。

记录 100/1000 路由规模的生成、Build、文档读取与内存基准，区分核心和适配阶段。业务请求不经过文档逻辑通过结构和比较测试证明，不编造“零纳秒开销”或无实测吞吐数字。

注释、路径、Schema/引用解析有 fuzz；文档并发访问、共享 Bundle、多 Engine 有 race 测试。CI 的 fuzz 运行有时间预算，长期 fuzz 是后续维护工作，不要求在一次任务内无限运行。

## 21. 依赖与双仓库引导顺序

### 21.1 本地开发

共享 workspace 只用于并行改动两仓库。workspace 内测试通过不代表模块的 require 列表正确或远端可安装。[S3]

尚无可下载核心版本时，可以先在显式本地 workspace 中完成引导和纵向联调。确需临时 modfile/replace 时只用于明确记录的本地测试，完成后清理；不可把不存在的版本称为已发布依赖。

不要在发布 go.mod 中留下：

```go
replace github.com/openapi-golang/openapi => ../openapi
```

也不能依赖父目录的 module cache 中恰好存在一份未提交核心。

### 21.2 转为真实远端依赖

核心的可消费提交完成测试、秘密检查并同步到正确远端后，取得真实 commit SHA。Gin 模块用 Go 工具解析该提交对应的可解析伪版本，或者使用已实际存在且验证过的稳定版本。

条件满足后的典型步骤：

```bash
CORE=/Users/whylost/private-projects/openapi
ADAPTER=/Users/whylost/private-projects/gin-swagger
CORE_SHA="$(git -C "$CORE" rev-parse HEAD)"

# 此 SHA 必须已真实存在于目标远端，而不只是本地提交。
(
  cd "$ADAPTER"
  GOWORK=off go get "github.com/openapi-golang/openapi@$CORE_SHA"
  GOWORK=off go mod tidy
)
```

让 Go 生成合法版本，不手工编造时间戳伪版本，也不预先写不存在的 v0.1.0/v1.0.0 当最终依赖。核心后续修改必须重新提交、同步、更新依赖并回归，不能测试 workspace 新代码却让最终 go.mod 指向旧代码。

默认仅同步代码，不创建正式版本 tag 或 GitHub Release。真实提交的伪版本足以验证两个模块的独立消费；以后经授权发布时再按 SemVer 设置正式版本。

### 21.3 私有仓库访问

按当前认证方式配置最小必要访问。GOPRIVATE 如有需要，只作用于指定组织并保留已有设置；不要全局关闭公共依赖校验，不将 token 写入 URL、go.mod、go.sum 或日志。

CI 的跨私有仓库读取权限必须实测，不能假定一个仓库的默认 GITHUB_TOKEN 可读取另一个私有仓库。缺权限时报告失败位置，不扩大到组织管理员权限，也不自动将仓库公开。

### 21.4 独立冷环境验收

在新的临时目录或 CI runner 中分别 checkout 两个仓库，设置 `GOWORK=off`，使用干净模块缓存。验证适配器通过 go.mod 固定版本从远端解析核心，不依赖相邻源码。

两个验收场景都必须通过：

1. 只有 openapi 源码：构建核心 CLI、完整核心测试及外部 SDK 消费测试。
2. 只有 gin-swagger 源码：下载核心固定版本，构建 Gin CLI、生成示例并完成集成测试。

权限或网络导致冷环境验收失败时，保留本地已完成成果并标明缺口，不能宣称双仓库独立发布验收已通过。

## 22. 版本、CI 与发布策略

两个仓库独立 SemVer，不强迫版本号一致。核心 SDK 的兼容范围、Bundle 格式范围、适配器要求的最低核心版本分别记录。

最小 CI 包含：

- 精确 Go 1.27.1；Gin 项目精确 Gin 1.12.0。禁止工具链自动升级掩盖下限不兼容。
- 已声明的更新 Go/Gin 组合；更新不能擅自提升最低要求。
- 单元/集成、race、vet、生成物新鲜度、规范验证、依赖边界和冷环境模块消费。
- 实际可执行 runner 上的平台测试；交叉编译不算验证了目标平台的运行时符号行为。
- 离线浏览器用例、受控 fuzz、依赖安全检查和资源许可证检查。

核心变更先通过核心和外部前端测试，再由适配器升级固定依赖并回归。跨仓库联调工作流可以由适配器侧显式 checkout 两个指定提交，但不能代替单仓库的正常 CI，也不能引入核心到适配器的依赖环。

GitHub Actions 与开发工具版本锁定到可复现引用。README 中不要使用未存在的 release badge、下载链接或声称已经公开的 pkg.go.dev 文档。

MIT 可作为新代码的默认许可证；若组织已指定其他兼容许可证，遵循现有规范并记录。第三方 Swagger UI、规范相关素材等保留各自许可和 NOTICE，不把它们一律重新标为本项目许可证。

## 23. 实现顺序与阶段性交付

先阅读代码与约束，再以可验证增量推进。每项关键行为先写失败测试，再实现和回归；不要先铺几十个空接口。

| 阶段 | 工作 | 必须产生的结果 |
| --- | --- | --- |
| A | 环境、身份、目录、两个 module、认证/远端检查 | 正确仓库与路径；明确权限；不破坏现有工作 |
| B | 关键假设与最小 SDK | 真正验证编码、handler 身份边界、首次生成和公开前端接口 |
| C | 双仓库纵向切片 | 零 tag 源码经 Gin CLI 生成核心 Bundle，原路由上显示真实 UI |
| D | 核心产品化 | 完成 DSL、类型/codec 投影、标准模型、通用分析服务、诊断与 Bundle |
| E | Gin 产品化 | 请求响应、helper、控制流、动态路由、消歧、作用域与挂载 |
| F | 完整 3.2 与共享 UI | 标准矩阵、高级构造入口、流式/上传验证、离线和安全测试 |
| G | 真正跨框架/跨模块验证 | 外部非 Gin 前端、非 net/http 资源消费、无反向依赖、远端固定版本 |
| H | 独立验收与交付 | 冷环境构建、全量测试、双仓库 CI、README、限制与真实日志 |

核心和适配器可以交替推进，不必在第一阶段把核心所有抽象一次性定死。只有由测试证明需要的能力才进入公共 SDK；跨仓库接口变更时先更新规范与合约测试，再同步两边。

不要每个阶段都停下来询问是否继续；非阻塞细节做合理决定并记录。真正缺少权限、身份、可见性策略、目标工具链或依赖时说明确切阻塞，不擅自改变目标来制造通过。

多代理仅用于接口已确定、文件边界独立的任务。由主执行者维护依赖契约并完成集成审查；不让不同代理各写一份不兼容的 Bundle/Schema/Config。

## 24. 必须存在的验证入口与执行命令

在文档中提供可复现的脚本；以下路径与 CLI flag 是目标要求，需要在实现中兑现。二进制输出到临时目录，避免污染 Git。

### 24.1 开发期双仓库验证

```bash
BASE=/Users/whylost/private-projects
WORK="$BASE/openapi-golang.work"
VERIFY_TMP="$(mktemp -d)"

(
  cd "$BASE/openapi"
  GOWORK="$WORK" go test ./...
  GOWORK="$WORK" go build -o "$VERIFY_TMP/openapi" ./cmd/openapi
)

(
  cd "$BASE/gin-swagger"
  GOWORK="$WORK" go test ./...
  GOWORK="$WORK" go build -o "$VERIFY_TMP/gin-swagger" ./cmd/gin-swagger
)
```

这不是独立验收。临时目录仅存本次输出，结束时由脚本安全清理。

### 24.2 核心独立验收

在已准备精确目标工具链的单模块环境执行：

```bash
go version
GOWORK=off go mod verify
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...

GOWORK=off go test ./internal/comment -run '^$' \
  -fuzz=FuzzDirective -fuzztime=30s

GOWORK=off go test ./internal/verify \
  -run 'TestNoFrameworkDependencies|TestRuntimeDependencyBoundary|TestExternalFrontend' \
  -count=1

GOWORK=off go run ./cmd/openapi check \
  --spec ./testdata/golden/openapi32-full.json
```

测试名必须有真实实现，不能通过没有匹配测试而“成功”。`openapi32-full.json` 是标准验证 fixture，不是生产 schema-first 输入。

### 24.3 Gin 独立验收

```bash
go version
GOWORK=off go list -m github.com/gin-gonic/gin
GOWORK=off go list -m github.com/openapi-golang/openapi
GOWORK=off go mod verify
GOWORK=off go test ./...
GOWORK=off go test -race ./...
GOWORK=off go vet ./...

GOWORK=off go run ./cmd/gin-swagger generate \
  --dir ./examples/basic --output ./internal/apidoc

GOWORK=off go run ./cmd/gin-swagger check \
  --dir ./examples/basic --output ./internal/apidoc

GOWORK=off go test ./internal/integration -count=1
GOWORK=off go test ./internal/routes -run '^$' \
  -fuzz=FuzzRoutePath -fuzztime=30s

GOWORK=off go test ./internal/verify \
  -run 'TestRuntimeDependencyBoundary|TestNoLocalReplace|TestGeneratorBootstrap' \
  -count=1
```

另外验证真实导出的 OpenAPI，而不只检查标准 golden；执行两个 CLI 的远端固定版本安装和无相邻仓库消费测试。浏览器脚本、版本与启动/关闭流程固定且可重复，失败保留必要日志。

Windows、非默认架构、无法使用 race 的环境必须单独记录实际执行情况；不能把跳过列为通过。校验真实工具链版本，不能只看 go.mod 的 go 行。

## 25. 文档、可维护性与成果记录

两个仓库保存同一 Goal ID 的需求快照，禁止分别维护互相矛盾的总体目标。公共 SDK 规范以核心 `docs/adapter-sdk.md` 为权威，适配器按其固定核心提交引用和测试。

核心至少提供：中文 Quick Start、根 API、compiler SDK、如何写新前端、独立 Schema、OAS 3.2 矩阵、Bundle 兼容、UI 资源、安全与诊断说明。

适配器至少提供：最小零 tag 接入、完整可运行示例、CLI、helper 适配、路由与闭包限制、middleware 说明、原逻辑不变的验证方式、Gin 版本/codec 矩阵。

两个仓库都有简洁的 `docs/design.md`、`docs/plan.md`、`docs/status.md`、`docs/verification.md`，分别记录设计决策、待完成任务、现状和执行证据，不复制大量流水账。

README 必须区分自动推导、显式声明、集中适配、无法无歧义分析四类能力；明确未来 Fiber/Echo 适配只是扩展方向，而非本次已经支持。

记录真实仓库状态、分支、提交、固定依赖版本、Go/Gin/工具版本、实际命令与退出结果。日志不能包含 token、用户私有数据或未经脱敏的绝对源码路径。

## 26. 完成标准与最终回复

以下项目全部满足才可宣布目标完整验收：

| 交付 | 完成条件 |
| --- | --- |
| 两个仓库 | 本地路径、Git 根、module、远端身份正确；存在真实源码和提交 |
| 依赖方向 | 核心无框架依赖；适配器只通过公开 SDK 使用核心 |
| 核心可用 | 不带 Gin 也能编译类型、构建/验证文档、复用 UI |
| Gin 可用 | 零 tag、语义注释、原路由和 handler 不变、生成与挂载完整 |
| 标准 | 3.2 矩阵有实际表达入口和正反测试，不只是版本字符串 |
| 跨框架 | 非 Gin 外部前端合约测试与框架中立资源消费通过 |
| 工程化 | 初次生成、确定性、诊断、版本、冷环境独立构建通过 |
| UI | 真实、离线、固定资源，默认禁止实际提交，浏览器验证通过 |
| 安全 | 无凭证泄漏、外部引用默认拒绝、挂载冲突和输入安全有测试 |
| 维护 | 两份清晰 README、SDK/支持矩阵、许可证和真实验收记录 |

不允许用占位 panic、空方法、固定示例文档、虚假的 dependency version、空测试名或大面积 skip 充当完成。

有些任意业务模式不能自动推导时，可以作为明确产品边界，但其诊断、适配方式和测试必须完成。缺少仓库权限、网络、工具链或浏览器执行条件时，交付实际完成部分，列出失败命令、影响和复现步骤，不能声称整个目标已经验收。

最终回复明确列出：两个仓库真实位置/远端/提交；实现能力；公共 API；核心和适配器各自执行的验证；冷环境依赖结果；仍存在的限制及环境阻塞。不要以创建文件数量代替质量证明。

## 27. Codex 开始执行指令

读取本文件，将其作为此次双仓库实现的目标与验收标准。

先检查目标环境、目录、现有指令与权限，保护已有代码；创建或确认两个正确的 Git 仓库和 Go module；验证关键假设与公开 SDK；然后依次完成纵向切片、核心、Gin 适配、标准矩阵、UI、安全、独立消费测试和最终审查。

不要只输出方案，不停留于 MVP，不继续沿用旧的单仓库 ginoas 架构。只实现这两个产品仓库；未来框架通过真实的公开契约测试预留，不提前扩张范围。

贯穿全部工作的原则：

**代码提供结构，注释补充契约，真实编解码决定网络事实；框架差异止于适配器，共同能力留在核心；未知就诊断，不猜测、不改业务。**

---

## 28. 实现参考与事实核对

本 Goal 的项目身份、版本下限、架构边界与验收标准是用户要求和本次设计决策；以下官方资料用于核对语言、框架、规范和工具行为。资料检查日期：2026-09-05。开始实现时重新核对实际工具与依赖，但不要自行改变固定目标。

- [S1] OpenAPI 3.2.0 正文：规范对象、Schema 方言、引用、参数、媒体类型与安全要求。
- [S2] OpenAPI 官方升级说明：识别 3.2 功能面，不能替代正文。
- [S3] Go Modules Reference：module、require/replace、workspace、GOWORK 和私有依赖。
- [S4] Go 发布记录：确认最低工具链的真实版本，不靠日期猜测。
- [S5] go/packages 文档：源码和类型加载能力与限制。
- [S6] encoding/json 文档：默认字段名、map key、自定义序列化等实际协议行为。
- [S7] reflect 文档：函数代码地址不足以唯一标识任意函数值。
- [S8] Gin v1.12.0 固定源码：路由表及末位 handler、生命周期与编解码配置。
- [S9] Swagger UI 配置：validator、授权存储、查询配置和实际提交控制。
- [S10] GitHub CLI：明确 owner、可见性、source 和 remote 的创建语义。
- [S11] Go 模块发布说明：模块路径、版本和可下载消费流程。
- [S12] GitHub Actions token 文档：工作流凭证和权限边界。
- [S13] JSON Schema 官方说明：组合与引用语义。
- [S14] Go 命令文档：internal 目录的可见性限制。

```text
[S1] https://spec.openapis.org/oas/v3.2.0.html
[S2] https://learn.openapis.org/upgrading/v3.1-to-v3.2.html
[S3] https://go.dev/ref/mod
[S4] https://go.dev/doc/devel/release
[S5] https://pkg.go.dev/golang.org/x/tools/go/packages
[S6] https://pkg.go.dev/encoding/json
[S7] https://pkg.go.dev/reflect#Value.Pointer
[S8] https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/go.mod
     https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/gin.go
     https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/context.go
     https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/binding/form_mapping.go
     https://raw.githubusercontent.com/gin-gonic/gin/v1.12.0/codec/json/json.go
[S9] https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/
[S10] https://cli.github.com/manual/gh_repo_create
[S11] https://go.dev/doc/modules/publishing
[S12] https://docs.github.com/en/actions/tutorials/authenticate-with-github_token
[S13] https://json-schema.org/understanding-json-schema/reference/combining
      https://json-schema.org/understanding-json-schema/structuring
[S14] https://pkg.go.dev/cmd/go#hdr-Internal_Directories
```

上面的 SDK、目录和示例 API 是本项目必须实现的目标，不是声称已有第三方包暴露了这些接口。旧 GINOAS_CODEX_GOAL.md 的零 tag、纯语义注释、非侵入、准确推导和标准验收约束已在本文件重述；执行不依赖旧文件。
