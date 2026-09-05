# 双仓库实施计划

规范：[GOAL.md](../GOAL.md)。本计划由当前任务按增量执行，所有关键行为先加入失败测试，再实现和回归；不启用子 agent。

- [ ] A：确认真实 Mac、父目录指令、精确工具链、两个独立 Git 根与 module、组织权限和正确远端。环境证据保存在 verification.md。
- [ ] B：在核心 spec、bundle、compiler 与 internal/comment 建立公开契约，验证 JSON 字段、存在性、注释扫描、类型视图和外部非 Gin 前端。Gin 的 internal/resolve 验证函数身份边界。
- [ ] C：实现 compiler、两个 CLI 和 examples/basic，使用零 tag 真实源码首次生成 Bundle，经原有 Engine.Routes 链接并挂载共享 UI；集成测试核对业务行为不变。
- [ ] D：补齐 internal/comment、Schema codec 投影、有界调用调度、来源诊断、Bundle 兼容、独立 Schema 与原子确定性输出。正反例覆盖 GOAL 第 10—13、16—17 节。
- [ ] E：补齐 Gin 请求响应、helper、控制流、路径、歧义、作用域与挂载前检查。internal/integration 与 routes fuzz 覆盖 GOAL 第 13—14 节。
- [ ] F：逐项实现 spec 和语义验证的 3.2 矩阵，contracttest 验证真实 SSE/NDJSON，固定 UI 资源、校验和及许可证；浏览器验证离线、注入和提交开关。
- [ ] G：外部非 Gin 前端、非 net/http UI 消费和依赖链检查；核心测试与秘密检查后正常推送，用 Go 从真实 SHA 解析适配器依赖。
- [ ] H：两个独立冷缓存 checkout 执行 GOAL 第 24 节完整命令、race/vet/fuzz、真实 UI、基准和安全检查；锁定 CI 引用、补齐支持矩阵与中文文档，核对提交与最终依赖。

每个阶段仅在 verification.md 有实际命令、退出结果和输出摘要时勾选。仅本地 workspace 通过不能勾选 G/H。失败保留现场与证据，不降低版本下限、不改变仓库可见性。
