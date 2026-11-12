package httpx

import (
	"errors"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力チェックの規則と読み方は internal/validation にある（HTTP を知らないので、モデルからも使える。JUK-219）。
// ここは、弾いた理由を 400 の応答にするところだけを持つ。

// WriteValidationIssue は弾いた理由の1件を 400 で返す。field が "" なら応答の field は null。
func WriteValidationIssue(w http.ResponseWriter, v *validation.Issue) {
	body := apischema.ValidationError{Error: v.Message, Code: v.Code}
	if v.Field != "" {
		body.Field = &v.Field
	}
	WriteJSON(w, http.StatusBadRequest, body)
}

// Reject は in に issue があれば 400 を送って true を返す。
func Reject(w http.ResponseWriter, in *validation.ObjectInput) bool {
	if in.Issue == nil {
		return false
	}
	WriteValidationIssue(w, in.Issue)
	return true
}

// RejectIssue は err が入力の誤り（*validation.Issue。モデルの操作が入力の決まりで断ったもの）なら
// 400 を送って true を返す。それ以外の err には何もしない。
func RejectIssue(w http.ResponseWriter, err error) bool {
	var v *validation.Issue
	if !errors.As(err, &v) {
		return false
	}
	WriteValidationIssue(w, v)
	return true
}
