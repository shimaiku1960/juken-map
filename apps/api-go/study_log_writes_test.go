package main

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// 期待値は、Node（Zod 4.5.4）に同じ本文を送って返ってきた 400（2026-09-30 に手元で確かめた）。
// 日付の形と暦の規則は、このとき Zod に足したもの（JUK-75）。

func TestReadStudyLogInput(t *testing.T) {
	const today = "2026-09-30"
	issue := func(message, code, field string) string {
		return `{"error":"` + message + `","code":"` + code + `","field":"` + field + `"}`
	}
	tests := []struct {
		name string
		body string
		want string // "" なら通る
	}{
		{"時間だけ", `{"date":"2026-09-01","minutes":30}`, ""},
		{"今日", `{"date":"2026-09-30","minutes":30}`, ""},
		{"全部入り", `{"date":"2026-09-01","minutes":30,"subject":"math","textbookId":3,"rangeStart":1,"rangeEnd":10,"rangeUnit":"page","memo":"  青チャート  "}`, ""},
		{"null の任意項目", `{"date":"2026-09-01","minutes":30,"subject":null,"textbookId":null,"rangeStart":null,"rangeEnd":null,"rangeUnit":null}`, ""},
		{"1440.0 は整数", `{"date":"2026-09-01","minutes":1440.0}`, ""},

		{"日付が無い", `{"minutes":30}`, issue("Invalid input: expected string, received undefined", "invalid_type", "date")},
		{"日付が数", `{"date":20260901,"minutes":30}`, issue("Invalid input: expected string, received number", "invalid_type", "date")},
		{"日付が空", `{"date":"","minutes":30}`, issue("日付を選択してください", "too_small", "date")},
		{"日付の形が違う（1）", `{"date":"1","minutes":30}`, issue("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"日付の形が違う（空白）", `{"date":" ","minutes":30}`, issue("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"日付に時刻", `{"date":"2026-09-01T10:00:00Z","minutes":30}`, issue("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"暦に無い", `{"date":"2026-02-30","minutes":30}`, issue("存在しない日付です", "invalid_date", "date")},
		{"13月", `{"date":"2025-13-01","minutes":30}`, issue("存在しない日付です", "invalid_date", "date")},
		{"未来", `{"date":"2026-10-01","minutes":30}`, issue("未来日は実績として記録できません", "future_date", "date")},
		{"日付が先に弾かれる", `{"date":"","minutes":"x","subject":"bad"}`, issue("日付を選択してください", "too_small", "date")},

		{"時間が無い", `{"date":"2026-09-01"}`, issue("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が文字", `{"date":"2026-09-01","minutes":"30"}`, issue("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が null", `{"date":"2026-09-01","minutes":null}`, issue("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が Infinity", `{"date":"2026-09-01","minutes":1e400}`, issue("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が小数", `{"date":"2026-09-01","minutes":1.5}`, issue("整数で入力してください", "invalid_type", "minutes")},
		{"時間が負の小数", `{"date":"2026-09-01","minutes":-1.5}`, issue("整数で入力してください", "invalid_type", "minutes")},
		{"時間が大きすぎる整数", `{"date":"2026-09-01","minutes":1e20}`, issue("整数で入力してください", "too_big", "minutes")},
		{"時間が小さすぎる整数", `{"date":"2026-09-01","minutes":-1e20}`, issue("整数で入力してください", "too_small", "minutes")},
		{"時間が0", `{"date":"2026-09-01","minutes":0}`, issue("1分以上を入力してください", "too_small", "minutes")},
		{"時間が負", `{"date":"2026-09-01","minutes":-5}`, issue("1分以上を入力してください", "too_small", "minutes")},
		{"時間が1441", `{"date":"2026-09-01","minutes":1441}`, issue("24時間（1440分）以内で入力してください", "too_big", "minutes")},

		{"科目が不正", `{"date":"2026-09-01","minutes":30,"subject":"xx"}`, issue("科目の値が不正です", "invalid_subject", "subject")},
		{"科目が空", `{"date":"2026-09-01","minutes":30,"subject":""}`, issue("科目の値が不正です", "invalid_subject", "subject")},
		{"科目が数", `{"date":"2026-09-01","minutes":30,"subject":1}`, issue("Invalid input: expected string, received number", "invalid_type", "subject")},

		{"参考書が小数", `{"date":"2026-09-01","minutes":30,"textbookId":1.5}`, issue("Invalid input: expected int, received number", "invalid_type", "textbookId")},
		{"参考書が0", `{"date":"2026-09-01","minutes":30,"textbookId":0}`, issue("Too small: expected number to be >0", "too_small", "textbookId")},
		{"参考書が -0", `{"date":"2026-09-01","minutes":30,"textbookId":-0}`, issue("Too small: expected number to be >0", "too_small", "textbookId")},
		{"参考書が安全な整数の外", `{"date":"2026-09-01","minutes":30,"textbookId":9007199254740993}`, issue("Too big: expected int to be <=9007199254740991", "too_big", "textbookId")},
		{"参考書が負で安全な整数の外", `{"date":"2026-09-01","minutes":30,"textbookId":-1e20}`, issue("Too small: expected int to be >=-9007199254740991", "too_small", "textbookId")},
		{"参考書が Infinity", `{"date":"2026-09-01","minutes":30,"textbookId":1e400}`, issue("Invalid input: expected number, received Infinity", "invalid_type", "textbookId")},
		{"参考書が -Infinity", `{"date":"2026-09-01","minutes":30,"textbookId":-1e400}`, issue("Invalid input: expected number, received -Infinity", "invalid_type", "textbookId")},
		{"参考書が文字", `{"date":"2026-09-01","minutes":30,"textbookId":"1"}`, issue("Invalid input: expected number, received string", "invalid_type", "textbookId")},
		{"参考書が真偽値", `{"date":"2026-09-01","minutes":30,"textbookId":true}`, issue("Invalid input: expected number, received boolean", "invalid_type", "textbookId")},
		{"参考書が配列", `{"date":"2026-09-01","minutes":30,"textbookId":[]}`, issue("Invalid input: expected number, received array", "invalid_type", "textbookId")},
		{"開始が0", `{"date":"2026-09-01","minutes":30,"rangeStart":0,"rangeEnd":0,"rangeUnit":"page"}`, issue("Too small: expected number to be >0", "too_small", "rangeStart")},

		{"単位が不正", `{"date":"2026-09-01","minutes":30,"rangeUnit":"zzz"}`, issue("単位の値が不正です", "invalid_range_unit", "rangeUnit")},
		{"単位が数", `{"date":"2026-09-01","minutes":30,"rangeUnit":1}`, issue("Invalid input: expected string, received number", "invalid_type", "rangeUnit")},

		{"メモが501文字", `{"date":"2026-09-01","minutes":30,"memo":"` + strings.Repeat("a", 501) + `"}`, issue("500文字以内で入力してください", "too_big", "memo")},
		{"メモが絵文字500個は通る", `{"date":"2026-09-01","minutes":30,"memo":"` + strings.Repeat("😀", 500) + `"}`, ""},
		{"メモが null は不可", `{"date":"2026-09-01","minutes":30,"memo":null}`, issue("Invalid input: expected string, received null", "invalid_type", "memo")},

		{"開始だけ", `{"date":"2026-09-01","minutes":30,"rangeStart":1}`, issue("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeEnd")},
		{"開始と null の終了", `{"date":"2026-09-01","minutes":30,"rangeStart":3,"rangeEnd":null}`, issue("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeEnd")},
		{"終了だけ", `{"date":"2026-09-01","minutes":30,"rangeEnd":3}`, issue("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeStart")},
		{"開始 > 終了", `{"date":"2026-09-01","minutes":30,"rangeStart":5,"rangeEnd":2,"rangeUnit":"page"}`, issue("終了は開始以上にしてください", "range_end_before_start", "rangeEnd")},
		{"開始 > 終了で単位なし", `{"date":"2026-09-01","minutes":30,"rangeStart":5,"rangeEnd":2}`, issue("終了は開始以上にしてください", "range_end_before_start", "rangeEnd")},
		{"単位なし", `{"date":"2026-09-01","minutes":30,"rangeStart":1,"rangeEnd":2}`, issue("単位を選択してください", "range_unit_required", "rangeUnit")},
		{"項目の誤りが範囲の規則より先", `{"date":"2026-09-01","minutes":30,"rangeStart":2,"rangeEnd":1,"rangeUnit":"x"}`, issue("単位の値が不正です", "invalid_range_unit", "rangeUnit")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := parseJSON(tt.body)
			if err != nil {
				t.Fatal(err)
			}
			in, _ := readStudyLogInput(body, today)
			res := httptest.NewRecorder()
			rejected := in.reject(res)
			if tt.want == "" {
				if rejected {
					t.Fatalf("弾かれた: %s", res.Body)
				}
				return
			}
			if !rejected {
				t.Fatal("通ってしまった")
			}
			assertJSONEqual(t, res.Body.String(), tt.want)
		})
	}
}

func TestReadStudyLogInputValues(t *testing.T) {
	body, _ := parseJSON(`{"date":"2026-09-01","minutes":30,"subject":null,"rangeStart":1,"rangeEnd":10,"rangeUnit":"page","memo":"  メモ  "}`)
	in, v := readStudyLogInput(body, "2026-09-30")
	if in.issue != nil {
		t.Fatal(in.issue)
	}
	if v.date != "2026-09-01" || v.minutes != 30 || *v.memo.value != "メモ" {
		t.Errorf("値 = %+v", v)
	}
	// subject は null、textbookId は無い。どちらも DB には null で書くが、PATCH の比べ方が違う
	if !v.subject.present || v.subject.value != nil || v.textbookID.present {
		t.Errorf("subject = %+v, textbookId = %+v", v.subject, v.textbookID)
	}
}

func TestOptionalDiffers(t *testing.T) {
	// Node の `data.x !== current`。current は DB の値で、null か値。
	one, two := int64(1), int64(2)
	missing := optional[int64]{}
	null := optional[int64]{present: true}
	value := optional[int64]{present: true, value: &one}
	tests := []struct {
		name    string
		o       optional[int64]
		current *int64
		want    bool
	}{
		{"無い（undefined）は null とも違う", missing, nil, true},
		{"無い（undefined）は値とも違う", missing, &one, true},
		{"null と null は同じ", null, nil, false},
		{"null と値は違う", null, &one, true},
		{"値と null は違う", value, nil, true},
		{"同じ値", value, &one, false},
		{"違う値", value, &two, true},
	}
	for _, tt := range tests {
		if got := tt.o.differs(tt.current); got != tt.want {
			t.Errorf("%s: differs = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestTextbookRangeError(t *testing.T) {
	page, question := "page", "question"
	total := int64(300)
	input := func(body string) studyLogInput {
		v, _ := parseJSON(body)
		_, in := readStudyLogInput(v, "2026-09-30")
		return in
	}
	tests := []struct {
		name string
		tb   ownedTextbook
		in   studyLogInput
		want string
	}{
		{"範囲が無ければ見ない", ownedTextbook{rangeUnit: &question, totalAmount: &total}, input(`{"date":"2026-09-01","minutes":1}`), ""},
		{"単位が違う", ownedTextbook{rangeUnit: &question}, input(`{"date":"2026-09-01","minutes":1,"rangeStart":1,"rangeEnd":2,"rangeUnit":"page"}`), "範囲の単位を参考書の逆算設定に合わせてください"},
		{"総量を超える", ownedTextbook{rangeUnit: &page, totalAmount: &total}, input(`{"date":"2026-09-01","minutes":1,"rangeStart":1,"rangeEnd":301,"rangeUnit":"page"}`), "終了位置は参考書の総量（300）以下にしてください"},
		{"総量ちょうど", ownedTextbook{rangeUnit: &page, totalAmount: &total}, input(`{"date":"2026-09-01","minutes":1,"rangeStart":1,"rangeEnd":300,"rangeUnit":"page"}`), ""},
		{"参考書に設定が無い", ownedTextbook{}, input(`{"date":"2026-09-01","minutes":1,"rangeStart":1,"rangeEnd":9999,"rangeUnit":"chapter"}`), ""},
	}
	for _, tt := range tests {
		if got := textbookRangeError(tt.tb, tt.in); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestIsCalendarYMD(t *testing.T) {
	for s, want := range map[string]bool{
		"2026-09-30": true, "2024-02-29": true, "2000-02-29": true, "0000-02-29": true,
		"2026-02-30": false, "2025-02-29": false, "1900-02-29": false, "2025-13-01": false,
		"2025-00-10": false, "2025-04-31": false, "2025-01-00": false,
	} {
		if got := isCalendarYMD(s); got != want {
			t.Errorf("isCalendarYMD(%s) = %v, want %v", s, got, want)
		}
	}
}
