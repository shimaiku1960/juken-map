package studyrecord

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 期待値は、Node（Zod 4.5.4）に同じ本文を送って返ってきた 400（2026-09-30 に手元で確かめた）。
// 日付の形と暦の規則は、このとき Zod に足したもの（JUK-75）。予定は createStudyPlansSchema・
// updateStudyPlanSchema・completeStudyPlanSchema に同じ入力を通した結果。

// parse は本文を入口と同じく読む（数は json.Number）。model は httpx を知らないので、ここで同じことをする。
func parse(t *testing.T, body string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// bad は期待する誤り（入口はこれをそのまま 400 の error・code・field にする）。
func bad(message, code, field string) string { return code + " " + field + " " + message }

// checkIssue は want が空なら通ること、そうでなければ最初の誤りが want であることを見る。
func checkIssue(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("弾かれた: %v", err)
		}
		return
	}
	var v *validation.Issue
	if !errors.As(err, &v) {
		t.Fatalf("誤りが *validation.Issue でない: %v", err)
	}
	if got := bad(v.Message, v.Code, v.Field); got != want {
		t.Errorf("誤り = %q, want %q", got, want)
	}
}

func TestDecodeLogInput(t *testing.T) {
	const today = "2026-09-30"
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

		{"日付が無い", `{"minutes":30}`, bad("Invalid input: expected string, received undefined", "invalid_type", "date")},
		{"日付が数", `{"date":20260901,"minutes":30}`, bad("Invalid input: expected string, received number", "invalid_type", "date")},
		{"日付が空", `{"date":"","minutes":30}`, bad("日付を選択してください", "too_small", "date")},
		{"日付の形が違う（1）", `{"date":"1","minutes":30}`, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"日付の形が違う（空白）", `{"date":" ","minutes":30}`, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"日付に時刻", `{"date":"2026-09-01T10:00:00Z","minutes":30}`, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"暦に無い", `{"date":"2026-02-30","minutes":30}`, bad("存在しない日付です", "invalid_date", "date")},
		{"13月", `{"date":"2025-13-01","minutes":30}`, bad("存在しない日付です", "invalid_date", "date")},
		{"未来", `{"date":"2026-10-01","minutes":30}`, bad("未来日は実績として記録できません", "future_date", "date")},
		{"日付が先に弾かれる", `{"date":"","minutes":"x","subject":"bad"}`, bad("日付を選択してください", "too_small", "date")},

		{"時間が無い", `{"date":"2026-09-01"}`, bad("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が文字", `{"date":"2026-09-01","minutes":"30"}`, bad("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が null", `{"date":"2026-09-01","minutes":null}`, bad("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が Infinity", `{"date":"2026-09-01","minutes":1e400}`, bad("学習時間を入力してください", "invalid_type", "minutes")},
		{"時間が小数", `{"date":"2026-09-01","minutes":1.5}`, bad("整数で入力してください", "invalid_type", "minutes")},
		{"時間が負の小数", `{"date":"2026-09-01","minutes":-1.5}`, bad("整数で入力してください", "invalid_type", "minutes")},
		{"時間が大きすぎる整数", `{"date":"2026-09-01","minutes":1e20}`, bad("整数で入力してください", "too_big", "minutes")},
		{"時間が小さすぎる整数", `{"date":"2026-09-01","minutes":-1e20}`, bad("整数で入力してください", "too_small", "minutes")},
		{"時間が0", `{"date":"2026-09-01","minutes":0}`, bad("1分以上を入力してください", "too_small", "minutes")},
		{"時間が負", `{"date":"2026-09-01","minutes":-5}`, bad("1分以上を入力してください", "too_small", "minutes")},
		{"時間が1441", `{"date":"2026-09-01","minutes":1441}`, bad("24時間（1440分）以内で入力してください", "too_big", "minutes")},

		{"科目が不正", `{"date":"2026-09-01","minutes":30,"subject":"xx"}`, bad("科目の値が不正です", "invalid_subject", "subject")},
		{"科目が空", `{"date":"2026-09-01","minutes":30,"subject":""}`, bad("科目の値が不正です", "invalid_subject", "subject")},
		{"科目が数", `{"date":"2026-09-01","minutes":30,"subject":1}`, bad("Invalid input: expected string, received number", "invalid_type", "subject")},

		{"参考書が小数", `{"date":"2026-09-01","minutes":30,"textbookId":1.5}`, bad("Invalid input: expected int, received number", "invalid_type", "textbookId")},
		{"参考書が0", `{"date":"2026-09-01","minutes":30,"textbookId":0}`, bad("Too small: expected number to be >0", "too_small", "textbookId")},
		{"参考書が -0", `{"date":"2026-09-01","minutes":30,"textbookId":-0}`, bad("Too small: expected number to be >0", "too_small", "textbookId")},
		{"参考書が安全な整数の外", `{"date":"2026-09-01","minutes":30,"textbookId":9007199254740993}`, bad("Too big: expected int to be <=9007199254740991", "too_big", "textbookId")},
		{"参考書が負で安全な整数の外", `{"date":"2026-09-01","minutes":30,"textbookId":-1e20}`, bad("Too small: expected int to be >=-9007199254740991", "too_small", "textbookId")},
		{"参考書が Infinity", `{"date":"2026-09-01","minutes":30,"textbookId":1e400}`, bad("Invalid input: expected number, received Infinity", "invalid_type", "textbookId")},
		{"参考書が -Infinity", `{"date":"2026-09-01","minutes":30,"textbookId":-1e400}`, bad("Invalid input: expected number, received -Infinity", "invalid_type", "textbookId")},
		{"参考書が文字", `{"date":"2026-09-01","minutes":30,"textbookId":"1"}`, bad("Invalid input: expected number, received string", "invalid_type", "textbookId")},
		{"参考書が真偽値", `{"date":"2026-09-01","minutes":30,"textbookId":true}`, bad("Invalid input: expected number, received boolean", "invalid_type", "textbookId")},
		{"参考書が配列", `{"date":"2026-09-01","minutes":30,"textbookId":[]}`, bad("Invalid input: expected number, received array", "invalid_type", "textbookId")},
		{"開始が0", `{"date":"2026-09-01","minutes":30,"rangeStart":0,"rangeEnd":0,"rangeUnit":"page"}`, bad("Too small: expected number to be >0", "too_small", "rangeStart")},

		{"単位が不正", `{"date":"2026-09-01","minutes":30,"rangeUnit":"zzz"}`, bad("単位の値が不正です", "invalid_range_unit", "rangeUnit")},
		{"単位が数", `{"date":"2026-09-01","minutes":30,"rangeUnit":1}`, bad("Invalid input: expected string, received number", "invalid_type", "rangeUnit")},

		{"メモが501文字", `{"date":"2026-09-01","minutes":30,"memo":"` + strings.Repeat("a", 501) + `"}`, bad("500文字以内で入力してください", "too_big", "memo")},
		{"メモが絵文字500個は通る", `{"date":"2026-09-01","minutes":30,"memo":"` + strings.Repeat("😀", 500) + `"}`, ""},
		{"メモが null は不可", `{"date":"2026-09-01","minutes":30,"memo":null}`, bad("Invalid input: expected string, received null", "invalid_type", "memo")},

		{"開始だけ", `{"date":"2026-09-01","minutes":30,"rangeStart":1}`, bad("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeEnd")},
		{"開始と null の終了", `{"date":"2026-09-01","minutes":30,"rangeStart":3,"rangeEnd":null}`, bad("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeEnd")},
		{"終了だけ", `{"date":"2026-09-01","minutes":30,"rangeEnd":3}`, bad("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeStart")},
		{"開始 > 終了", `{"date":"2026-09-01","minutes":30,"rangeStart":5,"rangeEnd":2,"rangeUnit":"page"}`, bad("終了は開始以上にしてください", "range_end_before_start", "rangeEnd")},
		{"開始 > 終了で単位なし", `{"date":"2026-09-01","minutes":30,"rangeStart":5,"rangeEnd":2}`, bad("終了は開始以上にしてください", "range_end_before_start", "rangeEnd")},
		{"単位なし", `{"date":"2026-09-01","minutes":30,"rangeStart":1,"rangeEnd":2}`, bad("単位を選択してください", "range_unit_required", "rangeUnit")},
		{"項目の誤りが範囲の規則より先", `{"date":"2026-09-01","minutes":30,"rangeStart":2,"rangeEnd":1,"rangeUnit":"x"}`, bad("単位の値が不正です", "invalid_range_unit", "rangeUnit")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeLogInput(parse(t, tt.body), today)
			checkIssue(t, err, tt.want)
		})
	}
}

func TestDecodeLogInputValues(t *testing.T) {
	v, err := decodeLogInput(parse(t, `{"date":"2026-09-01","minutes":30,"subject":null,"rangeStart":1,"rangeEnd":10,"rangeUnit":"page","memo":"  メモ  "}`), "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Date.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || v.Minutes != 30 || *v.Memo.Value != "メモ" {
		t.Errorf("値 = %+v", v)
	}
	// subject は null、textbookId は無い。どちらも DB には null で書くが、変更での比べ方が違う
	if !v.Subject.Present || v.Subject.Value != nil || v.TextbookID.Present {
		t.Errorf("subject = %+v, textbookId = %+v", v.Subject, v.TextbookID)
	}
}

func TestDecodePlans(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"通る（未来の日付も可、メモは削る）", `{"date":"2099-10-01","items":[{"content":" a "},{"textbookId":1,"subject":null}]}`, ""},
		{"空", `{}`, bad("Invalid input: expected string, received undefined", "invalid_type", "date")},
		{"items が無い", `{"date":"2026-10-01"}`, bad("Invalid input: expected array, received undefined", "invalid_type", "items")},
		{"items がオブジェクト", `{"date":"2026-10-01","items":{}}`, bad("Invalid input: expected array, received object", "invalid_type", "items")},
		{"items が空", `{"date":"2026-10-01","items":[]}`, bad("内容を1つ以上入力してください", "too_small", "items")},
		{"要素が数", `{"date":"2026-10-01","items":[1]}`, bad("Invalid input: expected object, received number", "invalid_type", "items.0")},
		{"要素が null", `{"date":"2026-10-01","items":[null]}`, bad("Invalid input: expected object, received null", "invalid_type", "items.0")},
		{"要素が空", `{"date":"2026-10-01","items":[{}]}`, bad("参考書・範囲・メモのいずれかを入力してください", "plan_content_required", "items.0.content")},
		{"メモが空白だけ（削ると空）", `{"date":"2026-10-01","items":[{"content":"   "}]}`, bad("参考書・範囲・メモのいずれかを入力してください", "plan_content_required", "items.0.content")},
		{"メモが null", `{"date":"2026-10-01","items":[{"content":null}]}`, bad("Invalid input: expected string, received null", "invalid_type", "items.0.content")},
		{"要素0の組み合わせの規則が要素1の型の誤りより先", `{"date":"2026-10-01","items":[{},{"textbookId":0}]}`, bad("参考書・範囲・メモのいずれかを入力してください", "plan_content_required", "items.0.content")},
		{"日付が要素より先", `{"date":"x","items":[{"textbookId":0}]}`, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"範囲の開始だけ", `{"date":"2026-10-01","items":[{"rangeStart":1}]}`, bad("範囲は開始と終了の両方を入力してください", "range_incomplete", "items.0.rangeEnd")},
		{"単位が不正（中身ゼロより先）", `{"date":"2026-10-01","items":[{"rangeUnit":"x"}]}`, bad("単位の値が不正です", "invalid_range_unit", "items.0.rangeUnit")},
		{"2つ目の要素の誤り", `{"date":"2026-10-01","items":[{"content":"a"},{"textbookId":1.5}]}`, bad("Invalid input: expected int, received number", "invalid_type", "items.1.textbookId")},
		{"暦に無い日付", `{"date":"2026-02-30","items":[{"content":"a"}]}`, bad("存在しない日付です", "invalid_date", "date")},
		{"日付が空", `{"date":"","items":[{"content":"a"}]}`, bad("日付を選択してください", "too_small", "date")},
		{"メモが501文字", `{"date":"2026-10-01","items":[{"content":"` + strings.Repeat("a", 501) + `"}]}`, bad("500文字以内で入力してください", "too_big", "items.0.content")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := decodePlans(parse(t, tt.body))
			checkIssue(t, err, tt.want)
		})
	}

	date, items, err := decodePlans(parse(t, `{"date":"2099-10-01","items":[{"content":" a "},{"textbookId":1,"subject":null}]}`))
	if err != nil || !date.Equal(time.Date(2099, 10, 1, 0, 0, 0, 0, time.UTC)) || len(items) != 2 || *items[0].Content != "a" ||
		*items[1].TextbookID != 1 || items[1].Subject != nil {
		t.Errorf("値 = %v %+v（err %v）", date, items, err)
	}
}

