package main

import (
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// 期待値は、Node の Zod（goalSchema・updateGoalSchema・patchGoalSchema、zod 4.5.4）に
// 同じ入力を通した結果（2026-10-01 に手元で確かめた）。

const goalStatusIssue = `Invalid option: expected one of \"candidate\"|\"decided\"`

func nullFieldIssueJSON(message, code string) string {
	return `{"error":"` + message + `","code":"` + code + `","field":null}`
}

func TestReadGoalInput(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"通る", `{"facultyId":1}`, ""},
		{"ステータスつき", `{"facultyId":1,"status":"candidate"}`, ""},
		{"空", `{}`, httpxtest.IssueJSON("Invalid input: expected number, received undefined", "invalid_type", "facultyId")},
		{"学部が文字列", `{"facultyId":"1"}`, httpxtest.IssueJSON("Invalid input: expected number, received string", "invalid_type", "facultyId")},
		{"学部が0", `{"facultyId":0}`, httpxtest.IssueJSON("志望学部を選択してください", "too_small", "facultyId")},
		{"学部が小数", `{"facultyId":1.5}`, httpxtest.IssueJSON("Invalid input: expected int, received number", "invalid_type", "facultyId")},
		{"負の小数は整数の誤りが先", `{"facultyId":-1.5}`, httpxtest.IssueJSON("Invalid input: expected int, received number", "invalid_type", "facultyId")},
		{"学部が大きすぎる", `{"facultyId":1e20}`, httpxtest.IssueJSON("Too big: expected int to be <=9007199254740991", "too_big", "facultyId")},
		{"ステータスが不正", `{"facultyId":1,"status":"x"}`, httpxtest.IssueJSON(goalStatusIssue, "invalid_value", "status")},
		{"ステータスが null", `{"facultyId":1,"status":null}`, httpxtest.IssueJSON(goalStatusIssue, "invalid_value", "status")},
		{"本文が null", `null`, nullFieldIssueJSON("Invalid input: expected object, received null", "invalid_type")},
		{"本文が配列", `[]`, nullFieldIssueJSON("Invalid input: expected object, received array", "invalid_type")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := httpx.ReadObject(httpxtest.ParseBody(t, tt.body))
			in.Number("facultyId", facultyIDRule)
			in.OptionalEnum("status", goalStatuses, false)
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}
}

func TestReadGoalUpdate(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空でも通る", `{}`, ""},
		{"学部が負", `{"facultyId":-1}`, httpxtest.IssueJSON("志望学部を選択してください", "too_small", "facultyId")},
		{"ステータスが数", `{"status":1}`, httpxtest.IssueJSON(goalStatusIssue, "invalid_value", "status")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := httpx.ReadObject(httpxtest.ParseBody(t, tt.body))
			in.OptionalInt("facultyId", facultyIDRule, false)
			in.OptionalEnum("status", goalStatuses, false)
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}
}

func TestReadGoalPatch(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空でも通る", `{}`, ""},
		{"メモは null で消せる", `{"note":null}`, ""},
		{"メモ500文字", `{"note":"` + strings.Repeat("a", 500) + `"}`, ""},
		{"第一志望が文字列", `{"isFirstChoice":"y"}`, httpxtest.IssueJSON("Invalid input: expected boolean, received string", "invalid_type", "isFirstChoice")},
		{"メモ501文字", `{"note":"` + strings.Repeat("a", 501) + `"}`, httpxtest.IssueJSON("500文字以内で入力してください", "too_big", "note")},
		{"メモが数", `{"note":1}`, httpxtest.IssueJSON("Invalid input: expected string, received number", "invalid_type", "note")},
		{"ステータスが不正", `{"status":"z"}`, httpxtest.IssueJSON(goalStatusIssue, "invalid_value", "status")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := httpx.ReadObject(httpxtest.ParseBody(t, tt.body))
			in.OptionalBool("isFirstChoice")
			in.OptionalString("note", goalNoteRule, true)
			in.OptionalEnum("status", goalStatuses, false)
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}
}
