package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadBody(t *testing.T) {
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
