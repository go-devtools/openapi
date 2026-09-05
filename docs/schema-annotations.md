# Schema 检查与源码声明

`openapi.Check` / `CheckWithOptions` 检查原始文档中标准关键字的值、对象上下文和引用。`compiler.Project.Schema` 还会检查源码声明与已经推导出的网络类型、数组长度和数值边界是否冲突。两者检查的对象不同。

## 原始规范是否合法

JSON Schema 允许不适用于当前实例类型的约束，也允许没有任何实例能满足的 Schema。因此以下两个原始 Schema 都合法：

```json
{"type":"string","minimum":1}
```

```json
{"type":"string","minLength":10,"maxLength":2}
```

前者的 minimum 不约束字符串，后者无法被任何字符串满足。原始规范检查不把这些情况作为语法错误。enum 的空数组和重复值也不会被强制拒绝；规范对此使用建议性要求，而不是禁止性要求。

标准关键字必须使用合法值：例如 type 必须是合法类型名或非空且不重复的类型数组，长度必须是非负整数，multipleOf 必须严格为正。组合和 prefixItems 必须是非空 Schema 数组，items 是单个 Schema。properties / $defs 等字典中的键始终是属性或定义名称，包括以 x- 开头的名称。

JSON 数字按原始十进制文本判断，不转换为浮点数，也不按指数展开大整数。原始检查可以识别 `100.00e-2` 为整数、`100.00e-3` 为非整数；极大正负指数同样受原始输入字节预算控制。

本检查不对 pattern 的字符串内容实施完整 ECMA-262 语法验证。format 的断言、样本是否满足 Schema，以及自定义方言的完整语义仍需独立验证能力；不能把关键字检查通过等同于完整标准验收。

## 源码声明是否与事实相容

自动推导的 string 字段声明 `minimum=1` 会返回 `openapi.comment.type`；源码中的 minLength 大于 maxLength 返回 `openapi.comment.range`。这些诊断属于源码契约，不会改变原始 JSON Schema 的合法性定义。

检查在完整组件图建立后进行，字段指向命名类型的引用仍能获得其类型与边界。常见诊断包括：

| 编码 | 情况 |
| --- | --- |
| `openapi.comment.type` | 约束不适用于实际网络类型 |
| `openapi.comment.range` | 已声明的上下界没有交集，包括字段引用与组件边界 |
| `openapi.comment.derived` | 覆盖固定数组长度，或放宽类型推导的数值下界 |
| `openapi.comment.value` | 非法关键字值，例如 required=5、minLength=null、multipleOf=0 |
| `openapi.comment.budget` | 精确数值边界比较超过编译期预算 |

上下文标志 required / nullable / nonnull / ignore 必须是布尔值。required 只用于字段。nullable 对已包含 null 的直接类型联合保持幂等。原先对实际输出 writeOnly、伪造 type 和隐藏真实字段的拒绝仍保留。

编译期边界比较最多支持 4096 位数值文本和绝对值不超过 4096 的十进制指数，超过范围返回明确预算错误，不进行大规模整数展开。这是源码声明比较的实现预算，不是 JSON Schema 对原始数值的限制。

当前范围检查覆盖标准投影、命名组件、以及用于 null 的简单联合。任意复杂组合的可满足性判断、完整示例可编码性校验、引用联合的 nullable / nonnull 改写、所有来源位置和数值类型化边界仍未完成，状态见验证记录。约束声明不是服务端已执行对应业务校验的证明。

## 验证与依据

`testdata/golden/schema-keywords.json` 的 52 个正反例由自有检查器和固定官方 OpenAPI 3.2 元 Schema 分别验证。源码回归使用真实 Go 类型与注释，另覆盖命名引用、固定数组、无符号数、精确大整数及极端指数。数字分类 fuzz 使用有界的独立精确有理数作为对照。

规范依据：[JSON Schema 2020-12 Validation](https://json-schema.org/draft/2020-12/json-schema-validation) 的 type、enum、数值和长度关键字，以及 [Core](https://json-schema.org/draft/2020-12/json-schema-core) 的组合关键字。官方元 Schema 的固定来源与校验和见 [PROVENANCE.md](../contracttest/testdata/oas32/PROVENANCE.md)。
