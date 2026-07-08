// Package schemaclean 提供 JSON Schema 跨协议清理，确保兼容 Gemini API 等有限制的后端。
// 参考: Sub2API antigravity/schema_cleaner.go
package schemaclean

import (
	"encoding/json"
	"strings"
)

// ForGemini 清理 JSON Schema 以适配 Gemini API 限制：
//   - 类型大写：object → OBJECT（Gemini 历史版本要求）
//   - format 映射：date → date-time
//   - 移除 $defs / definitions（不支持 $ref）
//   - 展开 oneOf / anyOf 为宽松的 wrapper（Gemini 不严格支持）
//   - 移除 const / additionalProperties / $schema / strict（不兼容字段）
//
// 返回清理后的 json.RawMessage。如果输入无效则返回原始 JSON。
func ForGemini(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return raw // 非 JSON 对象，原样返回
	}
	cleaned := cleanForGemini(schema)
	result, err := json.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return result
}

func cleanForGemini(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	// 0. 预处理：type 大写（Gemini 老版本要求 "OBJECT"/"STRING" 大写）
	if t, ok := schema["type"].(string); ok && t != "" {
		schema["type"] = strings.ToUpper(t)
	}
	// format 映射（Gemini 不直接支持 "date"，需转为 "date-time"）
	if f, ok := schema["format"].(string); ok && f == "date" {
		schema["format"] = "date-time"
	}

	// 1. 提取 $defs / definitions 后删除（避免 Gemini 报错）
	defs := extractDefs(schema)

	// 2. 展开 $ref
	if len(defs) > 0 {
		flattenRefs(schema, defs)
	}

	// 3. 递归处理
	result := make(map[string]any, len(schema))
	for k, v := range schema {
		switch k {
		case "const", "$defs", "definitions", "$ref", "additionalProperties", "$schema", "strict":
			// 删除不兼容字段
		case "oneOf", "anyOf":
			// 改进：保留第一个选项的 type/properties，合并所有选项的 description 和 enum
			// 这比粗暴地展开为空壳能保留更多有用信息。
			merged := mergeOneOfAnyOf(v)
			for mk, mv := range merged {
				if _, exists := result[mk]; !exists {
					result[mk] = mv
				}
			}
			if _, hasType := result["type"]; !hasType {
				result["type"] = "object"
			}
			if _, hasProps := result["properties"]; !hasProps {
				result["properties"] = map[string]any{}
			}
		case "enum":
			// 保留 enum 但移除空数组（Gemini 兼容）
			if arr, ok := v.([]any); ok && len(arr) == 0 {
				// 跳过空 enum
			} else {
				result[k] = v
			}
		default:
			result[k] = cleanValue(v)
		}
	}

	// 4. 确保 type 和 properties 都存在（裸 schema）
	if _, hasType := result["type"]; !hasType {
		if _, hasProps := result["properties"]; hasProps {
			result["type"] = "object"
		}
	}

	return result
}

func extractDefs(schema map[string]any) map[string]any {
	defs := make(map[string]any)
	for _, key := range []string{"$defs", "definitions"} {
		if d, ok := schema[key].(map[string]any); ok {
			for k, v := range d {
				defs[k] = v
			}
			delete(schema, key)
		}
	}
	return defs
}

func flattenRefs(schema map[string]any, defs map[string]any) {
	if len(defs) == 0 {
		return
	}
	if ref, ok := schema["$ref"].(string); ok {
		delete(schema, "$ref")
		parts := strings.Split(ref, "/")
		refName := parts[len(parts)-1]
		if defSchema, exists := defs[refName]; exists {
			if defMap, ok := defSchema.(map[string]any); ok {
				for k, v := range defMap {
					if _, has := schema[k]; !has {
						schema[k] = deepCopyValue(v)
					}
				}
				flattenRefs(schema, defs)
			}
		}
		return
	}
	// 递归子结点
	for _, v := range schema {
		switch val := v.(type) {
		case map[string]any:
			flattenRefs(val, defs)
		case []any:
			for _, item := range val {
				if m, ok := item.(map[string]any); ok {
					flattenRefs(m, defs)
				}
			}
		}
	}
}

func cleanValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cleanForGemini(val)
	case []any:
		result := make([]any, 0, len(val))
		for _, item := range val {
			if m, ok := item.(map[string]any); ok {
				result = append(result, cleanForGemini(m))
			} else {
				result = append(result, item)
			}
		}
		return result
	default:
		return v
	}
}

// mergeOneOfAnyOf 合并 oneOf/anyOf 中所有选项：
//   - type: 使用第一个非空选项的类型
//   - properties: 合并所有选项的 properties（同名属性后面的覆盖）
//   - required: 合并所有选项的 required 字段
//   - description: 合并所有描述（用分号分隔）
//   - enum: 合并所有 enum 值（扁平化去重）
func mergeOneOfAnyOf(v any) map[string]any {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}

	result := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	var descriptions []string
	var allEnums []any
	var allRequired []string

	props := result["properties"].(map[string]any)

	for _, item := range arr {
		sub, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// type: 取第一个非空
		if result["type"] == "object" {
			if t, ok := sub["type"].(string); ok && t != "" && t != "object" {
				result["type"] = t
			}
		}
		// properties
		if p, ok := sub["properties"].(map[string]any); ok {
			for pk, pv := range p {
				props[pk] = pv
			}
		}
		// description
		if d, ok := sub["description"].(string); ok && d != "" {
			descriptions = append(descriptions, d)
		}
		// enum
		if e, ok := sub["enum"].([]any); ok {
			allEnums = append(allEnums, e...)
		}
		// required
		if r, ok := sub["required"].([]any); ok {
			for _, rv := range r {
				if rs, ok := rv.(string); ok {
					allRequired = append(allRequired, rs)
				}
			}
		}
	}

	if len(descriptions) > 0 {
		result["description"] = strings.Join(descriptions, "; ")
	}
	if len(allEnums) > 0 {
		result["enum"] = dedupeSlice(allEnums)
	}
	if len(allRequired) > 0 {
		req := make([]any, 0, len(allRequired))
		seen := make(map[string]bool)
		for _, r := range allRequired {
			if !seen[r] {
				seen[r] = true
				req = append(req, r)
			}
		}
		result["required"] = req
	}

	return result
}

// dedupeSlice 去重切片元素（保持顺序）
func dedupeSlice(s []any) []any {
	seen := make(map[any]bool)
	result := make([]any, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func extractFirstDescription(v any) string {
	arr, ok := v.([]any)
	if !ok {
		return ""
	}
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if desc, ok := m["description"].(string); ok && desc != "" {
				return desc
			}
		}
	}
	return ""
}

func deepCopyValue(v any) any {
	if m, ok := v.(map[string]any); ok {
		result := make(map[string]any, len(m))
		for k, val := range m {
			result[k] = deepCopyValue(val)
		}
		return result
	}
	if arr, ok := v.([]any); ok {
		result := make([]any, len(arr))
		for i, val := range arr {
			result[i] = deepCopyValue(val)
		}
		return result
	}
	return v
}
