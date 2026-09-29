package main

import (
	"encoding/json"
	"testing"
)

func TestGoalJSONTags(t *testing.T) {
	// 合成データの学部はどれもタグを持つので、「タグの無い学部」は応答一致テストで確かめられない。
	// Node の形（一覧はタグが無くても "tags": []、第一志望はキーごと無い）をここで確かめる。
	withoutTags := goalDTO{Faculty: facultyDTO{Tags: []tagDTO{}}}
	firstChoice := goalDTO{}

	for _, tt := range []struct {
		name    string
		goal    goalDTO
		wantKey bool
	}{
		{"一覧：タグの無い学部は tags: []", withoutTags, true},
		{"第一志望：tags のキーを出さない", firstChoice, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.goal)
			if err != nil {
				t.Fatal(err)
			}
			faculty := decodeJSON(t, string(raw))["faculty"].(map[string]any)
			tags, has := faculty["tags"]
			if has != tt.wantKey {
				t.Fatalf("tags のキーの有無 = %v, want %v（%s）", has, tt.wantKey, raw)
			}
			if has {
				if list, ok := tags.([]any); !ok || len(list) != 0 {
					t.Errorf("tags = %v, want []", tags)
				}
			}
		})
	}
}
