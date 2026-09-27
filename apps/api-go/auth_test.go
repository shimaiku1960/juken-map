package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
)

// sign は better-call の signCookieValue と同じ形（encodeURIComponent("トークン.署名")）を作る。
func sign(token, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(token))
	return url.QueryEscape(token + "." + base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

func TestVerifySignedValue(t *testing.T) {
	secret := []byte("test-secret")
	valid := sign("abc123", "test-secret")

	// Go のテストは「表」で並べるのが定番。ケースを足すときは1行増やすだけで済む。
	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{"正しい署名", valid, "abc123", true},
		{"別の secret で署名したもの", sign("abc123", "other-secret"), "", false},
		{"トークンだけすり替えたもの", "zzz" + valid[len("abc123"):], "", false},
		{"署名が無い", "abc123", "", false},
		{"空", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := verifySignedValue(tt.raw, secret)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("verifySignedValue(%q) = (%q, %v), want (%q, %v)", tt.raw, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
