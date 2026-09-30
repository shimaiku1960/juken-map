package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// リクエスト本文の読み方を Node（Fastify ＋ server.ts の JSON パーサー）に揃える（JUK-75）。
//
// Node の決まりは次のとおりで、ここでも同じ順に判定する。
//   - Content-Type が無く、本文も無い（Content-Length が無いか 0、かつ chunked でない）→ 本文なし
//   - Content-Type が無いのに本文がある、または application/json・text/plain 以外 → 415
//   - 本文が 1MiB を超える（Content-Length の申告だけで超えていても）→ 413
//   - application/json で、空か JSON として読めない → 400 にせず「本文なし」として扱う。
//     認証を通った後なので、ハンドラの入力チェックが「expected object, received undefined」で 400 にする
//     （server.ts の addContentTypeParser。解析の失敗で認証より先に 400 を返さないための作り）
//   - text/plain → 文字列（入力チェックが「received string」で弾く）
//
// 認証（401・403）は本文を読む前にルーターが済ませているので、ログインしていない人には
// 本文の中身で結果が変わることはない（Node も onRequest で先に断る）。

// bodyLimit は本文の上限。Node の BODY_LIMIT（error-handling.ts）と同じ 1MiB。
const bodyLimit = 1 << 20

// Node の CLIENT_MESSAGES と同じ文言。
const (
	bodyTooLargeMessage     = "送信されたデータが大きすぎます"
	invalidMediaTypeMessage = "この形式のデータは受け取れません"
)

// jsUndefined は「値が無い」を表す。JSON の null と区別するため、nil とは別の値にする。
// Node で言えば request.body が undefined のとき、オブジェクトにキーが無いときにあたる。
type jsUndefined struct{}

// readBody は本文を読み、JavaScript で JSON.parse したのと同じ値にして返す。
//   - 本文なし・JSON として読めない：jsUndefined{}
//   - text/plain：string
//   - JSON：nil（null）・bool・float64・string・[]any・map[string]any
//
// 413・415・400（Content-Length と中身が合わない）のときは応答を送って false を返す。
// 呼び出し側は `body, ok := readBody(w, r); if !ok { return }` で抜ける。
func readBody(w http.ResponseWriter, r *http.Request) (any, bool) {
	ct, hasCT := r.Header["Content-Type"]
	if !hasCT {
		if isEmptyBody(r) {
			return jsUndefined{}, true
		}
		writeClientError(w, r, http.StatusUnsupportedMediaType, BodyErrorCodeInvalidMediaType, invalidMediaTypeMessage)
		return nil, false
	}

	asJSON := false
	switch mediaType(ct[0]) {
	case "application/json":
		asJSON = true
	case "text/plain":
	default:
		writeClientError(w, r, http.StatusUnsupportedMediaType, BodyErrorCodeInvalidMediaType, invalidMediaTypeMessage)
		return nil, false
	}

	text, status, code := readBodyText(r)
	if status != 0 {
		message := fallbackClientMessage
		if status == http.StatusRequestEntityTooLarge {
			message = bodyTooLargeMessage
		}
		writeClientError(w, r, status, code, message)
		return nil, false
	}
	if !asJSON {
		return text, true
	}
	if text == "" {
		return jsUndefined{}, true
	}
	v, err := parseJSON(text)
	if err != nil {
		return jsUndefined{}, true
	}
	return v, true
}

// isEmptyBody は Fastify の isEmptyBody と同じ。chunked なら中身が空でも「本文あり」。
func isEmptyBody(r *http.Request) bool {
	if len(r.TransferEncoding) > 0 {
		return false
	}
	cl := r.Header.Get("Content-Length")
	return cl == "" || cl == "0"
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
// 失敗したときは status（413 か 400）と Node の code を返す。
//
// Fastify は本文を UTF-8 として読み、読めないバイトを U+FFFD（3バイト）に置き換えてから長さを数える。
// そのため、不正なバイトを含むと数えた長さが Content-Length と合わず 400 になる。ここでも置き換えた後の
// 長さで同じ判定をする。
func readBodyText(r *http.Request) (text string, status int, code BodyErrorCode) {
	const tooLarge = BodyErrorCodeTooLarge
	declared, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	hasDeclared := err == nil
	if hasDeclared && declared > bodyLimit {
		return "", http.StatusRequestEntityTooLarge, tooLarge
	}

	// 上限より1バイト多く読めたら、上限を超えている。
	raw, err := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
	switch {
	case err != nil:
		// 途中で切れた・読めなかった。Fastify も読み取りの失敗は 400 にする。
		return "", http.StatusBadRequest, BodyErrorCodeBadRequest
	case len(raw) > bodyLimit:
		return "", http.StatusRequestEntityTooLarge, tooLarge
	}

	text = decodeUTF8Like(raw)
	if len(text) > bodyLimit {
		return "", http.StatusRequestEntityTooLarge, tooLarge
	}
	if hasDeclared && int64(len(text)) != declared {
		return "", http.StatusBadRequest, BodyErrorCodeInvalidContentLength
	}
	return text, 0, ""
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
		if lead == 0xE0 {
			lower = 0xA0
		} else if lead == 0xED {
			upper = 0x9F
		}
	case lead >= 0xF0 && lead <= 0xF4:
		need = 3
		if lead == 0xF0 {
			lower = 0x90
		} else if lead == 0xF4 {
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

// parseJSON は JSON.parse と同じ値を作る。数は float64 にし、範囲を超えたものは JavaScript と同じく
// ±Infinity（小さすぎるものは 0）にする。Go の json.Unmarshal は範囲外の数をエラーにするので、
// 数は文字列のまま受け取ってから変換する。前後の空白以外の余りがあれば失敗にする（JSON.parse と同じ）。
func parseJSON(text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if rest := bytes.TrimLeft([]byte(text[dec.InputOffset():]), " \t\r\n"); len(rest) > 0 {
		return nil, errors.New("JSON の後ろに余分な文字がある")
	}
	return toJSValue(v), nil
}

func toJSValue(v any) any {
	switch x := v.(type) {
	case json.Number:
		f, err := strconv.ParseFloat(string(x), 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			// Decoder が数として読んだものなので、ここには来ない。
			return math.NaN()
		}
		return f
	case []any:
		for i := range x {
			x[i] = toJSValue(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = toJSValue(x[k])
		}
	}
	return v
}

// writeClientError は Fastify が本文の段階で断ったときの形（{error, code, reqId}）で返す。
// Node の setErrorHandler が 4xx に付ける文言と同じ。code は Fastify のエラーの名前（契約の BodyError）。
func writeClientError(w http.ResponseWriter, r *http.Request, status int, code BodyErrorCode, message string) {
	writeJSON(w, status, BodyError{Error: message, Code: code, ReqID: requestIDFrom(r.Context())})
}
