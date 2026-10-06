package httpx

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// 全員に同じで、変わるのは管理画面の編集だけという応答（大学一覧・参考書マスター）を、JSON にし終えた
// バイト列と gzip 版・ETag でメモリに持つ（JUK-50）。DB を引く手間だけでなく、JSON にする CPU も省ける。
//
// 管理画面の編集は Invalidate で捨てるので、編集はすぐ応答に出る。期限（ttl）は、DB を直接書き換えたとき
// （seed など）の保険。
// ⚠️ キャッシュはプロセスごとに持つ。無停止デプロイで新旧が並走する間に編集すると、捨てられるのは
// 編集を受けたプロセスだけで、もう一方は期限まで古い中身を返す。

// JSONSnapshot は応答の JSON と、その gzip 版・ETag。
// 圧縮は読み込みのときの1回だけなので、圧縮率を最大にしてよい。リクエストのたびに nginx が
// 圧縮し直すのを避ける。Go の標準ライブラリに br は無いので gzip だけ。
type JSONSnapshot struct {
	json      []byte
	gzip      []byte
	etag      string
	expiresAt time.Time
}

type JSONSnapshotCache struct {
	ttl time.Duration
	now func() time.Time
	// load は DB から応答の値を作る。JSON にするのはキャッシュの側で行う。
	load func(ctx context.Context) (any, error)

	snapshot atomic.Pointer[JSONSnapshot]
	// 期限切れの直後に同時に来たリクエストは、1回の読み込みを待ち合わせる。
	loading singleflight.Group
	// generation は Invalidate のたびに増える。読み込みの途中で捨てられたら、その結果は編集の前の
	// DB から作ったかもしれないので置かない。
	// 「世代を比べて置く」と「世代を進めて捨てる」の間に割り込まれないよう、両方を mu の中で行う。
	mu         sync.Mutex
	generation uint64
}

func NewJSONSnapshotCache(ttl time.Duration, load func(ctx context.Context) (any, error)) *JSONSnapshotCache {
	return &JSONSnapshotCache{ttl: ttl, now: time.Now, load: load}
}

// Get はスナップショットを返す。期限内なら DB を引かない。
func (c *JSONSnapshotCache) Get(ctx context.Context) (*JSONSnapshot, error) {
	if s := c.snapshot.Load(); s != nil && c.now().Before(s.expiresAt) {
		return s, nil
	}
	v, err, _ := c.loading.Do("", func() (any, error) {
		c.mu.Lock()
		generation := c.generation
		c.mu.Unlock()
		// 待ち合わせている全員のための読み込みなので、最初に来たリクエストが切断しても止めない。
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		value, err := c.load(loadCtx)
		if err != nil {
			return nil, err
		}
		s, err := newJSONSnapshot(value, c.now().Add(c.ttl))
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.generation == generation {
			c.snapshot.Store(s)
		}
		c.mu.Unlock()
		return s, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*JSONSnapshot), nil
}

// Invalidate は中身の元になる表を変えたら呼ぶ。次のリクエストで DB から作り直す。
// 読み込みの途中なら、その待ち合わせには加わらせず、新しく読み込ませる。
// 確定（commit）してから呼ぶこと。確定前に呼ぶと、別のリクエストが古い DB を読んで置き直せる。
func (c *JSONSnapshotCache) Invalidate() {
	c.mu.Lock()
	c.generation++
	c.snapshot.Store(nil)
	c.mu.Unlock()
	c.loading.Forget("")
}

func newJSONSnapshot(v any, expiresAt time.Time) (*JSONSnapshot, error) {
	body, err := marshalLikeJS(v)
	if err != nil {
		return nil, err
	}
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if _, err := w.Write(body); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	// ETag は JSON の SHA-1 を base64url にしたもの（Node の頃と同じ作り方）。JSON が同じなら値も同じなので、
	// 作り直しやプロセスの入れ替えをまたいでも、ブラウザが持っている ETag で 304 が返る。
	sum := sha1.Sum(body)
	return &JSONSnapshot{
		json:      body,
		gzip:      gz.Bytes(),
		etag:      `"` + base64.RawURLEncoding.EncodeToString(sum[:]) + `"`,
		expiresAt: expiresAt,
	}, nil
}

// marshalLikeJS は JSON.stringify と同じバイト列にする。< > & を書き換えず、末尾に改行を付けない。
func marshalLikeJS(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// WriteJSONSnapshot はスナップショットを返す。ブラウザには毎回確かめさせ（no-cache）、変わっていなければ
// 304 で本文を省く。ログインが要る応答なので共有キャッシュには置かせない（private）。
func WriteJSONSnapshot(w http.ResponseWriter, r *http.Request, snap *JSONSnapshot) {
	hdr := w.Header()
	hdr.Set("Cache-Control", "private, no-cache")
	hdr.Set("ETag", snap.etag)
	// 同じ URL でも Accept-Encoding で本文の形が変わる、と途中のキャッシュに伝える。
	hdr.Set("Vary", "Accept-Encoding")
	if matchesETag(r.Header.Get("If-None-Match"), snap.etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	hdr.Set("Content-Type", "application/json; charset=utf-8")
	body := snap.json
	// Content-Encoding を付けて返すと、nginx の gzip は圧縮し直さない。
	if AcceptsGzip(r.Header.Get("Accept-Encoding")) {
		hdr.Set("Content-Encoding", "gzip")
		body = snap.gzip
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// matchesETag は If-None-Match（カンマ区切りで複数並べられる）に etag が含まれるか。
// 途中で圧縮し直されると ETag は弱い形（W/"..."）で戻ってくるので、W/ を外して比べる。
func matchesETag(ifNoneMatch, etag string) bool {
	if ifNoneMatch == "" {
		return false
	}
	for _, v := range strings.Split(ifNoneMatch, ",") {
		if strings.TrimPrefix(strings.TrimSpace(v), "W/") == etag {
			return true
		}
	}
	return false
}

// AcceptsGzip は Accept-Encoding が gzip（または *）を受け付けるか。"gzip;q=0" は「受け付けない」。
func AcceptsGzip(acceptEncoding string) bool {
	for _, part := range strings.Split(acceptEncoding, ",") {
		name, params, _ := strings.Cut(strings.ToLower(strings.TrimSpace(part)), ";")
		name = strings.TrimSpace(name)
		if name != "gzip" && name != "*" {
			continue
		}
		rejected := false
		for _, p := range strings.Split(params, ";") {
			if q, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok && isZeroQ(q) {
				rejected = true
			}
		}
		if !rejected {
			return true
		}
	}
	return false
}

// isZeroQ は q の値が 0 か。空文字（"q="）も 0 とみなす（Node の Number(q) === 0 と同じ）。
func isZeroQ(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return true
	}
	f, err := strconv.ParseFloat(q, 64)
	return err == nil && f == 0
}
