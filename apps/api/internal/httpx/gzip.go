package httpx

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
)

// GzipMinSize より小さい応答は、縮めても得が少ないので圧縮しない。
const GzipMinSize = 1024

// GzipBytes は本文を gzip にする。起動時に作り置く分は最高圧縮、リクエストのたびに作る HTML は既定の強さにする。
func GzipBytes(body []byte, level int) []byte {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, level)
	_, _ = w.Write(body)
	_ = w.Close()
	return buf.Bytes()
}

// WriteMaybeGzip は本文を返す。受け付けるなら gzip にする（小さいものはそのまま）。Content-Type は呼び出し側が付ける。
func WriteMaybeGzip(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	if len(body) >= GzipMinSize {
		h.Add("Vary", "Accept-Encoding")
		if AcceptsGzip(r.Header.Get("Accept-Encoding")) {
			h.Set("Content-Encoding", "gzip")
			body = GzipBytes(body, gzip.DefaultCompression)
		}
	}
	h.Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
