package universities

import (
	"encoding/json"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestExploreTypesMatchOpenAPI(t *testing.T) {
	// 大学の一覧は、Go へ移したときに JSON のキーの順番（と ETag）を以前と同じにするため、生成した型
	// （ExploreUniversity。キーはアルファベット順になる）を使わず手書きの型で返している。
	// 手書きの型が契約（openapi/openapi.yaml）とずれていないかを、同じ中身を JSON にして比べる。
	hand, err := json.Marshal(exploreUniversityDTO{
		ID: 1, Name: "大学", Prefecture: "東京都", Type: "国立",
		Faculties: []exploreFacultyDTO{{Tags: []exploreTagDTO{{Name: "文系"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := json.Marshal(apischema.ExploreUniversity{
		ID: 1, Name: "大学", Prefecture: "東京都", Type: "国立",
		Faculties: []apischema.ExploreFaculty{{Tags: []apischema.ExploreTag{{Name: "文系"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	httpxtest.AssertJSONEqual(t, string(hand), string(generated))
}
