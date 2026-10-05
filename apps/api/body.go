package main

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// 書き込みの本文の読み方（JUK-80・JUK-75）。Node の Fastify が本文を解析する部分（server.ts の
// addContentTypeParser と csp-report.ts）と同じ結果にする。Node に実際に送った結果と、
// Fastify 5.12 のソース（lib/content-type.js・content-type-parser.js・handle-request.js）で確かめた。
//
//   - Content-Type が無く、本文も無い（Content-Length が無いか 0、かつ chunked でない）→ 本文なし
//   - Content-Type が無いのに本文がある、または下の4つ以外 → 415（本文が空でも）
//   - 上限を超えたら 413（ふだんは 1MiB、ルートごとに小さくできる）。Content-Length の申告だけで超えていても 413
//   - JSON が空・壊れていても 400 にせず「本文なし」として扱い、判断をハンドラに任せる。
//     認証を通っていない送り手が、JSON の中身で結果を知ることがないようにするため（server.ts のコメント）
//   - text/plain は文字列のまま
//
// 認証・トークンの確認はルーターが先に済ませるので、ここに来るのは通してよいリクエストだけ
// （Node も onRequest の拒否が本文の解析より先に走る）。

// defaultBodyLimit は Node の BODY_LIMIT（error-handling.ts）と同じ 1MiB。
const defaultBodyLimit = 1 << 20

// bodyMediaTypes は受け付ける Content-Type と、その本文を JSON として読むか。
// CSP の報告の2つは csp-report.ts がアプリ全体に登録しているので、どのルートでも JSON として読む。
var bodyMediaTypes = map[string]bool{
	"application/json":         true,
	"application/csp-report":   true,
	"application/reports+json": true,
	"text/plain":               false,
}

// requestBody は読んだ本文。
type requestBody struct {
	raw string
	// json は JSON として読めたときの値（数は json.Number）。読めなかった・本文が無い・JSON の null のときは nil。
	json any
	// parsed は JSON として読めたか（本文が null なら json は nil のまま true）。
	parsed bool
	// text は text/plain で受けたか。
	text bool
}

// jsUndefined は「値が無い」を表す。JSON の null（nil）と区別するため、別の値にする。
// Node で言えば request.body が undefined のとき、オブジェクトにキーが無いときにあたる。
type jsUndefined struct{}

// value は Node のハンドラが受け取る request.body と同じ値を返す。入力チェック（validate.go）に渡す。
//   - 本文なし・JSON として読めない：jsUndefined{}
//   - text/plain：string
//   - JSON：nil（null）・bool・json.Number・string・[]any・map[string]any
func (b requestBody) value() any {
	switch {
	case b.text:
		return b.raw
	case b.parsed:
		return b.json
	}
	return jsUndefined{}
}

// readBody は本文を読む。受け付けない形なら 415、大きすぎれば 413、UTF-8 として不正で
// Content-Length と合わなければ 400 を書いて ok=false を返す。
func readBody(w http.ResponseWriter, r *http.Request, limit int64) (body requestBody, ok bool) {
	ct, hasCT := r.Header["Content-Type"]
	if !hasCT {
		if isEmptyBody(r) {
			return requestBody{}, true
		}
		rejectBody(w, r, http.StatusUnsupportedMediaType, ServerErrorCodeInvalidMediaType)
		return requestBody{}, false
	}
	asJSON, known := bodyMediaTypes[mediaType(ct[0])]
	if !known {
		rejectBody(w, r, http.StatusUnsupportedMediaType, ServerErrorCodeInvalidMediaType)
		return requestBody{}, false
	}

	text, status, code, err := readBodyText(r, limit)
	switch {
	case err != nil:
		internalError(w, r, err)
		return requestBody{}, false
	case status != 0:
		rejectBody(w, r, status, code)
		return requestBody{}, false
	}

	body = requestBody{raw: text, text: !asJSON}
	if asJSON && text != "" {
		if v, err := parseJSON(text); err == nil {
			body.json, body.parsed = v, true
		}
	}
	return body, true
}

// isEmptyBody は Fastify の isEmptyBody と同じ。chunked なら中身が空でも「本文あり」。
// r.ContentLength は、サーバーが Content-Length ヘッダーから入れる値（無ければ 0、chunked なら -1）。
func isEmptyBody(r *http.Request) bool {
	return len(r.TransferEncoding) == 0 && r.ContentLength == 0
}

// Fastify の typeNameReg・subtypeNameReg（lib/content-type.js）。
var (
	mediaTypeNamePattern    = regexp.MustCompile("^[\\w!#$%&'*+.^`|~-]+$")
	mediaSubtypeNamePattern = regexp.MustCompile("^[\\w!#$%&'*+.^`|~-]+\\s*$")
)

// mediaType は Content-Type から "type/subtype" を小文字で取り出す。Fastify の ContentType と同じく、
// 形が不正なら "" を返す（呼び出し側で 415 になる）。; より後ろ（charset など）は見ない。
// mime.ParseMediaType は ; の後ろが壊れているとエラーにするが、Fastify は受け付けるので使わない。
func mediaType(header string) string {
	value, _, _ := strings.Cut(header, ";")
	typ, subtype, ok := strings.Cut(strings.ToLower(value), "/")
	if !ok {
		return ""
	}
	typ, subtype = strings.TrimLeft(typ, " \t\r\n"), strings.TrimRight(subtype, " \t\r\n")
	if !mediaTypeNamePattern.MatchString(typ) || !mediaSubtypeNamePattern.MatchString(subtype) {
		return ""
	}
	return typ + "/" + subtype
}

