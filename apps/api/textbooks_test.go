package main

import (
	"encoding/json"
	"testing"
)

func TestTextbookMasterJSON(t *testing.T) {
	// 手元のマスターはどれも総量の候補と出版社・版を持つので、その逆の場合は応答一致テストで確かめられない。
	// Node の形（候補が無くても "metrics": []、出版社・版が無ければ null）をここで確かめる。
	raw, err := json.Marshal(TextbookMaster{Metrics: []TextbookMasterMetric{}})
	if err != nil {
		t.Fatal(err)
	}
	got := decodeJSON(t, string(raw))
	if list, ok := got["metrics"].([]any); !ok || len(list) != 0 {
		t.Errorf("metrics = %v, want []", got["metrics"])
	}
	for _, key := range []string{"publisher", "edition"} {
		if v, has := got[key]; !has || v != nil {
			t.Errorf("%s = %v（キーあり %v）, want null", key, v, has)
		}
	}
}
