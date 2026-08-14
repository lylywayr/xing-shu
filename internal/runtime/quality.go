package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Quality struct {
	Transport      bool         `json:"transport_success"`
	Protocol       bool         `json:"protocol_success"`
	Content        bool         `json:"content_success"`
	Structured     bool         `json:"structured_success"`
	ToolCompatible bool         `json:"tool_compatible"`
	SemanticMatch  bool         `json:"semantic_match"`
	Conclusion     string       `json:"conclusion"`
	Cause          FailureCause `json:"failure_cause"`
}

func EvaluateResponse(raw, request, original []byte) Quality {
	q := Quality{Transport: true}
	if !json.Valid(raw) {
		q.Cause = CauseProtocol
		q.Conclusion = "invalid_json"
		return q
	}
	var x struct {
		Choices []struct {
			Message struct {
				Content   any   `json:"content"`
				ToolCalls []any `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &x) != nil || len(x.Choices) == 0 {
		q.Cause = CauseProtocol
		q.Conclusion = "missing_choices"
		return q
	}
	q.Protocol = true
	content := fmt.Sprint(x.Choices[0].Message.Content)
	q.Content = strings.TrimSpace(content) != ""
	q.ToolCompatible = validateTools(x.Choices[0].Message.ToolCalls, request)
	q.Structured = structuredOK(request, content)
	q.SemanticMatch = semanticMatch(original, content)
	if !q.Content {
		q.Cause = CauseCapability
		q.Conclusion = "empty_content"
	} else if !q.Structured {
		q.Cause = CauseProtocol
		q.Conclusion = "structured_output_invalid"
	} else if len(original) > 0 && !q.SemanticMatch {
		q.Cause = CauseCapability
		q.Conclusion = "semantic_similarity_below_threshold"
	} else {
		q.Conclusion = "quality_checks_ok"
	}
	return q
}
func validateTools(calls []any, req []byte) bool {
	var root struct {
		Tools []struct {
			Function struct {
				Name       string         `json:"name"`
				Parameters map[string]any `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if json.Unmarshal(req, &root) != nil {
		return false
	}
	declared := map[string]map[string]any{}
	for _, tool := range root.Tools {
		if tool.Function.Name != "" {
			declared[tool.Function.Name] = tool.Function.Parameters
		}
	}
	for _, value := range calls {
		item, ok := value.(map[string]any)
		if !ok {
			return false
		}
		fn, ok := item["function"].(map[string]any)
		if !ok {
			return false
		}
		name, ok := fn["name"].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return false
		}
		schema, ok := declared[name]
		if !ok {
			return false
		}
		args, ok := fn["arguments"].(string)
		if !ok || !json.Valid([]byte(args)) {
			return false
		}
		var object map[string]any
		if json.Unmarshal([]byte(args), &object) != nil || !validateToolObject(object, schema) {
			return false
		}
	}
	return len(calls) == 0 || len(declared) > 0
}
func validateToolObject(object map[string]any, schema map[string]any) bool {
	if schema == nil {
		return true
	}
	if required, ok := schema["required"].([]any); ok {
		for _, item := range required {
			name, ok := item.(string)
			if !ok {
				return false
			}
			if _, exists := object[name]; !exists {
				return false
			}
		}
	}
	properties, _ := schema["properties"].(map[string]any)
	for name, value := range object {
		rule, ok := properties[name].(map[string]any)
		if !ok {
			continue
		}
		kind, _ := rule["type"].(string)
		if !toolValueType(value, kind) {
			return false
		}
	}
	return true
}
func toolValueType(value any, kind string) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "integer":
		n, ok := value.(float64)
		return ok && n == float64(int64(n))
	case "number":
		_, ok := value.(float64)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return true
	}
}
func structuredOK(req []byte, content string) bool {
	var x struct {
		ResponseFormat any `json:"response_format"`
	}
	_ = json.Unmarshal(req, &x)
	if x.ResponseFormat == nil {
		return true
	}
	return json.Valid([]byte(content))
}
func semanticMatch(original []byte, candidate string) bool {
	if len(original) == 0 {
		return true
	}
	var x struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(original, &x) != nil || len(x.Choices) == 0 {
		return true
	}
	base := fmt.Sprint(x.Choices[0].Message.Content)
	a := tokens(base)
	b := tokens(candidate)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range a {
		seen[v] = true
	}
	hit := 0
	for _, v := range b {
		if seen[v] {
			hit++
		}
	}
	return float64(hit)/float64(len(a)) >= 0.2 || float64(hit)/float64(len(b)) >= 0.35
}
func tokens(s string) []string {
	raw := strings.Fields(strings.ToLower(s))
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.Trim(v, ".,!?;:()[]{}\"'`，。！？；：（）【】")
		if len(v) >= 2 {
			out = append(out, v)
		}
	}
	return out
}
