package runtime

import "testing"

func TestEvaluateResponseRequiresChoicesAndContent(t *testing.T) {
	q := EvaluateResponse([]byte(`{"id":"x","choices":[]}`), []byte(`{"messages":[]}`), nil)
	if q.Protocol || q.Content {
		t.Fatal("empty choices must fail")
	}
	q = EvaluateResponse([]byte(`{"choices":[{"message":{"content":"ok"}}]}`), []byte(`{"messages":[]}`), nil)
	if !q.Protocol || !q.Content || !q.SemanticMatch {
		t.Fatal("valid content must pass")
	}
}
func TestToolCallsValidateShape(t *testing.T) {
	q := EvaluateResponse([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"bad":true}]}}]}`), []byte(`{"tools":[{}],"messages":[]}`), nil)
	if q.ToolCompatible {
		t.Fatal("malformed tool call must fail")
	}
}
func TestToolCallsMustMatchDeclaredTool(t *testing.T) {
	req := []byte(`{"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}]}`)
	raw := []byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"delete_file","arguments":"{\"path\":\"x\"}"}}]}}]}`)
	if EvaluateResponse(raw, req, nil).ToolCompatible {
		t.Fatal("undeclared tool must fail")
	}
}

func TestToolCallsValidateDeclaredSchemaArguments(t *testing.T) {
	req := []byte(`{"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"},"lines":{"type":"integer"}},"required":["path"]}}}]}`)
	missing := []byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"read_file","arguments":"{}"}}]}}]}`)
	wrongType := []byte(`{"choices":[{"message":{"tool_calls":[{"function":{"name":"read_file","arguments":"{\"path\":42}"}}]}}]}`)
	if EvaluateResponse(missing, req, nil).ToolCompatible || EvaluateResponse(wrongType, req, nil).ToolCompatible {
		t.Fatal("invalid tool arguments must fail schema validation")
	}
}
