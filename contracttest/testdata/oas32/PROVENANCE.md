# 官方验证资源

这些测试资源来自 OpenAPI Initiative，保持原始字节，按上游 Apache 2.0 许可证使用。它们只由测试读取，不会嵌入生产根包或 Swagger UI。

| 文件 | 固定来源 | SHA-256 |
| --- | --- | --- |
| schema.json | https://spec.openapis.org/oas/3.2/schema/2025-11-23 | 7d48f01f37eeae4799041b371ad5f533f9f533fd2b0caa1011a8ba27c5b48b70 |
| schema-base.json | https://spec.openapis.org/oas/3.2/schema-base/2025-11-23 | 423daa88e2285fa343856c08502fe63fd8aa3674cd5b4ef88746ba6f82647af3 |
| dialect.json | https://spec.openapis.org/oas/3.2/dialect/2025-09-17 | 4e2c989f3d1e6489d41bc1ca4ade11743278c612b82fba9144c6116c79f1c273 |
| meta.json | https://spec.openapis.org/oas/3.2/meta/2025-09-17 | a1959c0aa1f9a7ce58f2b75699be5bc5187c8a25901fe04a227f1d9419ba4e9c |

`schema-base` 要求显式使用表中的固定方言，完整标准样例以该方言交叉验收。产品仍遵循 OAS 3.2 正文规定的默认 `https://spec.openapis.org/oas/3.1/dialect/base`，不能用这个测试文件反向修改规范默认值。

许可证来源：https://raw.githubusercontent.com/OAI/OpenAPI-Specification/3.2.0/LICENSE 。测试使用独立 JSON Schema 2020-12 引擎，其网络和文件 loader 均关闭。此结构校验不能替代跨字段语义、应用行为或引用目标有效性检查。
