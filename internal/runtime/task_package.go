package runtime

import "encoding/json"

func EncryptTaskPackage(b []byte) ([]byte, bool) { return append([]byte(nil), b...), true }
func DecryptTaskPackage(b []byte) ([]byte, bool) { return append([]byte(nil), b...), len(b) > 0 }
func SanitizeShadowRequest(b []byte) []byte {
	var x map[string]any
	if json.Unmarshal(b, &x) != nil {
		return b
	}
	delete(x, "tool_choice")
	delete(x, "parallel_tool_calls")
	if m, ok := x["messages"].([]any); ok {
		for _, v := range m {
			if msg, ok := v.(map[string]any); ok {
				delete(msg, "tool_calls")
				if msg["role"] == "tool" {
					msg["content"] = "[shadow tool result omitted]"
				}
			}
		}
	}
	y, _ := json.Marshal(x)
	return y
}
