package textbooks

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// 期待値は、Node の Zod（createTextbookSchema・updateTextbookProgressSchema、zod 4.5.4）に
// 同じ入力を通した結果（2026-10-01 に手元で確かめた）。

const (
	invalidUnionJSON = `{"error":"Invalid input","code":"invalid_union","field":null}`
	subjectIssue     = `Invalid option: expected one of \"english\"|\"math\"|\"japanese\"|\"science\"|\"social\"|\"other\"`
	rangeUnitIssue   = `Invalid option: expected one of \"page\"|\"question\"|\"chapter\"|\"number\"|\"part\"|\"section\"`
)

func TestReadTextbookInput(t *testing.T) {
	tests := []struct {
		name, body string
		// want は弾いたときの本文。通るなら ""
		want string
		// fromMaster・value は通ったときの中身（value は名前か masterId）
		fromMaster bool
		value      string
	}{
		{"名前で作る（前後の空白は削る）", `{"name":" a ","subject":null}`, "", false, "a"},
		{"マスターから作る", `{"masterId":3}`, "", true, "3"},
		{"両方送ると名前で作る", `{"name":"a","masterId":"x"}`, "", false, "a"},
		{"名前が誤りならマスターで作る", `{"name":"","masterId":1}`, "", true, "1"},

		// どちらにも当たらない：型の誤りを含まない選択肢がちょうど1つなら、その issue
		{"名前が空", `{"name":""}`, httpxtest.IssueJSON("参考書名を入力してください", "too_small", "name"), false, ""},
		{"名前が空白だけ", `{"name":"  "}`, httpxtest.IssueJSON("参考書名を入力してください", "too_small", "name"), false, ""},
		{"名前が全角空白だけ", `{"name":"　"}`, httpxtest.IssueJSON("参考書名を入力してください", "too_small", "name"), false, ""},
		{"名前が101文字", `{"name":"` + strings.Repeat("a", 101) + `"}`, httpxtest.IssueJSON("100文字以内で入力してください", "too_big", "name"), false, ""},
		{"名前が空・科目は null・単位は正しい", `{"name":"","subject":null,"rangeUnit":"page"}`, httpxtest.IssueJSON("参考書名を入力してください", "too_small", "name"), false, ""},
		{"名前が空・マスターが小数", `{"name":"","masterId":1.5}`, httpxtest.IssueJSON("参考書名を入力してください", "too_small", "name"), false, ""},
		{"マスターが0", `{"masterId":0}`, httpxtest.IssueJSON("Too small: expected number to be >0", "too_small", "masterId"), false, ""},
		{"マスターが0・名前が数", `{"masterId":0,"name":1}`, httpxtest.IssueJSON("Too small: expected number to be >0", "too_small", "masterId"), false, ""},
		{"マスターが0・科目が不正", `{"masterId":0,"subject":"x"}`, httpxtest.IssueJSON("Too small: expected number to be >0", "too_small", "masterId"), false, ""},
		{"マスターが大きすぎる", `{"masterId":1e20}`, httpxtest.IssueJSON("Too big: expected int to be <=9007199254740991", "too_big", "masterId"), false, ""},
		{"マスターが 2^53", `{"masterId":9007199254740992}`, httpxtest.IssueJSON("Too big: expected int to be <=9007199254740991", "too_big", "masterId"), false, ""},

		// どちらにも当たらない：それ以外は invalid_union
		{"空", `{}`, invalidUnionJSON, false, ""},
		{"名前が数", `{"name":1}`, invalidUnionJSON, false, ""},
		{"科目が不正", `{"name":"a","subject":"x"}`, invalidUnionJSON, false, ""},
		{"単位が null", `{"name":"a","rangeUnit":null}`, invalidUnionJSON, false, ""},
		{"名前が空・科目が不正", `{"name":"","subject":"x"}`, invalidUnionJSON, false, ""},
		{"マスターが文字列", `{"masterId":"1"}`, invalidUnionJSON, false, ""},
		{"マスターが小数", `{"masterId":1.5}`, invalidUnionJSON, false, ""},
		{"マスターが負の小数", `{"masterId":-1.5}`, invalidUnionJSON, false, ""},
		{"両方とも型でない誤り", `{"name":"","masterId":0}`, invalidUnionJSON, false, ""},
		{"名前が長い・マスターが負", `{"name":"` + strings.Repeat("a", 101) + `","masterId":-1}`, invalidUnionJSON, false, ""},
		{"本文が null", `null`, invalidUnionJSON, false, ""},
		{"本文が文字列", `"x"`, invalidUnionJSON, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, issue := readTextbookInput(httpxtest.ParseBody(t, tt.body))
			if tt.want != "" {
				if issue == nil {
					t.Fatalf("通ってしまった: %+v", input)
				}
				res := httptest.NewRecorder()
				issue.Write(res)
				httpxtest.AssertJSONEqual(t, res.Body.String(), tt.want)
				return
			}
			if issue != nil {
				t.Fatalf("弾かれた: %+v", issue)
			}
			got := input.name
			if input.fromMaster {
				got = strconv.FormatInt(input.masterID, 10)
			}
			if input.fromMaster != tt.fromMaster || got != tt.value {
				t.Errorf("fromMaster=%v value=%q, want fromMaster=%v value=%q", input.fromMaster, got, tt.fromMaster, tt.value)
			}
		})
	}
}

