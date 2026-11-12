package notification

import (
	"testing"

	vt "github.com/shimaiku1960/juken-map/apps/api/internal/validation/validationtest"
)

// 期待値は、契約（openapi/openapi.yaml の NotificationPreference）の「4つとも真偽値で必須」。
func TestDecodePreference(t *testing.T) {
	tests := []struct {
		name, body, want string
		pref             Preference
	}{
		{"4つとも真偽値", `{"emailMorningEnabled":true,"emailEveningEnabled":false,"lineMorningEnabled":false,"lineEveningEnabled":true}`,
			"", Preference{EmailMorningEnabled: true, LineEveningEnabled: true}},
		{"1つ足りない", `{"emailMorningEnabled":true,"emailEveningEnabled":false,"lineMorningEnabled":false}`,
			vt.Bad("Invalid input: expected boolean, received undefined", "invalid_type", "lineEveningEnabled"), Preference{}},
		{"文字列は通さない", `{"emailMorningEnabled":"true","emailEveningEnabled":false,"lineMorningEnabled":false,"lineEveningEnabled":false}`,
			vt.Bad("Invalid input: expected boolean, received string", "invalid_type", "emailMorningEnabled"), Preference{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodePreference(vt.Parse(t, tt.body))
			vt.CheckIssue(t, err, tt.want)
			if tt.want == "" && got != tt.pref {
				t.Errorf("got %+v, want %+v", got, tt.pref)
			}
		})
	}
}
