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
type access string

const (
	accessPublic  access = "public"  // E1 誰でも呼べる読み取り
	accessUser    access = "user"    // E3 ログイン必須。デモは書き込み不可。持ち主の確認はハンドラが行う
	accessAdmin   access = "admin"   // E4 ログイン＋role=admin
	accessWebhook access = "webhook" // E5 外のサービスが呼ぶ（LINE・microCMS）。署名はハンドラが本文と一緒に確かめる
	accessJob     access = "job"     // E6 自分のジョブ（cron・sim）。セッションではなく共有トークンで守る
	// E7 ログイン不要の書き込み（CSP 違反の報告）。書き込めても害が無い設計にする（DB に書かない・大きさに上限）
	accessAnonymousWrite access = "anonymous-write"
	accessOAuth          access = "oauth" // E8 外部との連携（OAuth）。画面遷移なので、未ログインはハンドラがログインへ送る
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

// oauthHandler は OAuth の入口のハンドラ。s は未ログインなら nil（ハンドラが戻り先つきでログインへ送る）。
type oauthHandler func(w http.ResponseWriter, r *http.Request, s *session)

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
	crossOrigin *http.CrossOriginProtection
}

// crossOriginMessage は、別のサイトから送られた書き込みを断るときの文言。
const crossOriginMessage = "別のサイトからの書き込みは受け付けません"

func newRouter(load sessionLoader) *router {
	// crossOrigin は Cookie で認証する書き込みの CSRF 対策（標準の net/http）。ブラウザが付ける
	// Sec-Fetch-Site（無ければ Origin と Host の比較）で、別のサイトから送られた POST・PUT・PATCH・DELETE を断る。
	// 本番の nginx は Host をそのまま渡すので、https://juken-map.com の画面からの書き込みは同じサイトとして通る。
	// どちらのヘッダーも無いリクエスト（curl・応答一致テスト）はブラウザではないので通す。
	//
	// Node の自前 API はオリジンを確かめず、SameSite=Lax の Cookie だけで守っている。Go ではこれを足して一段強くする。
	rt := &router{mux: http.NewServeMux(), loadSession: load, crossOrigin: http.NewCrossOriginProtection()}
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
// Cookie で認証しないので、別のサイトからの書き込みも断らない（CSP の報告はブラウザが送る）。
func (rt *router) anonymousWrite(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, accessAnonymousWrite, h)
}

// user はログイン必須のルートを登録する。別のサイトからの書き込みは 403、未ログインは 401、停止中は 403、
// デモアカウントの書き込み（GET・HEAD 以外）は 403 で、ハンドラまで来ない。
func (rt *router) user(pattern string, h sessionHandler) {
	rt.handle(pattern, accessUser, func(w http.ResponseWriter, r *http.Request) {
		if !rt.sameOrigin(w, r) {
			return
		}
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

// admin は管理者だけのルートを登録する。別のサイトからの書き込みと、管理者でなければ 403。
func (rt *router) admin(pattern string, h sessionHandler) {
	rt.handle(pattern, accessAdmin, func(w http.ResponseWriter, r *http.Request) {
		if !rt.sameOrigin(w, r) {
			return
		}
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

// webhook は外のサービスが呼ぶルートを登録する（LINE・microCMS の Webhook）。
// 署名は受け取ったままの本文で確かめる必要があり、本文を読むのはハンドラなので、確認もハンドラが行う。
// ルーターは何も断らない。種類を分けて登録するのは、一覧（TestRegisteredRoutes）で入口を取り違えないため。
func (rt *router) webhook(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, accessWebhook, h)
}

// oauth は外部との連携の往復（LINE Login）のルートを登録する。ブラウザが画面遷移で開くので、
// 未ログインでも 401 にはせず、s を nil にしてハンドラへ渡す（ハンドラがログインへ 302 で送る）。
// ログイン済みなら、停止中とデモは 403（連携は書き込みなので、GET でもデモは断る。Node と同じ）。
func (rt *router) oauth(pattern string, h oauthHandler) {
	rt.handle(pattern, accessOAuth, func(w http.ResponseWriter, r *http.Request) {
		s, err := rt.loadSession(r)
		if err != nil {
			internalError(w, r, err)
			return
		}
		if s != nil {
			// Node の oauth の入口は停止中を見ていない（停止のときに session を消すので、普通は来ない）。
			// user・admin の入口と揃えて、残っていたセッションでも連携させない。
			if s.Banned {
				writeError(w, http.StatusForbidden, "このアカウントは利用を停止されています。")
				return
			}
			if s.Email == demoEmail {
				writeError(w, http.StatusForbidden, "デモアカウントは閲覧専用です")
				return
			}
		}
		h(w, r, s)
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

// sameOrigin は、別のサイトから送られた書き込みなら 403 を送って false を返す。GET・HEAD・OPTIONS は常に通す。
func (rt *router) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if err := rt.crossOrigin.Check(r); err != nil {
		writeError(w, http.StatusForbidden, crossOriginMessage)
		return false
	}
	return true
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
