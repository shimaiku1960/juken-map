package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
)

// WriteJSON は値を JSON にして返す。Fastify がハンドラの戻り値を JSON にしている部分にあたる。
// Go ではハンドラが値を返すのではなく、ResponseWriter に自分で書き込む。
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// ヘッダー → ステータス → 本文の順。本文を書き始めた後のヘッダー変更は効かない。
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	// 既定では < > & を < などに書き換える（HTML に埋め込む場合の備え）。
	// Node の JSON.stringify は書き換えないので、応答を揃えるために止める。
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// ステータスはもう送ってしまったので、ここではログに残すことしかできない。
		slog.Error("write json", "err", err)
	}
}

// WriteValidationError は 400 を返す。Node と同じく、弾いた理由の最初の1件だけを返す。
func WriteValidationError(w http.ResponseWriter, code, field, message string) {
	WriteJSON(w, http.StatusBadRequest, apischema.ValidationError{Error: message, Code: code, Field: &field})
}
