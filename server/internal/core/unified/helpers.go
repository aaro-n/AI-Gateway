package unified

import "encoding/json"

// ContentString 从 Message.Content (json.RawMessage) 中提取纯文本。
// Content 可能是 string，也可能是 []ContentBlock。
func ContentString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// 尝试 string
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// 尝试 []ContentBlock
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		var result string
		for _, b := range blocks {
			if b.Type == "text" {
				result += b.Text
			}
		}
		return result
	}
	return ""
}

// ContentBlocks 从 Message.Content 中解析为 ContentBlock 列表。
// 如果 content 是 string，会包装成单个 text block。
func ContentBlocks(raw json.RawMessage) []ContentBlock {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []ContentBlock{{Type: "text", Text: s}}
	}
	var blocks []ContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		return blocks
	}
	return nil
}

// StringContent 将字符串包装为 json.RawMessage
func StringContent(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// BlocksContent 将 ContentBlock 列表序列化为 json.RawMessage
func BlocksContent(blocks []ContentBlock) json.RawMessage {
	b, _ := json.Marshal(blocks)
	return b
}

// RawJSON 将 json.RawMessage 解析为 interface{}，用于安全放入 map[string]interface{} 后再序列化。
// 直接放 json.RawMessage 到 map 中会被 encoding/json base64 编码，所以需要先反序列化一层。
func RawJSON(raw json.RawMessage) interface{} {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
