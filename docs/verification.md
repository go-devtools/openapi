# 验证记录

## 环境预检

- `uname -srm`：Darwin 27.0.0 arm64，退出 0。
- `go version`：go1.26.3 darwin/arm64，退出 0；不满足目标精确工具链。
- `gh --version`：命令不存在，退出 127。
- 适用父目录未发现 AGENTS.md；遵循会话提供的简体中文、无子 agent、中文注释要求。
- 父目录 `git rev-parse --show-toplevel`：不是 Git 仓库，退出 128；未初始化父目录。
- Go 1.27.1 官方归档请求 HTTP 200；限时探测下载超时，不计为工具链验证。
- Gin 模块代理返回 v1.12.0，源码提交 73726dc606796a025971fe451f0aa6f1b9b847f6。
- GitHub 连接器两个仓库元数据请求均为 404，组织列表为空；可能是授权范围不足，未执行创建操作。
- 指定核心远端 `git ls-remote`：Authentication failed；适配器远端：terminal prompts disabled。尚未获得任何远端提交。

## 实现验证

尚未执行产品测试、构建、UI 或冷环境独立验收。任何未执行项均不算通过。

## 已解除的环境限制

- 隔离工具链 `go version`：go1.27.1 darwin/arm64，退出 0。精确工具链从 Go 官方模块服务取得并通过 Go 校验。
- GitHub CLI v2.100.0 官方 arm64 安装包 SHA256：45f9a62da2f6e641a7fad57e2ce39656dfd7ef331372d80a2a2aed65abb01642，校验通过；用户选择 SSH，无需 gh 登录。
- `ssh -T git@github.com`：成功认证 Rainer-Yu；GitHub 不提供 shell，状态 1 是该探测的预期行为。
- 用户登录内置浏览器后，组织页面确认原先没有仓库；已创建 openapi 和 gin-swagger 两个 private 仓库，均未初始化远端文件。
- 两个本地 origin 均为对应的 git@github.com:openapi-golang/名称.git；两个 `git ls-remote origin` 均退出 0、无引用，符合新建空仓库。

## 首批实现增量

精确 Go 1.27.1、GOWORK=off：`go test ./...` 退出 0。覆盖根包、compiler、internal/comment、internal/validate、spec 五组包。每组新增行为先观察到缺失 API 的编译失败，再实现通过。

源码投影首次失败于 RawMessage 的真实类型别名；核对 Go 1.27.1 的 v2_stream.go 与 jsontext/value.go 后增加 jsontext.Value 原始 JSON 映射，`go test ./compiler` 退出 0。未执行用户的自定义序列化方法。

`compiler.TestReturnValueFrontend` 使用临时真实 Go 源码的 `(Request) (Response, error)` 形态，复用注释、Schema、生成和 Build，验证源码说明改变使生成物过期；这还不是最终外部模块 SDK 验收。
