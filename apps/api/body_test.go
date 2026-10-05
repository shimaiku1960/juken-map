package main

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadBodyMediaTypes(t *testing.T) {
	// Node（Fastify）で実際に確かめた結果と同じにする（2026-09-30、JUK-80）。
	tests := []struct {
		name        string
		contentType string
		body        string
		limit       int64
		wantStatus  int // 0 なら読めた
		wantJSON    bool
	}{
		{"JSON", "application/json", `{"a":1}`, 100, 0, true},
		{"CSP の報告", "application/csp-report", `{"csp-report":{}}`, 100, 0, true},
		{"report-to", "application/reports+json", `[]`, 100, 0, true},
		{"大文字と charset は無視", "APPLICATION/JSON; charset=utf-8", `{}`, 100, 0, true},
		{"text/plain は文字列のまま（JSON にしない）", "text/plain", `{"a":1}`, 100, 0, false},
		{"壊れた JSON は 400 にせず、本文なし", "application/json", `{no`, 100, 0, false},
		{"JSON の後ろに余計なもの", "application/json", `{} x`, 100, 0, false},
		{"空の JSON", "application/json", ``, 100, 0, false},
		{"Content-Type も本文も無い", "", ``, 100, 0, false},
		{"Content-Type が無いのに本文がある", "", `x`, 100, 415, false},
		{"フォーム", "application/x-www-form-urlencoded", `a=b`, 100, 415, false},
		{"似た名前", "application/csp-reportx", `{}`, 100, 415, false},
		{"上限ちょうど", "text/plain", strings.Repeat("x", 100), 100, 0, false},
		{"上限を1バイト超える", "text/plain", strings.Repeat("x", 101), 100, 413, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			res := httptest.NewRecorder()
			body, ok := readBody(res, req, tt.limit)

			if tt.wantStatus != 0 {
				if ok || res.Code != tt.wantStatus {
					t.Fatalf("ok = %v, status = %d, want %d", ok, res.Code, tt.wantStatus)
				}
				return
			}
			if !ok {
				t.Fatalf("読めなかった（status %d、本文 %s）", res.Code, res.Body)
			}
			if (body.json != nil) != tt.wantJSON {
				t.Errorf("json = %#v, want JSON: %v", body.json, tt.wantJSON)
			}
		})
	}
}

func TestReadBodyErrors(t *testing.T) {
	// 断るときの本文は Node の setErrorHandler と同じ（code は Fastify のもの）。
	for _, tt := range []struct {
		name, contentType, body, want string
	}{
		{"415", "application/xml", "<a/>", `{"error":"この形式のデータは受け取れません","code":"FST_ERR_CTP_INVALID_MEDIA_TYPE","reqId":""}`},
		{"413", "application/json", strings.Repeat("x", 11), `{"error":"送信されたデータが大きすぎます","code":"FST_ERR_CTP_BODY_TOO_LARGE","reqId":""}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			res := httptest.NewRecorder()
			readBody(res, req, 10)
			assertJSONEqual(t, res.Body.String(), tt.want)
		})
	}
}

func TestReadBodyNumbers(t *testing.T) {
	// 数は json.Number で受け取る（1 と 1.0 と 1.5 を、整数かどうかの判定まで区別できるように）。
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"seq":1.5}`))
	req.Header.Set("Content-Type", "application/json")
	body, ok := readBody(httptest.NewRecorder(), req, defaultBodyLimit)
	if !ok {
		t.Fatal("読めなかった")
	}
	if n, isNumber := body.json.(map[string]any)["seq"].(json.Number); !isNumber || n.String() != "1.5" {
		t.Errorf("seq = %#v", body.json)
	}
}

// 期待値は、Node の API（当時の apps/api）に同じリクエストを送って返ってきたもの（2026-09-30 に手元で確かめた）。

