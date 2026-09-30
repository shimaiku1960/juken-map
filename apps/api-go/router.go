package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// 全ルートに「誰が呼んでよいか（入口の種類）」を持たせ、認証・管理者・デモの拒否を
// ルーターがまとめて行う。Node の access-control.ts（JUK-66）と同じ考え方。
//
// Node は config.access の付け忘れを起動時に見つけて落としていた。Go では登録の関数
// そのものを入口の種類ごとに分けたので、種類を選ばずに登録する書き方がそもそも無い。
// さらに、user・admin のハンドラはセッションを引数で受け取るので、「ログイン必須の
// ルートなのにセッションが無い」という取り違えはコンパイルの時点で起きない。

// access は入口の種類。値は Node の ACCESS_ENTRY と同じ名前にする。
// Webhook・OAuth などは、その API を Go へ移すときに足す（JUK-79）。
type access string

const (
	accessPublic access = "public" // E1 誰でも呼べる読み取り
	accessUser   access = "user"   // E3 ログイン必須。デモは書き込み不可。持ち主の確認はハンドラが行う
	accessAdmin  access = "admin"  // E4 ログイン＋role=admin
	accessJob    access = "job"    // E6 自分のジョブ（cron・sim）。セッションではなく共有トークンで守る
	// E7 ログイン不要の書き込み（CSP 違反の報告）。書き込めても害が無い設計にする（DB に書かない・大きさに上限）
	accessAnonymousWrite access = "anonymous-write"
)

// デモアカウント（面接官向け・閲覧専用）。Node の src/shared/demo.ts と同じ値。
const demoEmail = "demo@juken-map.com"

// 管理者だが2段階認証を通していないときの 403 の印。Node の src/shared/admin.ts と同じ値。
const twoFactorRequired = "TWO_FACTOR_REQUIRED"

// session はログイン中の利用者。Better Auth が Node 側で発行したものを、auth.go が DB から読む。
type session struct {
	UserID string
	Email  string
	Role   string
	Banned bool
	// TwoFactorVerified は、そのセッションが2段階認証を通して作られたか。
	// Node の auth.ts が session.twoFactorVerified に付ける。管理者のルートはこれを求める。
	TwoFactorVerified bool
}

// sessionHandler はログイン済みのルートのハンドラ。Node の currentSession(request) にあたる値を引数で受け取る。
type sessionHandler func(w http.ResponseWriter, r *http.Request, s *session)

// sessionLoader はリクエストからセッションを読む。ログインしていなければ (nil, nil) を返す。
// 関数で受け取るのは、テストで DB の代わりに決まったセッションを返せるようにするため。
type sessionLoader func(r *http.Request) (*session, error)

// routeEntry は登録済みのルート1本。一覧は、ルートと入口の種類を突き合わせるテストに使う
// （Node の JUK-67 と同じことを Go でもできるように持っておく）。
type routeEntry struct {
	Pattern string
	Access  access
}

type router struct {
	mux         *http.ServeMux
	loadSession sessionLoader
	routes      []routeEntry
}

func newRouter(load sessionLoader) *router {
	rt := &router{mux: http.NewServeMux(), loadSession: load}
	// どのルートにも当たらないものは 404。ServeMux の既定はテキストの「404 page not found」で、
	// JSON を読むつもりの画面が壊れるので、Node と同じ形のエラーにする。
	// メソッド違い（POST /api/dashboard など）もここに来る。ServeMux は当たるルートが1本も
	// 無いときだけ 405 を返すので、全メソッドに当たるこの "/" があると 405 にはならず、
	// Node（Fastify）と同じ 404 になる。
	rt.mux.HandleFunc("/", notFound)
	return rt
}

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mux.ServeHTTP(w, r)
}

// public は誰でも呼べるルートを登録する。
func (rt *router) public(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, accessPublic, h)
}

// anonymousWrite はログインせずに書き込めるルートを登録する。セッションは読まない。
// 誰が送ってきても困らないことは、ハンドラの側で保つ（DB に書かない・読む大きさに上限を置く）。
func (rt *router) anonymousWrite(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, accessAnonymousWrite, h)
}

