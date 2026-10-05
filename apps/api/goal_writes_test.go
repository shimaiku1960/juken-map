package main

import (
	"strings"
	"testing"
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
		{"空", `{}`, issueJSON("Invalid input: expected number, received undefined", "invalid_type", "facultyId")},
		{"学部が文字列", `{"facultyId":"1"}`, issueJSON("Invalid input: expected number, received string", "invalid_type", "facultyId")},
		{"学部が0", `{"facultyId":0}`, issueJSON("志望学部を選択してください", "too_small", "facultyId")},
		{"学部が小数", `{"facultyId":1.5}`, issueJSON("Invalid input: expected int, received number", "invalid_type", "facultyId")},
		{"負の小数は整数の誤りが先", `{"facultyId":-1.5}`, issueJSON("Invalid input: expected int, received number", "invalid_type", "facultyId")},
		{"学部が大きすぎる", `{"facultyId":1e20}`, issueJSON("Too big: expected int to be <=9007199254740991", "too_big", "facultyId")},
		{"ステータスが不正", `{"facultyId":1,"status":"x"}`, issueJSON(goalStatusIssue, "invalid_value", "status")},
		{"ステータスが null", `{"facultyId":1,"status":null}`, issueJSON(goalStatusIssue, "invalid_value", "status")},
		{"本文が null", `null`, nullFieldIssueJSON("Invalid input: expected object, received null", "invalid_type")},
		{"本文が配列", `[]`, nullFieldIssueJSON("Invalid input: expected object, received array", "invalid_type")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := readObject(parse(t, tt.body))
			in.number("facultyId", facultyIDRule)
			in.optionalEnum("status", goalStatuses, false)
			checkIssue(t, in, tt.want)
		})
	}
}

func TestReadGoalUpdate(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空でも通る", `{}`, ""},
		{"学部が負", `{"facultyId":-1}`, issueJSON("志望学部を選択してください", "too_small", "facultyId")},
		{"ステータスが数", `{"status":1}`, issueJSON(goalStatusIssue, "invalid_value", "status")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := readObject(parse(t, tt.body))
			in.optionalInt("facultyId", facultyIDRule, false)
			in.optionalEnum("status", goalStatuses, false)
			checkIssue(t, in, tt.want)
		})
	}
}

func TestReadGoalPatch(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空でも通る", `{}`, ""},
		{"メモは null で消せる", `{"note":null}`, ""},
		{"メモ500文字", `{"note":"` + strings.Repeat("a", 500) + `"}`, ""},
		{"第一志望が文字列", `{"isFirstChoice":"y"}`, issueJSON("Invalid input: expected boolean, received string", "invalid_type", "isFirstChoice")},
		{"メモ501文字", `{"note":"` + strings.Repeat("a", 501) + `"}`, issueJSON("500文字以内で入力してください", "too_big", "note")},
		{"メモが数", `{"note":1}`, issueJSON("Invalid input: expected string, received number", "invalid_type", "note")},
		{"ステータスが不正", `{"status":"z"}`, issueJSON(goalStatusIssue, "invalid_value", "status")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := readObject(parse(t, tt.body))
			in.optionalBool("isFirstChoice")
			in.optionalString("note", goalNoteRule, true)
			in.optionalEnum("status", goalStatuses, false)
			checkIssue(t, in, tt.want)
		})
	}
}