func TestReadTextbookProgress(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"空でも通る", `{}`, ""},
		{"全部", `{"totalAmount":100000,"rangeUnit":"page","targetDate":"2024-02-29","subject":null}`, ""},
		{"目標日は null で消せる", `{"targetDate":null}`, ""},
		{"0000年のうるう日", `{"targetDate":"0000-02-29"}`, ""},
		{"総量が0", `{"totalAmount":0}`, httpxtest.IssueJSON("1以上で入力してください", "too_small", "totalAmount")},
		{"総量が負", `{"totalAmount":-5}`, httpxtest.IssueJSON("1以上で入力してください", "too_small", "totalAmount")},
		{"総量が小数", `{"totalAmount":1.5}`, httpxtest.IssueJSON("整数で入力してください", "invalid_type", "totalAmount")},
		{"総量が文字列", `{"totalAmount":"1"}`, httpxtest.IssueJSON("Invalid input: expected number, received string", "invalid_type", "totalAmount")},
		{"総量が多すぎる", `{"totalAmount":100001}`, httpxtest.IssueJSON("100000以下で入力してください", "too_big", "totalAmount")},
		{"総量が安全な整数を超える", `{"totalAmount":1e20}`, httpxtest.IssueJSON("整数で入力してください", "too_big", "totalAmount")},
		{"総量が負に大きすぎる", `{"totalAmount":-1e20}`, httpxtest.IssueJSON("整数で入力してください", "too_small", "totalAmount")},
		{"単位が null", `{"rangeUnit":null}`, httpxtest.IssueJSON(rangeUnitIssue, "invalid_value", "rangeUnit")},
		{"暦に無い日付", `{"targetDate":"2026-02-30"}`, httpxtest.IssueJSON("Invalid ISO date", "invalid_format", "targetDate")},
		{"1900年はうるう年でない", `{"targetDate":"1900-02-29"}`, httpxtest.IssueJSON("Invalid ISO date", "invalid_format", "targetDate")},
		{"月が1桁", `{"targetDate":"2026-1-01"}`, httpxtest.IssueJSON("Invalid ISO date", "invalid_format", "targetDate")},
		{"13月", `{"targetDate":"2026-13-01"}`, httpxtest.IssueJSON("Invalid ISO date", "invalid_format", "targetDate")},
		{"目標日が空", `{"targetDate":""}`, httpxtest.IssueJSON("Invalid ISO date", "invalid_format", "targetDate")},
		{"目標日の前に空白", `{"targetDate":" 2026-01-01"}`, httpxtest.IssueJSON("Invalid ISO date", "invalid_format", "targetDate")},
		{"目標日が数", `{"targetDate":1}`, httpxtest.IssueJSON("Invalid input: expected string, received number", "invalid_type", "targetDate")},
		{"科目が不正", `{"subject":"x"}`, httpxtest.IssueJSON(subjectIssue, "invalid_value", "subject")},
		{"総量の誤りが科目より先", `{"subject":"x","totalAmount":0}`, httpxtest.IssueJSON("1以上で入力してください", "too_small", "totalAmount")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, _ := readTextbookProgress(httpxtest.ParseBody(t, tt.body))
			httpxtest.CheckIssue(t, in, tt.want)
		})
	}
}
