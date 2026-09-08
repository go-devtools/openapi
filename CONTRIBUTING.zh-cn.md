# 贡献与版本发布

[English](CONTRIBUTING.md)

`go-devtools/openapi` 和 `go-devtools/gin-swagger` 都从 **v0.0.1** 开始，各自独立发版。Go module 的实际版本由不可移动的 Git tag 确定；`VERSION` 表示当前分支的目标版本。适配器依赖真实发布的核心版本，提交中禁止使用本地 `replace`。

## 分支与合并方向

| 分支 | 用途 | 来源 |
| --- | --- | --- |
| `main` | 稳定集成，默认主分支 | `release/M.m` 的 PR |
| `develop` | 下一次次版本或主版本开发，初始 `v0.1.0-dev` | `feature/*`、`fix/*`、`docs/*`、`chore/*`、`refactor/*`、`test/*`、`ci/*`、`build/*`、`dependabot/*`、`sync/*` 的 PR |
| `release/M.m` | 单条版本线的稳定化与维护，初始 `release/0.0` | `prepare/vM.m.p` 或同版本线 `hotfix/M.m.p` 的 PR |
| `hotfix/M.m.p` | 下个补丁版本的独立修复，初始 `hotfix/0.0.2` | `fix/*`、`docs/*`、`test/*`、`chore/*`、`dependabot/*` 的 PR |

功能分支从 develop 创建，当前版本的修复从对应 hotfix 创建。普通贡献可来自 fork；发布与晋升分支必须位于本仓库。main、develop、release 线要求 PR 和 **Required checks**，禁止强推和删除；版本标签禁止修改和删除。

提交信息使用英文 Conventional Commits，例如 `feat: add ...`、`fix: handle ...`、`docs: explain ...`、`ci: verify ...`。新增需要解释的逻辑采用简洁的中英双语注释。公开文档和 OpenAPI 示例文字使用英文，README 保留对应中文版。

## 本地验证

使用 Go 1.27.1 和 Node 24.20.0：

```sh
GOWORK=off go mod download
GOWORK=off make dev
node --test scripts/release.test.mjs
node scripts/release.mjs policy
```

完整 CI 还执行 race、vet、有界 fuzz、漏洞扫描和离线浏览器验证。push 与手动运行会在独立消费者中验证真实远端提交。fork PR 无需私有模块凭据，也不使用提权的 `pull_request_target`。详见 [CI 文档](docs/ci.md)。

## 发版流程

1. 新版本线在 develop 完成，补丁在对应 hotfix 完成。适配器需要更新核心时，先发布核心，再固定其真实 tag。
2. 从 main 手动运行 **Prepare release**，输入新版本，如 `v0.0.2` 或 `v0.1.0-rc.1`。按匹配的 hotfix、已有 release 线、develop 的顺序选择来源，创建进入 `release/M.m` 的版本 PR，并显式启动 CI。审查后在 **Required checks** 通过时合并。新 release 线从 develop 截取；后续开发继续独立进行。
3. 当前或更新的正式版本线通过 CI 后，自动创建进入 main 的 PR。审查合并后，main 的同一提交须通过 CI，才自动创建附注 tag。预发布版本和较旧维护线通过同等门禁后直接从对应 release 线发布。已有版本不会重新打标。
4. **Release** 核对 tag 和 CI，在 Linux、macOS 上关闭 workspace，通过公开 Go proxy 与校验数据库无凭据安装固定版本；交叉构建六种 CLI 压缩包，上传源码、SHA-256 校验和及 Release 草稿；下载实际附件，在 Linux/amd64、macOS/arm64、Windows/amd64 原生运行检查命令后再公开发布。Windows 的此项验证不代表完整库测试覆盖。最大的正式版本成为 Latest；预发布和旧维护补丁不会覆盖它。
5. 发布后自动准备下个 hotfix。develop 尚不存在时创建下一次版本开发线，否则按需创建修复回流 PR。真实代码冲突由维护者处理；继续开发前合并修复。同步失败可单独重试，不改动已发布标签和附件。

自动化只使用仓库作用域的 `GITHUB_TOKEN`，无需个人令牌。须在仓库 **Settings → Actions → General** 允许 Actions 创建 PR。GitHub 不会因这个令牌的普通写入再触发工作流，因此脚本显式调度 CI 与 Release。不自动批准或合并 PR。

## 失败恢复与兼容性

- 构建或附件验证失败时保留草稿。基础设施恢复后重试失败作业；源代码有变化须发布新版本，不移动已有 tag。
- 自动打标中断可重试源 CI 或 Promote release 作业；tag 已存在而未开始发布时，以该 tag 手动调度 Release。不能从分支直接执行 Release。
- 同步失败在排除原因后重试 sync 作业；已有工作分支保留。
- v0 API 仍在演进：固定版本并阅读发布记录。补丁只应包含兼容修复，有意的 API 变化放到 develop 的下个次版本。未来 v2 及以后须使用对应 `/vN` 模块路径。
- 压缩包包含 MIT 和第三方许可证。业务依赖仍通过 `go get` 引入，不需要下载可执行文件。
