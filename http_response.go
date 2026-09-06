package openapi

import (
	"fmt"
	"github.com/openapi-golang/openapi/spec"
	"net/url"
	"sort"
	"strings"
)

// HEAD wire responses have no body; resolve and copy shared responses while preserving GET and components.
// HEAD 的网络响应没有正文；解析共享响应后复制，保留 GET 与组件原值。
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
				return fmt.Errorf("HEAD response reference cycle: %s", ref)
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
				return fmt.Errorf("HEAD requires a resolvable local response reference: %s", ref)
			}
			name := strings.ReplaceAll(strings.ReplaceAll(fragment, "~1", "/"), "~0", "~")
			var ok bool
			entry, ok = components.Responses[name]
			if !ok {
				return fmt.Errorf("HEAD response reference does not exist: %s", ref)
			}
		}
		if entry.Value == nil {
			return fmt.Errorf("HEAD response object is missing: %s", status)
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
