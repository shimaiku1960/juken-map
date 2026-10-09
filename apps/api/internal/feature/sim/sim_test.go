package sim

import (
	"encoding/json"
	"strings"
	"testing"
)

// jsonValue は httpx.ReadBody と同じく、数を json.Number にして JSON を読む。
func jsonValue(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestParseSimMark(t *testing.T) {
	// Node の markBodySchema（Zod）と isSimEmail と同じ判定。期待値は Node で確かめた（2026-09-30）
	for _, tt := range []struct {
		name string
		body string
		ok   bool
	}{
		{"正しい", `{"email":"delivered+sim00001@resend.dev","seq":1,"cohort":"steady"}`, true},
		{"余計なキーは無視", `{"email":"delivered+sim1@resend.dev","seq":1,"cohort":"fading","x":1}`, true},
		{"大文字のアドレス", `{"email":"Delivered+SIM1@Resend.dev","seq":1,"cohort":"steady"}`, true},
		{"1.0 は整数", `{"email":"delivered+sim1@resend.dev","seq":1.0,"cohort":"steady"}`, true},
		{"実ユーザーのアドレス", `{"email":"a@b.com","seq":1,"cohort":"steady"}`, false},
		{"seed の合成ユーザー", `{"email":"u1@synthetic.juken-map.invalid","seq":1,"cohort":"steady"}`, false},
		{"連番が小数", `{"email":"delivered+sim1@resend.dev","seq":1.5,"cohort":"steady"}`, false},
		{"連番が0", `{"email":"delivered+sim1@resend.dev","seq":0,"cohort":"steady"}`, false},
		{"連番が安全な整数の外", `{"email":"delivered+sim1@resend.dev","seq":1e20,"cohort":"steady"}`, false},
		{"連番が文字列", `{"email":"delivered+sim1@resend.dev","seq":"1","cohort":"steady"}`, false},
		{"知らない型", `{"email":"delivered+sim1@resend.dev","seq":1,"cohort":"x"}`, false},
		{"配列", `[]`, false},
		{"null", `null`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := parseSimMark(jsonValue(t, tt.body)); ok != tt.ok {
				t.Errorf("ok = %v, want %v", ok, tt.ok)
			}
		})
	}
	if _, ok := parseSimMark(nil); ok {
		t.Error("本文なし（壊れた JSON）は 400")
	}
}

func TestParseSimUpdate(t *testing.T) {
	date := "2026-09-30"
	for _, tt := range []struct {
		name string
		body string
		ok   bool
		want simUpdate
	}{
		{"空は何も変えない", `{}`, true, simUpdate{}},
		{"日付を入れる", `{"lastActedOn":"2026-09-30"}`, true, simUpdate{lastActedOn: &date, setLastActedOn: true}},
		{"null は消す", `{"dormantFrom":null}`, true, simUpdate{setDormantFrom: true}},
		{"形が違う", `{"lastActedOn":"2026/09/30"}`, false, simUpdate{}},
		{"文字列でない", `{"dormantFrom":20260930}`, false, simUpdate{}},
		{"配列", `[]`, false, simUpdate{}},
		{"null", `null`, false, simUpdate{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSimUpdate(jsonValue(t, tt.body))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if got.setLastActedOn != tt.want.setLastActedOn || got.setDormantFrom != tt.want.setDormantFrom ||
				!sameString(got.lastActedOn, tt.want.lastActedOn) || !sameString(got.dormantFrom, tt.want.dormantFrom) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func sameString(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func TestParseSeq(t *testing.T) {
	// 0 より大きい整数だけを通す
	for _, tt := range []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1", 1, true},
		{"1e3", 1000, true},
		{"1.0", 1, true},
		{"99999999999999999999", -1, true}, // どの行にも当たらない値にする
		{"0", 0, false},
		{"-1", 0, false},
		{"1.5", 0, false},
		{"abc", 0, false},
		{"", 0, false},
		{"Infinity", 0, false},
		{"NaN", 0, false},
	} {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := parseSeq(tt.in)
			if ok != tt.ok || (ok && got != tt.want) {
				t.Errorf("parseSeq(%q) = %d, %v, want %d, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