// user はログイン必須のルートを登録する。未ログインは 401、停止中は 403、
// デモアカウントの書き込み（GET・HEAD 以外）は 403 で、ハンドラまで来ない。
func (rt *router) user(pattern string, h sessionHandler) {
	rt.handle(pattern, accessUser, func(w http.ResponseWriter, r *http.Request) {
		s, ok := rt.requireSession(w, r)
		if !ok {
			return
		}
		if isWrite(r) && s.Email == demoEmail {
			writeError(w, http.StatusForbidden, "デモアカウントは閲覧専用です")
			return
		}
		h(w, r, s)
	})
}

// admin は管理者だけのルートを登録する。管理者でなければ 403。
func (rt *router) admin(pattern string, h sessionHandler) {
	rt.handle(pattern, accessAdmin, func(w http.ResponseWriter, r *http.Request) {
		s, ok := rt.requireSession(w, r)
		if !ok {
			return
		}
		// 画面もメニューの出し分けに role を使うが、守るのはここ。
		if s.Role != "admin" {
			writeError(w, http.StatusForbidden, "Forbidden")
			return
		}
		// Node の requireAdmin と同じく、2段階認証を通したセッションだけを通す。
		if !s.TwoFactorVerified {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "管理画面を開くには、2段階認証を通してログインしてください。",
				"code":  twoFactorRequired,
			})
			return
		}
		h(w, r, s)
	})
}

// job は GitHub Actions などが共有トークンで呼ぶルートを登録する（cron と sim はトークンが別）。
// `Authorization: Bearer <secret>` が合わなければ 401 で、ハンドラまで来ない。
// secret が空なら何が送られても 401（設定し忘れで誰でも呼べる状態にしない）。
// Node はトークンをハンドラの中で確かめていたが、Go では登録の時点で必ず付くようにした。
func (rt *router) job(pattern, secret string, h http.HandlerFunc) {
	rt.handle(pattern, accessJob, func(w http.ResponseWriter, r *http.Request) {
		if !hasBearerToken(r, secret) {
			writeError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		h(w, r)
	})
}

// hasBearerToken は Node の bearer-token.ts と同じ比べ方。文字列を == で比べると、先頭から
// 何文字一致したかで返るまでの時間が変わり、外から1文字ずつ当てられる余地が残る。両方を SHA-256 に
// そろえてから一定時間で比べる（ハッシュにするのは、長さの違いで先に返して秘密値の長さを漏らさないため）。
func hasBearerToken(r *http.Request, secret string) bool {
	if secret == "" {
		return false
	}
	got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
	want := sha256.Sum256([]byte("Bearer " + secret))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// requireSession は Node の requireSession（context.ts）と同じ判定・同じ文言。
func (rt *router) requireSession(w http.ResponseWriter, r *http.Request) (*session, bool) {
	s, err := rt.loadSession(r)
	if err != nil {
		internalError(w, r, err)
		return nil, false
	}
	if s == nil {
		writeError(w, http.StatusUnauthorized, "Unauthorized")
		return nil, false
	}
	// 停止のときにその人の session は消しているので普通はここに来ないが、
	// 消す直前に始まっていたリクエストが残ることはある。
	if s.Banned {
		writeError(w, http.StatusForbidden, "このアカウントは利用を停止されています。")
		return nil, false
	}
	return s, true
}

// handle はルートを登録する。pattern は "GET /api/study-logs/{id}" のようにメソッドから書く。
func (rt *router) handle(pattern string, a access, h http.HandlerFunc) {
	method, path, ok := strings.Cut(pattern, " ")
	// メソッドを省くと全メソッドに当たる。GET のつもりのルートが POST も受けると、
	// 「書き込みかどうか」で決めるデモの拒否がずれるので、起動の時点で落とす。
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("ルート %q は「メソッド パス」の形で書いてください", pattern))
	}
	label := routeLabel(path)
	rt.routes = append(rt.routes, routeEntry{Pattern: pattern, Access: a})
	rt.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		// 外側の observe がメトリクスの route ラベルに使う。
		if info := requestInfoFrom(r.Context()); info != nil {
			info.route = label
		}
		h(w, r)
	})
}

func isWrite(r *http.Request) bool {
	return r.Method != http.MethodGet && r.Method != http.MethodHead
}

var pathParam = regexp.MustCompile(`\{([^}.]+)(?:\.\.\.)?\}`)

// routeLabel は Go の書き方（/api/study-logs/{id}）を Fastify の書き方（/api/study-logs/:id）に直す。
// メトリクスの route ラベルを Node と揃え、Grafana で同じルートとして並べられるようにする。
func routeLabel(path string) string {
	return pathParam.ReplaceAllString(path, ":$1")
}
