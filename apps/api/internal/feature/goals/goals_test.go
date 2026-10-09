package goals

import (
	"encoding/json"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestGoalJSONTags(t *testing.T) {
	// 合成データの学部はどれもタグを持つので、「タグの無い学部」は応答一致テストで確かめられない。
	// 画面が前提にしている形（一覧はタグが無くても "tags": []、第一志望はキーごと無い）をここで確かめる。
	for _, tt := range []struct {
		name    string
		goal    any
		wantKey bool
	}{
		{"一覧：タグの無い学部は tags: []", withTags(apischema.FirstChoiceGoal{}), true},
		{"第一志望：tags のキーを出さない", apischema.FirstChoiceGoal{}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.goal)
			if err != nil {
				t.Fatal(err)
			}
			faculty := httpxtest.DecodeJSON(t, string(raw))["faculty"].(map[string]any)
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
