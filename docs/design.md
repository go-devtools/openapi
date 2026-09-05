# 设计决策

目标权威为 [GOAL.md](../GOAL.md)，Goal ID 为 `openapi-golang/two-repository-code-first-v1`。

核心的轻量运行时负责不可变 Bundle、标准模型和中立路由链接；compiler 通过公开的标准库类型视图提供项目加载、注释、类型投影及有界分析服务。Gin 的生成前端、运行时证据匹配、路径语法和 codec 选择只存在于适配器。

源码生成与启动链接分离；请求期只读取缓存文档与资源。SDK 使用显式 Go 注册，诊断区分 derived、declared、unresolved。未知关键事实必须阻止选中路由成功链接，不能变为开放空 Schema 或虚构 default。

公开 SDK 先以真实外部前端测试约束，再由 Gin 复用。模型使用显式存在性与 Schema 布尔分支保留 null、false、零和空数组。Swagger UI 作为独立可选子包，资源以防御性复制接口提供给非 net/http 消费者。

按用户要求不启用子 agent。测试使用精确 Go 1.27.1，开发构建入口统一为 make dev；最终验收使用 GOWORK=off 和远端实际提交解析出的固定版本。不创建正式 tag 或 Release。

README.md 使用英文，并维护内容对应的 README.zh-cn.md。GitHub 描述使用英文。项目新增代码采用 MIT，上游资源保留其原许可证。
