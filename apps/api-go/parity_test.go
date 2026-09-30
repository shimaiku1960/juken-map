//go:build parity

// 応答一致テスト。動いている Node と Go に同じリクエストを送り、ステータス・本文・ヘッダーが
// 同じかを確かめる。API を1本 Go へ移すたびに、下の parityCases に1行足して使う。
//
// 両方のサーバーと DB が要るので、ふだんの go test では動かない（先頭の go:build parity）。
// parity.sh がサーバーを立ててから `go test -tags parity` で呼ぶ。
package main

import (
	"bytes"
	"compress/gzip"
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

	// 参考書（JUK-73）。マスターは全員に同じもの
	{"GET", "/api/textbooks", []who{anonymous, forged, unknownUser, eachUser}},
	{"GET", "/api/textbook-masters", []who{anonymous, eachUser}},

	// 通知設定（JUK-73）。保存していない人は全部 false
	{"GET", "/api/notification-preferences", []who{anonymous, forged, unknownUser, eachUser}},

	// 大学（JUK-73）。一覧は全員に同じもの。詳細は登録済みの学部が人によって違う（1 は学部なし、258 は13学部）
	{"GET", "/api/universities", []who{anonymous, forged, unknownUser, eachUser}},
	{"GET", "/api/universities/1", []who{anonymous, eachUser}},
	{"GET", "/api/universities/258", []who{eachUser}},
	{"GET", "/api/universities/999999", []who{eachUser}},
	{"GET", "/api/universities/abc", []who{eachUser}},
	{"GET", "/api/universities/0", []who{eachUser}},
	{"POST", "/api/universities/1", []who{eachUser}},

	// 毎日の通知（JUK-74）。トークンが無ければ 401（手元の .env は DAILY_NOTIFICATION_SECRET が空なので、
	// どちらも必ず 401 になり、実際には送らない）
	{"POST", "/api/cron/daily-study-notifications", []who{anonymous, eachUser}},

	// どのルートにも当たらないもの。Go に無いものは Node にも無い（Node にあるものは移していないだけ）。
	{"GET", "/api/no-such-route", []who{anonymous}},
	{"POST", "/api/dashboard", []who{eachUser}},
}

