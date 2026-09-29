//go:build parity

// 応答一致テスト。動いている Node と Go に同じリクエストを送り、ステータス・本文・ヘッダーが
// 同じかを確かめる。API を1本 Go へ移すたびに、下の parityCases に1行足して使う。
//
// 両方のサーバーと DB が要るので、ふだんの go test では動かない（先頭の go:build parity）。
// parity.sh がサーバーを立ててから `go test -tags parity` で呼ぶ。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// who は誰としてリクエストを送るか。
type who int

const (
	anonymous   who = iota // Cookie なし
	forged                 // 署名の合わない Cookie
	unknownUser            // 署名は正しいが、DB に無いトークン
	eachUser               // 合成ユーザー全員（PARITY_COOKIES の1行ずつ）
)

var whoNames = map[who]string{anonymous: "未ログイン", forged: "偽の署名", unknownUser: "無いセッション", eachUser: "利用者"}

// parityCases が比べるリクエストの一覧。Go へ移した API を足していく。
var parityCases = []struct {
	method string
	path   string
	as     []who
}{
	{"GET", "/api/health", []who{anonymous}},
	{"GET", "/api/dashboard", []who{anonymous, forged, unknownUser, eachUser}},

	// 学習記録・予定の一覧（JUK-73）。期間を省いたとき（既定の期間）・両端を指定・上限なし
	{"GET", "/api/study-logs", []who{anonymous, forged, unknownUser, eachUser}},
	{"GET", "/api/study-logs?from=2026-01-01&to=2026-12-31", []who{eachUser}},
	{"GET", "/api/study-logs?from=2026-09-01", []who{eachUser}},
	{"GET", "/api/study-logs/daily", []who{anonymous, eachUser}},
	{"GET", "/api/study-logs/daily?from=2025-01-01&to=2026-09-15", []who{eachUser}},
	{"GET", "/api/study-plans", []who{anonymous, eachUser}},
	{"GET", "/api/study-plans?from=2026-09-01&to=2026-10-31", []who{eachUser}},
	// 不正な期間。400 の本文（Zod の issues）と、暦に無い日付で空の一覧になるところ
	{"GET", "/api/study-logs?from=abc&to=x", []who{eachUser}},
	{"GET", "/api/study-logs?from=2026-09-01&from=2026-09-02", []who{eachUser}},
	{"GET", "/api/study-logs?from=2026-09-28;to=x", []who{eachUser}},
	{"GET", "/api/study-logs?fr%6Fm=abc", []who{eachUser}},
	{"GET", "/api/study-logs?from", []who{eachUser}},
	{"GET", "/api/study-logs?from=%ZZ", []who{eachUser}},
	{"GET", "/api/study-logs?from=2026-13-45", []who{eachUser}},
	// 2月30日は3月2日に繰り越される（JavaScript の new Date と同じ）
	{"GET", "/api/study-logs?from=2026-02-30&to=2026-09-30", []who{eachUser}},
	{"GET", "/api/study-logs/daily?to=bad", []who{eachUser}},
	{"GET", "/api/study-plans?from=2026-09-01&to=2026-04-31", []who{eachUser}},
	{"GET", "/api/study-plans?to=2026-09-01&to=2026-09-02", []who{eachUser}},

	// 志望校（JUK-73）。第一志望が無い人は null、タグの無い学部は tags: []
	{"GET", "/api/goals", []who{anonymous, forged, unknownUser, eachUser}},
	{"GET", "/api/goals/first-choice", []who{anonymous, eachUser}},
	// クエリは読まない（Node も読まない）
	{"GET", "/api/goals?status=decided", []who{eachUser}},

	// どのルートにも当たらないもの。Go に無いものは Node にも無い（Node にあるものは移していないだけ）。
	{"GET", "/api/no-such-route", []who{anonymous}},
	{"POST", "/api/dashboard", []who{eachUser}},
}

// 比べるヘッダー。CSP は API 向けに変えているので比べない（middleware.go の securityHeaders）。
var parityHeaders = []string{
	"Content-Type",
	"Strict-Transport-Security",
	"X-Content-Type-Options",
	"X-Frame-Options",
	"Referrer-Policy",
	"Retry-After",
}

