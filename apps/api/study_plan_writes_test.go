package main

import (
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// 期待値は、Node の Zod（createStudyPlansSchema・updateStudyPlanSchema・completeStudyPlanSchema、zod 4.5.4）に
// 同じ入力を通した結果（2026-09-30 に手元で確かめた）。

func TestReadStudyPlansInput(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"通る（未来の日付も可、メモは削る）", `{"date":"2099-10-01","items":[{"content":" a "},{"textbookId":1,"subject":null}]}`, ""},
		{"空", `{}`, httpxtest.IssueJSON("Invalid input: expected string, received undefined", "invalid_type", "date")},
		{"items が無い", `{"date":"2026-10-01"}`, httpxtest.IssueJSON("Invalid input: expected array, received undefined", "invalid_type", "items")},
		{"items がオブジェクト", `{"date":"2026-10-01","items":{}}`, httpxtest.IssueJSON("Invalid input: expected array, received object", "invalid_type", "items")},
		{"items が空", `{"date":"2026-10-01","items":[]}`, httpxtest.IssueJSON("内容を1つ以上入力してください", "too_small", "items")},
		{"要素が数", `{"date":"2026-10-01","items":[1]}`, httpxtest.IssueJSON("Invalid input: expected object, received number", "invalid_type", "items.0")},
		{"要素が null", `{"date":"2026-10-01","items":[null]}`, httpxtest.IssueJSON("Invalid input: expected object, received null", "invalid_type", "items.0")},
		{"要素が空", `{"date":"2026-10-01","items":[{}]}`, httpxtest.IssueJSON("参考書・範囲・メモのいずれかを入力してください", "plan_content_required", "items.0.content")},
		{"メモが空白だけ（削ると空）", `{"date":"2026-10-01","items":[{"content":"   "}]}`, httpxtest.IssueJSON("参考書・範囲・メモのいずれかを入力してください", "plan_content_required", "items.0.content")},
		{"メモが null", `{"date":"2026-10-01","items":[{"content":null}]}`, httpxtest.IssueJSON("Invalid input: expected string, received null", "invalid_type", "items.0.content")},
		{"要素0の組み合わせの規則が要素1の型の誤りより先", `{"date":"2026-10-01","items":[{},{"textbookId":0}]}`, httpxtest.IssueJSON("参考書・範囲・メモのいずれかを入力してください", "plan_content_required", "items.0.content")},
		{"日付が要素より先", `{"date":"x","items":[{"textbookId":0}]}`, httpxtest.IssueJSON("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"範囲の開始だけ", `{"date":"2026-10-01","items":[{"rangeStart":1}]}`, httpxtest.IssueJSON("範囲は開始と終了の両方を入力してください", "range_incomplete", "items.0.rangeEnd")},
		{"単位が不正（中身ゼロより先）", `{"date":"2026-10-01","items":[{"rangeUnit":"x"}]}`, httpxtest.IssueJSON("単位の値が不正です", "invalid_range_unit", "items.0.rangeUnit")},
		{"2つ目の要素の誤り", `{"date":"2026-10-01","items":[{"content":"a"},{"textbookId":1.5}]}`, httpxtest.IssueJSON("Invalid input: expected int, received number", "invalid_type", "items.1.textbookId")},
		{"暦に無い日付", `{"date":"2026-02-30","items":[{"content":"a"}]}`, httpxtest.IssueJSON("存在しない日付です", "invalid_date", "date")},
		{"日付が空", `{"date":"","items":[{"content":"a"}]}`, httpxtest.IssueJSON("日付を選択してください", "too_small", "date")},
		{"メモが501文字", `{"date":"2026-10-01","items":[{"content":"` + strings.Repeat("a", 501) + `"}]}`, httpxtest.IssueJSON("500文字以内で入力してください", "too_big", "items.0.content")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, _, _ := readStudyPlansInput(httpxtest.ParseBody(t, tt.body))
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}

	in, date, items := readStudyPlansInput(httpxtest.ParseBody(t, `{"date":"2099-10-01","items":[{"content":" a "},{"textbookId":1,"subject":null}]}`))
	if in.Issue != nil || date != "2099-10-01" || len(items) != 2 || *items[0].content.Value != "a" ||
		*items[1].textbookID.Value != 1 || !items[1].subject.Present || items[1].subject.Value != nil {
		t.Errorf("値 = %q %+v（issue %v）", date, items, in.Issue)
	}
}

func TestReadStudyPlanUpdate(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空は通る（何も書き換えない）", `{}`, ""},
		{"通る", `{"date":"2026-10-01","done":true,"textbookId":null,"content":"  x "}`, ""},
		{"日付が空（文言は Zod の既定）", `{"date":""}`, httpxtest.IssueJSON("Too small: expected string to have >=1 characters", "too_small", "date")},
		{"日付の形", `{"date":"1"}`, httpxtest.IssueJSON("日付は YYYY-MM-DD で指定してください", "invalid_format", "date")},
		{"日付が null", `{"date":null}`, httpxtest.IssueJSON("Invalid input: expected string, received null", "invalid_type", "date")},
		{"done が文字", `{"done":"yes"}`, httpxtest.IssueJSON("Invalid input: expected boolean, received string", "invalid_type", "done")},
		{"メモが null", `{"content":null}`, httpxtest.IssueJSON("Invalid input: expected string, received null", "invalid_type", "content")},
		{"範囲の終了だけ", `{"rangeEnd":3}`, httpxtest.IssueJSON("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeStart")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, _ := readStudyPlanUpdate(httpxtest.ParseBody(t, tt.body))
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}

	in, v := readStudyPlanUpdate(httpxtest.ParseBody(t, `{"date":"2026-10-01","done":true,"textbookId":null,"content":"  x "}`))
	if in.Issue != nil || *v.date.Value != "2026-10-01" || !*v.done.Value || !v.textbookID.Present || v.textbookID.Value != nil ||
		*v.content.Value != "x" || v.rangeStart.Present || v.subject.Present {
		t.Errorf("値 = %+v", v)
	}
}

func TestReadCompleteInput(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空", `{}`, httpxtest.IssueJSON("学習時間を入力してください", "invalid_type", "minutes")},
		{"通る", `{"minutes":30,"rangeStart":null}`, ""},
		{"範囲の開始だけ", `{"minutes":30,"rangeStart":1}`, httpxtest.IssueJSON("範囲は開始と終了の両方を入力してください", "range_incomplete", "rangeEnd")},
		{"メモが null", `{"minutes":30,"memo":null}`, httpxtest.IssueJSON("Invalid input: expected string, received null", "invalid_type", "memo")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, _ := readCompleteInput(httpxtest.ParseBody(t, tt.body))
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}
}