// parityWrites は本文やトークンの付いた書き込みの比較（JUK-80）。どれも DB を変えない入力にしてある
// （変えるものは下の TestParityAnalytics・TestParitySimStateful で順番を決めて比べる）。
var parityWrites = []struct {
	name        string
	method      string
	path        string
	contentType string
	body        string
	as          []who
	simToken    string // "ok" なら正しい SIMULATION_SECRET、"bad" なら違う値を Bearer で送る
}{
	// CSP の報告。誰でも送れて、読めない本文でも 204。形が違えば 415、16KB を超えたら 413
	{"report-uri", "POST", "/api/csp-report", "application/csp-report", `{"csp-report":{"blocked-uri":"inline"}}`, []who{anonymous, eachUser}, ""},
	{"report-to", "POST", "/api/csp-report", "application/reports+json", `[{"type":"csp-violation","body":{}}]`, []who{anonymous}, ""},
	{"壊れた JSON", "POST", "/api/csp-report", "application/csp-report", `{not`, []who{anonymous}, ""},
	{"JSON", "POST", "/api/csp-report", "application/json", `{}`, []who{anonymous}, ""},
	{"text/plain", "POST", "/api/csp-report", "text/plain", `hello`, []who{anonymous}, ""},
	{"本文なし", "POST", "/api/csp-report", "", ``, []who{anonymous}, ""},
	{"Content-Type なしで本文あり", "POST", "/api/csp-report", "", `x`, []who{anonymous}, ""},
	{"フォーム", "POST", "/api/csp-report", "application/x-www-form-urlencoded", `a=b`, []who{anonymous}, ""},
	{"16KB ちょうど", "POST", "/api/csp-report", "text/plain", strings.Repeat("x", 16*1024), []who{anonymous}, ""},
	{"16KB を超える", "POST", "/api/csp-report", "application/csp-report", strings.Repeat("x", 16*1024+1), []who{anonymous}, ""},
	{"読み取りは無い", "GET", "/api/csp-report", "", ``, []who{anonymous}, ""},

	// 登録の計測。未ログインは 401。形の違う本文は、ログイン済みでも DB に触る前に 415
	{"計測（未ログイン）", "POST", "/api/analytics/registration", "", ``, []who{anonymous, forged, unknownUser}, ""},
	{"計測（形が違う）", "POST", "/api/analytics/registration", "application/xml", `<a/>`, []who{eachUser}, ""},

	// シミュレーション。トークンが無い・違えば 401（本文より先に断る）
	{"sim 状態（トークンなし）", "GET", "/api/sim/state", "", ``, []who{anonymous, eachUser}, ""},
	{"sim 状態（違うトークン）", "GET", "/api/sim/state", "", ``, []who{anonymous}, "bad"},
	{"sim 状態", "GET", "/api/sim/state", "", ``, []who{anonymous}, "ok"},
	{"sim 付与（トークンなし・形が違う）", "POST", "/api/sim/users", "application/xml", `<a/>`, []who{anonymous}, ""},
	{"sim 付与（壊れた JSON）", "POST", "/api/sim/users", "application/json", `{no`, []who{anonymous}, "ok"},
	{"sim 付与（本文なし）", "POST", "/api/sim/users", "", ``, []who{anonymous}, "ok"},
	{"sim 付与（実ユーザー）", "POST", "/api/sim/users", "application/json", `{"email":"a@b.com","seq":1,"cohort":"steady"}`, []who{anonymous}, "ok"},
	{"sim 付与（小数）", "POST", "/api/sim/users", "application/json", `{"email":"delivered+sim1@resend.dev","seq":1.5,"cohort":"steady"}`, []who{anonymous}, "ok"},
	{"sim 付与（知らない型）", "POST", "/api/sim/users", "application/json", `{"email":"delivered+sim1@resend.dev","seq":1,"cohort":"x"}`, []who{anonymous}, "ok"},
	{"sim 付与（居ない）", "POST", "/api/sim/users", "application/json", `{"email":"delivered+sim99999@resend.dev","seq":99999,"cohort":"steady"}`, []who{anonymous}, "ok"},
	{"sim 付与（形が違う）", "POST", "/api/sim/users", "application/xml", `<a/>`, []who{anonymous}, "ok"},
	{"sim 更新（連番が文字）", "PATCH", "/api/sim/users/abc", "application/json", `{}`, []who{anonymous}, "ok"},
	{"sim 更新（連番が0）", "PATCH", "/api/sim/users/0", "application/json", `{}`, []who{anonymous}, "ok"},
	{"sim 更新（空は居なくても 204）", "PATCH", "/api/sim/users/99999", "application/json", `{}`, []who{anonymous}, "ok"},
	{"sim 更新（1e3）", "PATCH", "/api/sim/users/1e3", "application/json", `{}`, []who{anonymous}, "ok"},
	{"sim 更新（居ない）", "PATCH", "/api/sim/users/99999", "application/json", `{"lastActedOn":"2026-09-30"}`, []who{anonymous}, "ok"},
	{"sim 更新（日付の形）", "PATCH", "/api/sim/users/99999", "application/json", `{"lastActedOn":"2026/09/30"}`, []who{anonymous}, "ok"},
	{"sim 更新（null の本文）", "PATCH", "/api/sim/users/99999", "application/json", `null`, []who{anonymous}, "ok"},
	{"sim 更新（配列）", "PATCH", "/api/sim/users/99999", "application/json", `[]`, []who{anonymous}, "ok"},
}