func TestParity(t *testing.T) {
	nodeURL, goURL := os.Getenv("PARITY_NODE_URL"), os.Getenv("PARITY_GO_URL")
	if nodeURL == "" || goURL == "" {
		t.Fatal("PARITY_NODE_URL と PARITY_GO_URL が要ります（parity.sh から動かしてください）")
	}
	users := strings.Fields(os.Getenv("PARITY_COOKIES"))
	if len(users) == 0 {
		t.Fatal("PARITY_COOKIES が空です")
	}
	cookieName, _, _ := strings.Cut(users[0], "=")

	cookiesFor := func(w who) []string {
		switch w {
		case forged:
			return []string{cookieName + "=" + sign("forged-token", "not-the-secret")}
		case unknownUser:
			return []string{cookieName + "=" + sign("no-such-token", os.Getenv("BETTER_AUTH_SECRET"))}
		case eachUser:
			return users
		}
		return []string{""}
	}

	for _, c := range parityCases {
		for _, w := range c.as {
			t.Run(fmt.Sprintf("%s %s（%s）", c.method, c.path, whoNames[w]), func(t *testing.T) {
				for i, cookie := range cookiesFor(w) {
					node := fetch(t, nodeURL, c.method, c.path, cookie)
					gon := fetch(t, goURL, c.method, c.path, cookie)
					if diffs := compareResponses(node, gon); len(diffs) > 0 {
						t.Fatalf("%d人目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
					}
				}
			})
		}
	}
}

type response struct {
	status int
	header http.Header
	body   any
}

func fetch(t *testing.T, base, method, path, cookie string) response {
	t.Helper()
	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	// Node は応答を圧縮し、Go は圧縮しない。本文を比べるので、どちらにも圧縮させない。
	req.Header.Set("Accept-Encoding", "identity")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s%s: %v", method, base, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("%s %s%s の本文が JSON でない: %q", method, base, path, raw)
	}
	return response{status: res.StatusCode, header: res.Header, body: body}
}

func compareResponses(node, gon response) []string {
	var diffs []string
	if node.status != gon.status {
		diffs = append(diffs, fmt.Sprintf("status: %d → %d", node.status, gon.status))
	}
	for _, name := range parityHeaders {
		if a, b := node.header.Get(name), gon.header.Get(name); a != b {
			diffs = append(diffs, fmt.Sprintf("%s: %q → %q", name, a, b))
		}
	}
	// reqId はリクエストごとに違うので、有るか無いかだけ比べる。
	if (node.header.Get("X-Request-Id") == "") != (gon.header.Get("X-Request-Id") == "") {
		diffs = append(diffs, "X-Request-Id の有無が違う")
	}
	return append(diffs, diffJSON("$", withoutReqID(node.body), withoutReqID(gon.body))...)
}

// withoutReqID は、エラー応答の reqId を「有る」という印に置き換える（値は毎回違うため）。
func withoutReqID(body any) any {
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	if _, has := m["reqId"]; has {
		copied := make(map[string]any, len(m))
		for k, v := range m {
			copied[k] = v
		}
		copied["reqId"] = "(present)"
		return copied
	}
	return body
}

// diffJSON は JSON を読んだ値どうしを比べ、違う場所を $.logs[3].textbook.name の形で返す。
// 何か所も違うと読み切れないので、最初の10か所までにする。
func diffJSON(path string, a, b any) []string {
	var out []string
	var walk func(path string, a, b any)
	walk = func(path string, a, b any) {
		if len(out) >= 10 {
			return
		}
		switch av := a.(type) {
		case map[string]any:
			bv, ok := b.(map[string]any)
			if !ok {
				break
			}
			keys := map[string]bool{}
			for k := range av {
				keys[k] = true
			}
			for k := range bv {
				keys[k] = true
			}
			sorted := make([]string, 0, len(keys))
			for k := range keys {
				sorted = append(sorted, k)
			}
			sort.Strings(sorted)
			for _, k := range sorted {
				x, inA := av[k]
				y, inB := bv[k]
				switch {
				case !inA:
					out = append(out, fmt.Sprintf("%s.%s: (無い) → %v", path, k, y))
				case !inB:
					out = append(out, fmt.Sprintf("%s.%s: %v → (無い)", path, k, x))
				default:
					walk(path+"."+k, x, y)
				}
			}
			return
		case []any:
			bv, ok := b.([]any)
			if !ok {
				break
			}
			if len(av) != len(bv) {
				out = append(out, fmt.Sprintf("%s: 長さ %d → %d", path, len(av), len(bv)))
				return
			}
			for i := range av {
				walk(fmt.Sprintf("%s[%d]", path, i), av[i], bv[i])
			}
			return
		}
		if !reflect.DeepEqual(a, b) {
			out = append(out, fmt.Sprintf("%s: %v → %v", path, a, b))
		}
	}
	walk(path, a, b)
	return out
}
