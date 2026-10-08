package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
)

// WriteJSON は値を JSON にして返す。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// ヘッダー → ステータス → 本文の順。本文を書き始めた後のヘッダー変更は効かない。
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	// 既定では < > & を < などに書き換える（HTML に埋め込む場合の備え）。
	// 応答は HTML に埋め込まないので止め、文字をそのまま返す（JavaScript の JSON.stringify と同じ）。
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// ステータスはもう送ってしまったので、ここではログに残すことしかできない。
		slog.Error("write json", "err", err)
	}
}

// WriteValidationError は 400 を返す。弾いた理由の最初の1件だけを返す。
func WriteValidationError(w http.ResponseWriter, code, field, message string) {
	WriteJSON(w, http.StatusBadRequest, apischema.ValidationError{Error: message, Code: code, Field: &field})
}