func TestDecodePlanPatch(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空は通る（何も書き換えない）", `{}`, ""},
		{"通る", `{"date":"2026-10-01","done":true,"textbookId":null,"content":"  x "}`, ""},
		{"日付が空（文言は Zod の既定）", `{"date":""}`, bad("Too small: expected string to have >=1 characters", "too_small", "date")},
		{"日付の形", `{"date":"1"}`, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"日付が null", `{"date":null}`, bad("Invalid input: expected string, received null", "invalid_type", "date")},
		{"done が文字", `{"done":"yes"}`, bad("Invalid input: expected boolean, received string", "invalid_type", "done")},
		{"メモが null", `{"content":null}`, bad("Invalid input: expected string, received null", "invalid_type", "content")},
		{"範囲の終了だけ", `{"rangeEnd":3}`, bad("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeStart")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodePlanPatch(parse(t, tt.body))
			checkIssue(t, err, tt.want)
		})
	}

	v, err := decodePlanPatch(parse(t, `{"date":"2026-10-01","done":true,"textbookId":null,"content":"  x "}`))
	if err != nil || !v.Date.Value.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || !*v.Done.Value ||
		!v.TextbookID.Present || v.TextbookID.Value != nil || *v.Content.Value != "x" || v.RangeStart.Present || v.Subject.Present {
		t.Errorf("値 = %+v（err %v）", v, err)
	}
}

func TestDecodeCompletion(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空", `{}`, bad("学習時間を入力してください", "invalid_type", "minutes")},
		{"通る", `{"minutes":30,"rangeStart":null}`, ""},
		{"範囲の開始だけ", `{"minutes":30,"rangeStart":1}`, bad("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeEnd")},
		{"メモが null", `{"minutes":30,"memo":null}`, bad("Invalid input: expected string, received null", "invalid_type", "memo")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeCompletion(parse(t, tt.body))
			checkIssue(t, err, tt.want)
		})
	}
}
