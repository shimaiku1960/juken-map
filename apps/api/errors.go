package main

import (
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
)

// エラー応答は Node（apps/api/src/error-handling.ts）と同じ2つの形にする。
// 画面（apps/web の api-client.ts）が読むのは error だけなので、形が揃っていれば
// どちらのサーバーが返しても画面の表示は変わらない。
//
//   - ルートが自分で断るとき（未ログイン・ID の形が不正など）：{"error": "文言"}
//   - 想定外の失敗・混雑・存在しないパス：{"error": "文言", "code": "...", "reqId": "..."}

// code の値。Node の setErrorHandler・overload.ts・spa.ts と同じ（openapi/openapi.yaml の ServerError）。
const (
	codeInternal   = ServerErrorCodeInternal
	codeOverloaded = ServerErrorCodeOverloaded
	codeNotFound   = ServerErrorCodeNotFound
	// 利用者単位の回数制限（user_rate_limit.go）。Node には無い
	codeTooManyRequests = ServerErrorCodeTooManyRequests
)

// 利用者に見せる文言。5xx は原因を出さない（SQL やパスが画面に出ないように）。
const (
	fallbackClientMessage = "リクエストを処理できませんでした"
	serverMessage         = "サーバー側で問題が発生しました"
	overloadedMessage     = "ただいま混み合っています。少し待ってからもう一度お試しください"
)

// clientMessages は 4xx のうち、code ごとに決まった文言を出すもの。Node の error-handling.ts の CLIENT_MESSAGES と同じ。
var clientMessages = map[ServerErrorCode]string{
	ServerErrorCodeBodyTooLarge:     "送信されたデータが大きすぎます",
	ServerErrorCodeInvalidMediaType: "この形式のデータは受け取れません",
}

// newErrorBody は Node の errorBody と同じ規則で文言を選ぶ。
func newErrorBody(status int, code ServerErrorCode, reqID string) ServerError {
	message := fallbackClientMessage
	switch {
	case code == codeOverloaded:
		message = overloadedMessage
	case code == codeTooManyRequests:
		message = tooManyRequestsMessage
	case status >= 500:
		message = serverMessage
	case clientMessages[code] != "":
		message = clientMessages[code]
	}
	return ServerError{Error: message, Code: code, ReqID: reqID}
}

// writeErrorBody は code と reqId の付いたエラーを返す。
func writeErrorBody(w http.ResponseWriter, r *http.Request, status int, code ServerErrorCode) {
	writeJSON(w, status, newErrorBody(status, code, requestIDFrom(r.Context())))
}

// writeError はルートが自分で断るときの {"error": "文言"} を返す。
// Node の reply.code(400).send({ error: "..." }) にあたる。
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, Error{Error: message})
}

// internalError は想定外の失敗を 500 で返し、原因はログにだけ残す。
// Node で言えば、ハンドラが throw して setErrorHandler に届いたときの動き。
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed", "err", err.Error(), "statusCode", 500, "code", codeInternal)
	writeErrorBody(w, r, http.StatusInternalServerError, codeInternal)
}

// notFound は、どのルートにも当たらなかったリクエストに返す（Node の spa.ts の setNotFoundHandler）。
func notFound(w http.ResponseWriter, r *http.Request) {
	writeErrorBody(w, r, http.StatusNotFound, codeNotFound)
}

// idPattern は path の {id}（数値の主キー）として受け付ける形。Node の idParamsSchema と同じ。
// 15桁までにしているのは、Node 側が Number にしたときに誤差が出ない範囲に揃えるため。
var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,14}$`)

// pathID は path の {name} を正の整数として読む。形が不正なら 400 を送って false を返す。
// 呼び出し側は `id, ok := pathID(w, r, "id"); if !ok { return }` で抜ける。
// Node の readIdParam（routes/params.ts）と同じ判定・同じ文言（JUK-62）。
func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if !idPattern.MatchString(raw) {
		writeError(w, http.StatusBadRequest, "ID が正しくありません")
		return 0, false
	}
	// 形は上で確かめたので、ここで失敗することはない。
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id, true
}