// readBodyText は本文を UTF-8 の文字列として読む。Fastify の rawBody（parseAs: "string"）と同じ判定で、
// 断るときは status（413 か 400）と code を返す。読み取りそのものの失敗は err で返す。
//
// Fastify は本文を UTF-8 として読み、読めないバイトを U+FFFD（3バイト）に置き換えてから長さを数える。
// そのため、不正なバイトを含むと数えた長さが Content-Length と合わず 400 になる。ここでも置き換えた後の
// 長さで同じ判定をする。
func readBodyText(r *http.Request, limit int64) (text string, status int, code ServerErrorCode, err error) {
	// chunked のときは -1（長さの申告が無い）。
	declared := r.ContentLength
	hasDeclared := declared >= 0
	if hasDeclared && declared > limit {
		return "", http.StatusRequestEntityTooLarge, ServerErrorCodeBodyTooLarge, nil
	}

	// 上限より1バイト多く読めたら、上限を超えている。
	raw, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return "", 0, "", err
	}
	if int64(len(raw)) > limit {
		return "", http.StatusRequestEntityTooLarge, ServerErrorCodeBodyTooLarge, nil
	}

	text = decodeUTF8Like(raw)
	if int64(len(text)) > limit {
		return "", http.StatusRequestEntityTooLarge, ServerErrorCodeBodyTooLarge, nil
	}
	if hasDeclared && int64(len(text)) != declared {
		return "", http.StatusBadRequest, ServerErrorCodeInvalidContentLength, nil
	}
	return text, 0, "", nil
}

// decodeUTF8Like は、Node の TextDecoder（WHATWG の UTF-8 デコーダ）と同じ規則で不正なバイトを U+FFFD にする。
// 正しい UTF-8 ならそのまま返す。
//
// Go の string([]rune(…)) や strings.ToValidUTF8 とは置き換える単位が違う。WHATWG は
// 「正しい並びの途中で切れた部分（最大で3バイト）」を1つの U+FFFD にし、それ以外の不正なバイトは
// 1バイトずつ U+FFFD にする（Unicode の「maximal subpart」）。
func decodeUTF8Like(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var out strings.Builder
	out.Grow(len(b) + 16)
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || size > 1 {
			out.Write(b[i : i+size])
			i += size
			continue
		}
		out.WriteRune(utf8.RuneError)
		i += maximalSubpart(b[i:])
	}
	return out.String()
}

// maximalSubpart は、b の先頭の不正な並びのうち、1つの U+FFFD にまとめるバイト数を返す（1〜3）。
// 先頭バイトが示す長さの途中まで、正しい続きのバイトが並んでいる部分をまとめる。
func maximalSubpart(b []byte) int {
	lead := b[0]
	var need int
	lower, upper := byte(0x80), byte(0xBF)
	switch {
	case lead >= 0xC2 && lead <= 0xDF:
		need = 1
	case lead >= 0xE0 && lead <= 0xEF:
		need = 2
		switch lead {
		case 0xE0:
			lower = 0xA0
		case 0xED:
			upper = 0x9F
		}
	case lead >= 0xF0 && lead <= 0xF4:
		need = 3
		switch lead {
		case 0xF0:
			lower = 0x90
		case 0xF4:
			upper = 0x8F
		}
	default:
		return 1
	}
	n := 1
	for ; n <= need && n < len(b); n++ {
		c := b[n]
		if c < lower || c > upper {
			break
		}
		// 範囲の制限は2バイト目だけにかかる。
		lower, upper = 0x80, 0xBF
	}
	return n
}

// parseJSON は JSON.parse と同じく本文全体を1つの値として読む。前後の空白以外の余りがあれば失敗にする。
// 数は json.Number で受ける（1.5 と 1 を区別して、整数かどうかを Zod と同じく確かめるため。
// 範囲を超えた数も、JavaScript と同じく ±Infinity として読める。jsNumber を参照）。
func parseJSON(text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if rest := strings.TrimLeft(text[dec.InputOffset():], " \t\r\n"); rest != "" {
		return nil, errors.New("JSON の後ろに余分な文字がある")
	}
	return v, nil
}

// jsNumber は json.Number を JSON.parse と同じ float64 にする。範囲外の数は ±Infinity、小さすぎる数は 0。
// strconv.ParseFloat は範囲外で ErrRange を返すが、値は ±Inf・0 になっているのでそのまま使う。
func jsNumber(n json.Number) float64 {
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		// Decoder が数として読んだものなので、ここには来ない。
		return math.NaN()
	}
	return f
}

// rejectBody は本文を読まずに断る。Node の setErrorHandler と同じく、4xx は warn でログに残す。
func rejectBody(w http.ResponseWriter, r *http.Request, status int, code ServerErrorCode) {
	slog.WarnContext(r.Context(), "request rejected", "statusCode", status, "code", code)
	writeErrorBody(w, r, status, code)
}
