package httpx

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// エラー応答は次の2つの形にする（openapi/openapi.yaml の Error・ServerError）。
// 画面（apps/web の api-client.ts）が読むのは error だけ。
//
//   - ルートが自分で断るとき（未ログイン・ID の形が不正など）：{"error": "文言"}
//   - 想定外の失敗・混雑・存在しないパス：{"error": "文言", "code": "...", "reqId": "..."}

// code の値（openapi/openapi.yaml の ServerError）。
const (
	CodeInternal   = apischema.ServerErrorCodeInternal
	CodeOverloaded = apischema.ServerErrorCodeOverloaded
	CodeNotFound   = apischema.ServerErrorCodeNotFound
	// 利用者単位の回数制限（user_rate_limit.go）
	codeTooManyRequests = apischema.ServerErrorCodeTooManyRequests
)

// 利用者に見せる文言。5xx は原因を出さない（SQL やパスが画面に出ないように）。
const (
	FallbackClientMessage = "リクエストを処理できませんでした"
	ServerMessage         = "サーバー側で問題が発生しました"
	OverloadedMessage     = "ただいま混み合っています。少し待ってからもう一度お試しください"
)

// clientMessages は 4xx のうち、code ごとに決まった文言を出すもの。Node の error-handling.ts の CLIENT_MESSAGES と同じ。
var clientMessages = map[apischema.ServerErrorCode]string{
	apischema.ServerErrorCodeBodyTooLarge:     "送信されたデータが大きすぎます",
	apischema.ServerErrorCodeInvalidMediaType: "この形式のデータは受け取れません",
}

// newErrorBody は Node の errorBody と同じ規則で文言を選ぶ。
func newErrorBody(status int, code apischema.ServerErrorCode, reqID string) apischema.ServerError {
	message := FallbackClientMessage
	switch {
	case code == CodeOverloaded:
		message = OverloadedMessage
	case code == codeTooManyRequests:
		message = tooManyRequestsMessage
	case status >= 500:
		message = ServerMessage
	case clientMessages[code] != "":
		message = clientMessages[code]
	}
	return apischema.ServerError{Error: message, Code: code, ReqID: reqID}
}

// WriteErrorBody は code と reqId の付いたエラーを返す。
func WriteErrorBody(w http.ResponseWriter, r *http.Request, status int, code apischema.ServerErrorCode) {
	WriteJSON(w, status, newErrorBody(status, code, telemetry.RequestIDFrom(r.Context())))
}

// WriteError はルートが自分で断るときの {"error": "文言"} を返す。
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, apischema.Error{Error: message})
}

// InternalError は想定外の失敗を 500 で返し、原因はログにだけ残す。
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed", "err", err.Error(), "statusCode", 500, "code", CodeInternal)
	WriteErrorBody(w, r, http.StatusInternalServerError, CodeInternal)
}

// NotFound は、どのルートにも当たらなかったリクエストに返す。
func NotFound(w http.ResponseWriter, r *http.Request) {
	WriteErrorBody(w, r, http.StatusNotFound, CodeNotFound)
}

// IDPattern は path の {id}（数値の主キー）として受け付ける形。
// 15桁までにしているのは、画面（JavaScript）が Number にしたときに誤差が出ない範囲に揃えるため。
var IDPattern = regexp.MustCompile(`^[1-9][0-9]{0,14}$`)

// PathID は path の {name} を正の整数として読む。形が不正なら 400 を送って false を返す。
// 呼び出し側は `id, ok := PathID(w, r, "id"); if !ok { return }` で抜ける。
// 判定と文言は JUK-62 で決めた。
func PathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if !IDPattern.MatchString(raw) {
		WriteError(w, http.StatusBadRequest, "ID が正しくありません")
		return 0, false
	}
	// 形は上で確かめたので、ここで失敗することはない。
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id, true
}