func TestParityWrites(t *testing.T) {
	env := parityEnv(t)
	for _, c := range parityWrites {
		for _, w := range c.as {
			t.Run(fmt.Sprintf("%s %s %s（%s）", c.method, c.path, c.name, whoNames[w]), func(t *testing.T) {
				for i, cookie := range env.cookiesFor(w) {
					pr := parityRequest{method: c.method, path: c.path, cookie: cookie, body: c.body, header: map[string]string{}}
					if c.contentType != "" {
						pr.header["Content-Type"] = c.contentType
					}
					switch c.simToken {
					case "ok":
						pr.header["Authorization"] = "Bearer " + env.simSecret
					case "bad":
						pr.header["Authorization"] = "Bearer not-the-secret"
					}
					if diffs := compareResponses(send(t, env.node, pr), send(t, env.goURL, pr)); len(diffs) > 0 {
						t.Fatalf("%d人目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
					}
				}
			})
		}
	}
}

// TestParityAnalytics は登録の計測を比べる。1回目で「計測済み」の印が付くので、同じ人に Node → Go → Node の順で
// 送り、Go と2回目の Node（どちらも印が付いた後）を比べる。1回目の Node は true でも false でもよい。
func TestParityAnalytics(t *testing.T) {
	env := parityEnv(t)
	for i, cookie := range env.cookiesFor(eachUser) {
		pr := parityRequest{method: "POST", path: "/api/analytics/registration", cookie: cookie}
		first := send(t, env.node, pr)
		if first.status != http.StatusOK {
			t.Fatalf("%d人目: Node の1回目が %d", i+1, first.status)
		}
		gon := send(t, env.goURL, pr)
		node := send(t, env.node, pr)
		if diffs := compareResponses(node, gon); len(diffs) > 0 {
			t.Fatalf("%d人目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
		}
	}
}

// TestParitySimStateful は DB を書き換える sim の API を、元の値のまま書き戻す形で比べる。
// 同じ値で UPDATE しても「当たった」と数えること（Node の mysql2 の FOUND_ROWS。Go の ClientFoundRows）もここで確かめる。
func TestParitySimStateful(t *testing.T) {
	env := parityEnv(t)
	auth := map[string]string{"Authorization": "Bearer " + env.simSecret, "Content-Type": "application/json"}
	state := send(t, env.node, parityRequest{method: "GET", path: "/api/sim/state", header: auth})
	users, _ := state.body.(map[string]any)["users"].([]any)
	if len(users) < 2 {
		t.Skip("シミュレーションの合成ユーザーが2人以上いないので飛ばす（pnpm sim:run で作れる）")
	}
	u0, u1 := users[0].(map[string]any), users[1].(map[string]any)
	asJSON := func(v any) string { b, _ := json.Marshal(v); return string(b) }

	for _, c := range []parityRequest{
		// 今の値をそのまま書き戻す（どちらも 204。値が変わらなくても「見つからない」にならない）
		{method: "PATCH", path: fmt.Sprintf("/api/sim/users/%v", u0["seq"]),
			body: asJSON(map[string]any{"lastActedOn": u0["lastActedOn"], "dormantFrom": u0["dormantFrom"]})},
		// 付与も同じ連番・型で付け直す（updatedAt だけが変わる）
		{method: "POST", path: "/api/sim/users",
			body: asJSON(map[string]any{"email": u0["email"], "seq": u0["seq"], "cohort": u0["cohort"]})},
		// 別の人の連番を付けようとすると 409（UNIQUE）で、何も変わらない
		{method: "POST", path: "/api/sim/users",
			body: asJSON(map[string]any{"email": u0["email"], "seq": u1["seq"], "cohort": u0["cohort"]})},
	} {
		c.header = auth
		t.Run(c.method+" "+c.path+" "+c.body, func(t *testing.T) {
			if diffs := compareResponses(send(t, env.node, c), send(t, env.goURL, c)); len(diffs) > 0 {
				t.Fatalf("食い違い（Node → Go）:\n  %s", strings.Join(diffs, "\n  "))
			}
		})
	}

	// 書き戻した後の状態も、Node と Go で同じ
	after := parityRequest{method: "GET", path: "/api/sim/state", header: auth}
	if diffs := compareResponses(send(t, env.node, after), send(t, env.goURL, after)); len(diffs) > 0 {
		t.Fatalf("状態が食い違う:\n  %s", strings.Join(diffs, "\n  "))
	}
}

// parityEnvironment は parity.sh が渡す接続先と、誰として送るかの Cookie。
type parityEnvironment struct {
	node, goURL, simSecret string
	cookiesFor             func(who) []string
}

func parityEnv(t *testing.T) parityEnvironment {
	t.Helper()
	nodeURL, goURL := os.Getenv("PARITY_NODE_URL"), os.Getenv("PARITY_GO_URL")
	users := strings.Fields(os.Getenv("PARITY_COOKIES"))
	simSecret := os.Getenv("PARITY_SIM_SECRET")
	if nodeURL == "" || goURL == "" || len(users) == 0 || simSecret == "" {
		t.Fatal("parity.sh から動かしてください")
	}
	cookieName, _, _ := strings.Cut(users[0], "=")
	return parityEnvironment{node: nodeURL, goURL: goURL, simSecret: simSecret, cookiesFor: func(w who) []string {
		switch w {
		case forged:
			return []string{cookieName + "=" + sign("forged-token", "not-the-secret")}
		case unknownUser:
			return []string{cookieName + "=" + sign("no-such-token", os.Getenv("BETTER_AUTH_SECRET"))}
		case eachUser:
			return users
		}
		return []string{""}
	}}
}

// 比べるヘッダー。CSP は API 向けに変えているので比べない（middleware.go の securityHeaders）。
var parityHeaders = []string{
	"Content-Type",
	"Strict-Transport-Security",
	"X-Content-Type-Options",
	"X-Frame-Options",
	"Referrer-Policy",
	"Retry-After",
	// 大学の一覧だけが付ける（キャッシュと 304 のため）。ほかのルートはどちらも付けない
	"ETag",
	"Cache-Control",
	"Vary",
	"Content-Encoding",
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
	return send(t, base, parityRequest{method: method, path: path, cookie: cookie})
}

// parityRequest は本文やヘッダーの付いたリクエスト（書き込みの比較に使う）。
type parityRequest struct {
	method, path, cookie string
	header               map[string]string
	body                 string
}

func send(t *testing.T, base string, pr parityRequest) response {
	t.Helper()
	method, path := pr.method, pr.path
	req, err := http.NewRequest(method, base+path, strings.NewReader(pr.body))
	if err != nil {
		t.Fatal(err)
	}
	if pr.body == "" {
		req.Body, req.ContentLength = http.NoBody, 0
	}
	if pr.cookie != "" {
		req.Header.Set("Cookie", pr.cookie)
	}
	for k, v := range pr.header {
		req.Header.Set(k, v)
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
	// 204 など本文の無い応答は nil のまま比べる。
	var body any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("%s %s%s の本文が JSON でない: %q", method, base, path, raw)
		}
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

// TestParityUniversitiesConditional は、大学の一覧の 304 と圧縮を比べる。
// parityCases は Accept-Encoding: identity で本文を比べるので、ここだけ別に見る。
func TestParityUniversitiesConditional(t *testing.T) {
	nodeURL, goURL := os.Getenv("PARITY_NODE_URL"), os.Getenv("PARITY_GO_URL")
	users := strings.Fields(os.Getenv("PARITY_COOKIES"))
	if nodeURL == "" || goURL == "" || len(users) == 0 {
		t.Fatal("parity.sh から動かしてください")
	}
	get := func(base string, header map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", base+"/api/universities", nil)
		req.Header.Set("Cookie", users[0])
		for k, v := range header {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultTransport.RoundTrip(req) // 自動の解凍をさせない
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}

	// JSON のバイト列が同じなので、ETag も同じになる（振り分けを行き来しても 304 が効く）
	etag := get(nodeURL, map[string]string{"Accept-Encoding": "identity"}).Header.Get("ETag")
	if goETag := get(goURL, map[string]string{"Accept-Encoding": "identity"}).Header.Get("ETag"); goETag != etag {
		t.Fatalf("ETag: Node %q, Go %q", etag, goETag)
	}

	for _, inm := range []string{etag, "W/" + etag, `"other", W/` + etag} {
		for name, base := range map[string]string{"Node": nodeURL, "Go": goURL} {
			res := get(base, map[string]string{"If-None-Match": inm})
			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != http.StatusNotModified || len(body) != 0 {
				t.Errorf("%s If-None-Match %s: status %d, 本文 %d バイト", name, inm, res.StatusCode, len(body))
			}
		}
	}

	// gzip を受け付けるなら、どちらも圧縮済みを返し、解凍すると同じ JSON
	var plain [2][]byte
	for i, base := range []string{nodeURL, goURL} {
		res := get(base, map[string]string{"Accept-Encoding": "gzip"})
		if res.Header.Get("Content-Encoding") != "gzip" {
			t.Fatalf("%s: Content-Encoding = %q", base, res.Header.Get("Content-Encoding"))
		}
		zr, err := gzip.NewReader(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		plain[i], _ = io.ReadAll(zr)
	}
	if !bytes.Equal(plain[0], plain[1]) {
		t.Errorf("解凍した本文が違う（Node %d バイト、Go %d バイト）", len(plain[0]), len(plain[1]))
	}
}
