package contracttest

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// 在独立引擎前检查 JSON 数据、递归深度、节点数和精确数值的计算预算。
// Checks JSON data, depth, node count, and exact-number computation budgets before the independent engine.
func guardJSON(value any, maxDepth, maxNodes int, maxBytes int64) error {
	nodes := 0
	remaining := maxBytes
	var walk func(any, int) error
	charge := func(size int64) error {
		if maxBytes == 0 {
			return nil
		}
		remaining -= size
		if remaining < 0 {
			return fmt.Errorf("openapi.contract.budget: 已解码样本超过内容字节预算")
		}
		return nil
	}
	walk = func(value any, depth int) error {
		nodes++
		if depth > maxDepth || nodes > maxNodes {
			return fmt.Errorf("openapi.contract.budget: JSON 深度或节点数超过预算")
		}
		switch v := value.(type) {
		case map[string]any:
			if err := charge(2 + int64(max(len(v)-1, 0))); err != nil {
				return err
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := charge(int64(len(key) + 3)); err != nil {
					return err
				}
				if err := walk(v[key], depth+1); err != nil {
					return err
				}
			}
		case []any:
			if err := charge(2 + int64(max(len(v)-1, 0))); err != nil {
				return err
			}
			for _, item := range v {
				if err := walk(item, depth+1); err != nil {
					return err
				}
			}
		case string:
			return charge(int64(len(v) + 2))
		case json.Number:
			text := string(v)
			if len(text) > 4096 {
				return fmt.Errorf("openapi.contract.budget: 数值文本超过 4096 字节")
			}
			if text == "" || !(text[0] == '-' || text[0] >= '0' && text[0] <= '9') || !json.Valid([]byte(text)) {
				return fmt.Errorf("openapi.contract.json: 非法 JSON 数字")
			}
			if index := strings.IndexAny(text, "eE"); index >= 0 {
				exponent, err := strconv.Atoi(text[index+1:])
				if err != nil || exponent > 4096 || exponent < -4096 {
					return fmt.Errorf("openapi.contract.budget: 数值指数绝对值超过 4096")
				}
			}
			return charge(int64(len(text)))
		case float32:
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				return fmt.Errorf("openapi.contract.json: 数字不是有限值")
			}
			return charge(int64(len(strconv.FormatFloat(float64(v), 'g', -1, 32))))
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("openapi.contract.json: 数字不是有限值")
			}
			return charge(int64(len(strconv.FormatFloat(v, 'g', -1, 64))))
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return charge(int64(len(fmt.Sprint(v))))
		case bool:
			if v {
				return charge(4)
			}
			return charge(5)
		case nil:
			return charge(4)
		default:
			return fmt.Errorf("openapi.contract.json: 不支持的 JSON 数据类型 %T", value)
		}
		return nil
	}
	return walk(value, 0)
}
