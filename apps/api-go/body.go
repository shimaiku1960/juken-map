package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
)

// 書き込みの本文の読み方（JUK-80）。Node の Fastify が本文を解析する部分（server.ts の
// addContentTypeParser と csp-report.ts）と同じ結果にする。
//
//   - 受け付ける Content-Type は4つ。それ以外は 415。本文が無ければ Content-Type は無くてよい
//   - 上限を超えたら 413（ふだんは 1MiB、ルートごとに小さくできる）
//   - JSON が壊れていても 400 にせず「本文なし」として扱い、判断をハンドラに任せる。
//     認証を通っていない送り手が、JSON の中身で結果を知ることがないようにするため（server.ts のコメント）
//
// 認証・トークンの確認はルーターが先に済ませるので、ここに来るのは通してよいリクエストだけ
// （Node も onRequest の拒否が本文の解析より先に走る）。

// defaultBodyLimit は Node の BODY_LIMIT（error-handling.ts）と同じ 1MiB。
const defaultBodyLimit = 1 << 20

// bodyMediaTypes は受け付ける Content-Type と、その本文を JSON として読むか。
// text/plain は Fastify の既定の解析（文字列のまま）で、JSON のオブジェクトにはならない。
var bodyMediaTypes = map[string]bool{
	"application/json":         true,
	"application/csp-report":   true,
	"application/reports+json": true,
	"text/plain":               false,
}

// requestBody は読んだ本文。JSON は読めたときだけ入る（壊れていれば nil＝Node の undefined）。
type requestBody struct {
	raw  []byte
	json any
}

// readBody は本文を読む。受け付けない形なら 415、大きすぎれば 413 を書いて ok=false を返す。
func readBody(w http.ResponseWriter, r *http.Request, limit int64) (body requestBody, ok bool) {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		// Content-Type の無い本文は Fastify も読めない。本文が無いなら何も読まずに通す。
		if r.ContentLength == 0 {
			return requestBody{}, true
		}
		rejectBody(w, r, http.StatusUnsupportedMediaType, ServerErrorCodeInvalidMediaType)
		return requestBody{}, false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	asJSON, known := bodyMediaTypes[mediaType]
	if err != nil || !known {
		rejectBody(w, r, http.StatusUnsupportedMediaType, ServerErrorCodeInvalidMediaType)
		return requestBody{}, false
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		rejectBody(w, r, http.StatusRequestEntityTooLarge, ServerErrorCodeBodyTooLarge)
		return requestBody{}, false
	}
	if err != nil {
		internalError(w, r, err)
		return requestBody{}, false
	}

	body = requestBody{raw: raw}
	if asJSON && len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		// 数は json.Number で受ける（1.5 と 1 を区別して、整数かどうかを Zod と同じく確かめるため）。
		dec.UseNumber()
		var v any
		// JSON.parse と同じく、値の後ろに余計なものがあれば読めなかった扱いにする。
		if dec.Decode(&v) == nil && dec.Decode(new(any)) == io.EOF {
			body.json = v
		}
	}
	return body, true
}

// rejectBody は本文を読まずに断る。Node の setErrorHandler と同じく、4xx は warn でログに残す。
func rejectBody(w http.ResponseWriter, r *http.Request, status int, code ServerErrorCode) {
	slog.WarnContext(r.Context(), "request rejected", "statusCode", status, "code", code)
	writeErrorBody(w, r, status, code)
}
