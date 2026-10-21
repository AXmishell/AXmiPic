package api

import (
	"encoding/json"
	"testing"
)

// TestPluginConfigValuesAcceptJSONScalars 验证插件配置接口接受布尔与数字标量，
// 避免前端开关提交 true/false 时被 map[string]string 解码拒绝。
func TestPluginConfigValuesAcceptJSONScalars(t *testing.T) {
	var body pluginConfigRequest
	if err := json.Unmarshal([]byte(`{"values":{"host":"smtp.example.com","port":465,"use_tls":true,"username":null}}`), &body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	values, err := pluginConfigValues(body.Values)
	if err != nil {
		t.Fatalf("pluginConfigValues: %v", err)
	}
	want := map[string]string{
		"host":     "smtp.example.com",
		"port":     "465",
		"use_tls":  "true",
		"username": "",
	}
	for key, expected := range want {
		if values[key] != expected {
			t.Fatalf("%s = %q, want %q", key, values[key], expected)
		}
	}
}

func TestPluginConfigValuesRejectNestedObjects(t *testing.T) {
	if _, err := pluginConfigValues(map[string]any{"options": map[string]any{"a": "b"}}); err == nil {
		t.Fatal("expected an error for a nested object value")
	}
}