// prefsHandler は PUT /api/notification-preferences の入力チェックまでを通す。保存の代わりに 200 で入力を返す。
func prefsHandler(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	in := readObject(body.value())
	p := NotificationPreference{
		EmailMorningEnabled: in.boolean("emailMorningEnabled"),
		EmailEveningEnabled: in.boolean("emailEveningEnabled"),
		LineMorningEnabled:  in.boolean("lineMorningEnabled"),
		LineEveningEnabled:  in.boolean("lineEveningEnabled"),
	}
	if in.reject(w) {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func TestReadBodyLikeNode(t *testing.T) {
	const valid = `{"emailMorningEnabled":true,"emailEveningEnabled":false,"lineMorningEnabled":false,"lineEveningEnabled":false}`
	undefinedBody := `{"error":"Invalid input: expected object, received undefined","code":"invalid_type","field":null}`
	missingFirst := `{"error":"Invalid input: expected boolean, received undefined","code":"invalid_type","field":"emailMorningEnabled"}`
	tooLarge := `{"error":"送信されたデータが大きすぎます","code":"FST_ERR_CTP_BODY_TOO_LARGE","reqId":""}`
	badMedia := `{"error":"この形式のデータは受け取れません","code":"FST_ERR_CTP_INVALID_MEDIA_TYPE","reqId":""}`

	tests := []struct {
		name        string
		contentType string // "-" なら Content-Type を付けない
		body        string
		chunked     bool
		wantStatus  int
		wantBody    string
	}{
		{"正しい本文", "application/json", valid, false, 200, valid},
		{"余分な項目は無視する", "application/json", strings.TrimSuffix(valid, "}") + `,"zzz":1}`, false, 200, valid},
		{"同じキーは後の値", "application/json", `{"emailMorningEnabled":1,` + valid[1:], false, 200, valid},
		{"__proto__ はただの項目", "application/json", `{"__proto__":{"x":1}}`, false, 400, missingFirst},

		{"本文も Content-Type も無い", "-", "", false, 400, undefinedBody},
		{"Content-Type が無いのに本文がある", "-", `{}`, false, 415, badMedia},
		{"Content-Type が無く、chunked", "-", "", true, 415, badMedia},
		{"JSON で本文が空", "application/json", "", false, 400, undefinedBody},
		{"JSON で chunked の空", "application/json", "", true, 400, undefinedBody},
		{"壊れた JSON は本文なし扱い", "application/json", `{a`, false, 400, undefinedBody},
		{"JSON の後ろに余り", "application/json", `{} x`, false, 400, undefinedBody},
		{"空白だけ", "application/json", `   `, false, 400, undefinedBody},
		{"BOM 付きは読めない（JSON.parse と同じ）", "application/json", "\xef\xbb\xbf{}", false, 400, undefinedBody},
		{"null", "application/json", `null`, false, 400,
			`{"error":"Invalid input: expected object, received null","code":"invalid_type","field":null}`},
		{"配列", "application/json", `[]`, false, 400,
			`{"error":"Invalid input: expected object, received array","code":"invalid_type","field":null}`},
		{"数", "application/json", `1`, false, 400,
			`{"error":"Invalid input: expected object, received number","code":"invalid_type","field":null}`},
		{"項目の型が違う", "application/json", `{"emailMorningEnabled":"yes"}`, false, 400,
			`{"error":"Invalid input: expected boolean, received string","code":"invalid_type","field":"emailMorningEnabled"}`},
		{"2つ目の項目が無い", "application/json", `{"emailMorningEnabled":true}`, false, 400,
			`{"error":"Invalid input: expected boolean, received undefined","code":"invalid_type","field":"emailEveningEnabled"}`},

		{"text/plain は文字列", "text/plain", valid, false, 400,
			`{"error":"Invalid input: expected object, received string","code":"invalid_type","field":null}`},
		{"text/plain の空も文字列", "text/plain", "", false, 400,
			`{"error":"Invalid input: expected object, received string","code":"invalid_type","field":null}`},
		{"フォームは 415", "application/x-www-form-urlencoded", "a=1", false, 415, badMedia},
		{"本文が無くても形式が違えば 415", "text/html", "", false, 415, badMedia},
		{"+json も 415", "application/vnd.api+json", `{}`, false, 415, badMedia},
		{"application/ だけは 415", "application/", `{}`, false, 415, badMedia},
		{"charset 付き", "application/json; charset=utf-8", `{}`, false, 400, missingFirst},
		{"大文字", "APPLICATION/JSON", `{}`, false, 400, missingFirst},
		{"; の後ろが壊れていても読む", "application/json ;;; x", `{}`, false, 400, missingFirst},
		{"前後の空白", "  application/json ", `{}`, false, 400, missingFirst},

		{"1MiB ちょうどは読む", "application/json", strings.Repeat(" ", defaultBodyLimit), false, 400, undefinedBody},
		{"1MiB を超えたら 413", "application/json", strings.Repeat(" ", defaultBodyLimit+1), false, 413, tooLarge},
		{"chunked でも 413", "application/json", strings.Repeat(" ", defaultBodyLimit+1), true, 413, tooLarge},
		{"text/plain も 413", "text/plain", strings.Repeat(" ", defaultBodyLimit+1), false, 413, tooLarge},
		{"UTF-8 として不正なバイトは Content-Length と合わず 400", "application/json", "{\"a\":\"\xff\"}", false, 400,
			`{"error":"リクエストを処理できませんでした","code":"FST_ERR_CTP_INVALID_CONTENT_LENGTH","reqId":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/notification-preferences", strings.NewReader(tt.body))
			if tt.contentType != "-" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.chunked {
				req.TransferEncoding = []string{"chunked"}
				req.ContentLength = -1
			}
			res := httptest.NewRecorder()
			prefsHandler(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", res.Code, tt.wantStatus, res.Body)
			}
			assertJSONEqual(t, res.Body.String(), tt.wantBody)
		})
	}
}

func TestReadBodyDeclaredTooLarge(t *testing.T) {
	// Content-Length の申告だけで上限を超えていれば、本文を読まずに 413（Fastify と同じ）。
	req := httptest.NewRequest("PUT", "/", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 2000000
	res := httptest.NewRecorder()
	prefsHandler(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", res.Code)
	}
}

func TestJSNumber(t *testing.T) {
	// JSON.parse と同じく、範囲外の数は ±Infinity、小さすぎる数は 0 になる。
	tests := map[string]float64{
		"1": 1, "1.5": 1.5, "1e3": 1000, "-0": math.Copysign(0, -1),
		"1e400": math.Inf(1), "-1e400": math.Inf(-1), "1e-400": 0,
	}
	for in, want := range tests {
		if got := jsNumber(json.Number(in)); got != want || math.Signbit(got) != math.Signbit(want) {
			t.Errorf("jsNumber(%s) = %v, want %v", in, got, want)
		}
	}
}

func TestDecodeUTF8Like(t *testing.T) {
	// TextDecoder（WHATWG）と同じく、途中で切れた並びは1つの U+FFFD、それ以外の不正なバイトは1バイトずつ。
	// 期待値は Node の Buffer.from(bytes).toString("utf8") の結果。
	const r = "\xef\xbf\xbd" // U+FFFD
	tests := map[string]string{
		"abc":               "abc",
		"\xff":              r,
		"\xe2\x82":          r,             // 3バイトの文字の途中で切れた
		"\xe2\x82x":         r + "x",       // 同上。続く x はそのまま
		"\xf0\x9f\x98":      r,             // 4バイトの文字の途中で切れた
		"\xed\xa0\x80":      r + r + r,     // サロゲートの範囲は2バイト目で不正になる
		"\xe0\x80\x80":      r + r + r,     // 冗長な表現も同じ
		"\xc0\xaf":          r + r,         // C0 は先頭に来てはいけない
		"\xf4\x90\x80\x80":  r + r + r + r, // U+10FFFF を超える
		"a\xe3\x81\x82\xff": "aあ" + r,
	}
	for in, want := range tests {
		if got := decodeUTF8Like([]byte(in)); got != want {
			t.Errorf("decodeUTF8Like(%q) = %q, want %q", in, got, want)
		}
	}
}
