# 官方方言资源

本目录保存 OpenAPI Initiative 发布的原始 JSON 字节，按上游 Apache 2.0 许可证分发，见 LICENSE。它们只嵌入可选的 contracttest 包，不会进入核心运行时根包或 Swagger UI。

| 文件 | 来源 | SHA-256 |
| --- | --- | --- |
| oas31-dialect.json | https://spec.openapis.org/oas/3.1/dialect/base | 8a0e89e365dadbebce2921ce6244340c1090e9d544c60d977e9ad6b97a61227b |
| oas31-meta.json | https://spec.openapis.org/oas/3.1/meta/base | 267a88226e64e96dfc8c89dbd7e863160c84715e0fb893ca1d9fbf9f830f1f54 |
| oas32-dialect.json | https://spec.openapis.org/oas/3.2/dialect/2025-09-17 | 4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273 |
| oas32-meta.json | https://spec.openapis.org/oas/3.2/meta/2025-09-17 | a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c |

3.1 方言通过官方 URI 获取并固定内容；3.2 文件与已有独立规范测试中的固定资源一致。更新需要重新核对来源、许可证、校验和和兼容测试。资源预载不启用外部 HTTP 或文件加载器。OAS 注解不自动成为实例断言；完整 OpenAPI 文档校验仍由规范检查路径负责。
