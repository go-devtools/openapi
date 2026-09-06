package openapi

import (
	"fmt"
	"github.com/openapi-golang/openapi/spec"
	"net/url"
	"sort"
	"strings"
)

// HEAD 的网络响应没有正文；解析共享响应后复制，保留 GET 与组件原值。
// HEAD wire responses have no body; resolve and copy shared responses while preserving GET and components.
func projectHEADResponses(operation *spec.Operation, components spec.Components) error {
	statuses := make([]string, 0, len(operation.Responses))
	for status := range operation.Responses {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	for _, status := range statuses {
		entry := operation.Responses[status]
		seen := map[string]bool{}
		description, summary := "", ""
		for entry.Reference != nil {
			ref := entry.Reference.Ref
			if seen[ref] {
				return fmt.Errorf("HEAD 响应引用存在循环：%s", ref)
			}
			seen[ref] = true
			if summary == "" {
				summary = entry.Reference.Summary
			}
			if description == "" {
				description = entry.Reference.Description
			}
			fragment, err := url.PathUnescape(strings.TrimPrefix(ref, "#/components/responses/"))
			if err != nil || !strings.HasPrefix(ref, "#/components/responses/") {
				return fmt.Errorf("HEAD 响应需要可解析的本地响应引用：%s", ref)
			}
			name := strings.ReplaceAll(strings.ReplaceAll(fragment, "~1", "/"), "~0", "~")
			var ok bool
			entry, ok = components.Responses[name]
			if !ok {
				return fmt.Errorf("HEAD 响应引用不存在：%s", ref)
			}
		}
		if entry.Value == nil {
			return fmt.Errorf("HEAD 响应对象缺失：%s", status)
		}
		response := copyJSON(*entry.Value)
		if description != "" {
			response.Description = description
		}
		if summary != "" {
			response.Summary = summary
		}
		response.Content = nil
		operation.Responses[status] = spec.Inline(response)
	}
	return nil
}
