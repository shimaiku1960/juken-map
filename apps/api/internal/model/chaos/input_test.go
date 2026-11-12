package chaos

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSpecValidate(t *testing.T) {
	routes := []string{"GET /api/study-logs", "GET /api/health"}
	valid := map[Kind]Spec{
		KindLatency:         {Kind: KindLatency, Route: AllRoutes, Rate: 0.3, DelayMs: 2000, Duration: 10 * time.Minute},
		KindHTTPError:       {Kind: KindHTTPError, Route: "GET /api/study-logs", Rate: 1, StatusCode: 503, Duration: time.Minute},
		KindDBError:         {Kind: KindDBError, Route: AllRoutes, Rate: 0.1, Duration: time.Hour},
		KindOutboundTimeout: {Kind: KindOutboundTimeout, Route: AllRoutes, Rate: 0.5, DelayMs: MaxDelayMs, Duration: time.Minute},
	}
	for kind, s := range valid {
		if err := s.Validate(routes); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}

	invalid := []struct {
		name string
		edit func(*Spec)
	}{
		{"知らない種類", func(s *Spec) { s.Kind = "disk_full" }},
		{"対象外のルート", func(s *Spec) { s.Route = "GET /api/admin/chaos" }},
		{"割合が 0", func(s *Spec) { s.Rate = 0 }},
		{"割合が 1 を超える", func(s *Spec) { s.Rate = 1.01 }},
		{"終わる時刻が無い（長さ 0）", func(s *Spec) { s.Duration = 0 }},
		{"1時間を超える", func(s *Spec) { s.Duration = MaxDuration + time.Second }},
		{"待ち時間がリクエストの上限に届く", func(s *Spec) { s.DelayMs = MaxDelayMs + 1 }},
		{"遅延に statusCode", func(s *Spec) { s.StatusCode = 500 }},
	}
	for _, tt := range invalid {
		s := valid[KindLatency]
		tt.edit(&s)
		if err := s.Validate(routes); err == nil {
			t.Errorf("%s: 通ってしまった", tt.name)
		}
	}
	if err := (Spec{Kind: KindHTTPError, Route: AllRoutes, Rate: 1, StatusCode: 404, Duration: time.Minute}).Validate(routes); err == nil {
		t.Error("5xx でないステータスが通ってしまった")
	}
	if err := (Spec{Kind: KindDBError, Route: AllRoutes, Rate: 1, DelayMs: 10, Duration: time.Minute}).Validate(routes); err == nil {
		t.Error("db_error の delayMs が通ってしまった")
	}
}

func TestDecodeSpec(t *testing.T) {
	tests := []struct {
		name string
		body any
		want string // 弾いたときの文。通るなら ""
		spec Spec
	}{
		{"全部ある", map[string]any{"kind": "latency", "route": "*", "rate": json.Number("0.5"), "durationSeconds": json.Number("60"), "delayMs": json.Number("100"), "extra": true},
			"", Spec{Kind: KindLatency, Route: AllRoutes, Rate: 0.5, Duration: time.Minute, DelayMs: 100}},
		{"1.0 は整数", map[string]any{"kind": "http_error", "rate": json.Number("1"), "durationSeconds": json.Number("60.0"), "statusCode": json.Number("503")},
			"", Spec{Kind: KindHTTPError, Rate: 1, Duration: time.Minute, StatusCode: 503}},
		{"オブジェクトでない", []any{}, "本文は JSON のオブジェクトにしてください", Spec{}},
		{"rate が文字列", map[string]any{"rate": "0.5", "durationSeconds": json.Number("60")}, "rate は数にしてください", Spec{}},
		{"durationSeconds が小数", map[string]any{"rate": json.Number("0.5"), "durationSeconds": json.Number("1.5")}, "durationSeconds は整数にしてください", Spec{}},
		{"delayMs が小数", map[string]any{"rate": json.Number("0.5"), "durationSeconds": json.Number("60"), "delayMs": json.Number("0.5")}, "delayMs は整数にしてください", Spec{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeSpec(tt.body)
			if tt.want == "" {
				if err != nil || got != tt.spec {
					t.Fatalf("got %+v, %v; want %+v", got, err, tt.spec)
				}
				return
			}
			var ie InputError
			if !errors.As(err, &ie) || ie.Error() != tt.want {
				t.Fatalf("err = %v, want InputError %q", err, tt.want)
			}
		})
	}
}
