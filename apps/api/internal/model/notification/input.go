package notification

import "github.com/shimaiku1960/juken-map/apps/api/internal/validation"

// 入力の決まり。契約は openapi/openapi.yaml の NotificationPreference（4つとも真偽値）。

// decodePreference は通知の設定の本文を読む。
func decodePreference(body any) (Preference, error) {
	in := validation.ReadObject(body)
	p := Preference{
		EmailMorningEnabled: in.Boolean("emailMorningEnabled"),
		EmailEveningEnabled: in.Boolean("emailEveningEnabled"),
		LineMorningEnabled:  in.Boolean("lineMorningEnabled"),
		LineEveningEnabled:  in.Boolean("lineEveningEnabled"),
	}
	if err := in.Err(); err != nil {
		return Preference{}, err
	}
	return p, nil
}
