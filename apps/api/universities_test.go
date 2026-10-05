package main

import (
	"encoding/json"
	"testing"
)

func TestExploreTypesMatchOpenAPI(t *testing.T) {
	// 大学の一覧は、JSON のキーの順番を Node と同じにして ETag をそろえるため、生成した型
	// （ExploreUniversity。キーはアルファベット順になる）を使わず手書きの型で返している。
	// 手書きの型が契約（openapi/openapi.yaml）とずれていないかを、同じ中身を JSON にして比べる。
	hand, err := json.Marshal(exploreUniversityDTO{
		ID: 1, Name: "大学", Prefecture: "東京都", Type: "国立",
		Faculties: []exploreFacultyDTO{{Tags: []exploreTagDTO{{Name: "文系"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := json.Marshal(ExploreUniversity{
		ID: 1, Name: "大学", Prefecture: "東京都", Type: "国立",
		Faculties: []ExploreFaculty{{Tags: []ExploreTag{{Name: "文系"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, string(hand), string(generated))
}
