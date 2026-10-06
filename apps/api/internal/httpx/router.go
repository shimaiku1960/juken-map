package httpx

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// 全ルートに「誰が呼んでよいか（入口の種類）」を持たせ、認証・管理者・デモの拒否を
// ルーターがまとめて行う。Node の access-control.ts（JUK-66）と同じ考え方。
//
// Node は config.access の付け忘れを起動時に見つけて落としていた。Go では登録の関数
// そのものを入口の種類ごとに分けたので、種類を選ばずに登録する書き方がそもそも無い。
// さらに、user・admin のハンドラはセッションを引数で受け取るので、「ログイン必須の
// ルートなのにセッションが無い」という取り違えはコンパイルの時点で起きない。

// Access は入口の種類。値は Node の ACCESS_ENTRY と同じ名前にする。
type Access string

const (
	AccessPublic  Access = "public"  // E1 誰でも呼べる読み取り
	AccessUser    Access = "user"    // E3 ログイン必須。デモは書き込み不可。持ち主の確認はハンドラが行う
	AccessAdmin   Access = "admin"   // E4 ログイン＋role=admin
	AccessWebhook Access = "webhook" // E5 外のサービスが呼ぶ（LINE・microCMS）。署名はハンドラが本文と一緒に確かめる
	AccessJob     Access = "job"     // E6 自分のジョブ（cron・sim）。セッションではなく共有トークンで守る
	// E7 ログイン不要の書き込み（CSP 違反の報告）。書き込めても害が無い設計にする（DB に書かない・大きさに上限）
	AccessAnonymousWrite Access = "anonymous-write"
	AccessOAuth          Access = "oauth" // E8 外部との連携（OAuth）。画面遷移なので、未ログインはハンドラがログインへ送る
	// E2 認証の入口（/api/auth/*：ログイン・登録・再設定・2段階認証・外部ログイン）。ログインしていなくても呼べる。
	// 書き込みは別のサイトから断る（ログインの CSRF。10 D2）。セッションがあれば読んで渡す（無ければ nil）。
	AccessAuth Access = "auth"
)

// デモアカウント（面接官向け・閲覧専用）。Node の src/shared/demo.ts と同じ値。
const DemoEmail = "demo@juken-map.com"

// 管理者だが2段階認証を通していないときの 403 の印。Node の src/shared/admin.ts と同じ値。
const TwoFactorRequired = "TWO_FACTOR_REQUIRED"

// Session はログイン中の利用者。internal/feature/auth/session.go が Cookie のトークンで DB（AuthSession）から読む。
type Session struct {
	// ID はセッションの番号（トークンではない）。ログアウトや「ほかの端末を消す」で、このセッションを指す。
	ID     string
	UserID string
	Email  string
	Role   string
	Banned bool
	// TwoFactorVerified は、そのセッションが2段階認証を通して作られたか（AuthSession.mfaVerifiedAt、G3）。
	// 管理者のルートはこれを求める。
	TwoFactorVerified bool
}

// SessionHandler はログイン済みのルートのハンドラ。Node の currentSession(request) にあたる値を引数で受け取る。
type SessionHandler func(w http.ResponseWriter, r *http.Request, s *Session)

// OAuthHandler は OAuth の入口のハンドラ。s は未ログインなら nil（ハンドラが戻り先つきでログインへ送る）。
type OAuthHandler func(w http.ResponseWriter, r *http.Request, s *Session)

// AuthHandler は認証の入口のハンドラ。s は未ログインなら nil。停止中・デモの扱いはハンドラが決める
// （停止中でもログアウトはできる、など入口ごとに違うため）。
type AuthHandler func(w http.ResponseWriter, r *http.Request, s *Session)

// SessionLoader はリクエストからセッションを読む。ログインしていなければ (nil, nil) を返す。
// 関数で受け取るのは、テストで DB の代わりに決まったセッションを返せるようにするため。
type SessionLoader func(r *http.Request) (*Session, error)

// RouteEntry は登録済みのルート1本。一覧は、ルートと入口の種類を突き合わせるテストに使う
// （Node の JUK-67 と同じことを Go でもできるように持っておく）。
type RouteEntry struct {
	Pattern string
	Access  Access
}

type Router struct {
	mux         *http.ServeMux
	loadSession SessionLoader
	Routes      []RouteEntry
	crossOrigin *http.CrossOriginProtection
	rateLimiter *userRateLimiter
}

// CrossOriginMessage は、別のサイトから送られた書き込みを断るときの文言。
const CrossOriginMessage = "別のサイトからの書き込みは受け付けません"

func NewRouter(load SessionLoader) *Router {
	// crossOrigin は Cookie で認証する書き込みの CSRF 対策（標準の net/http）。ブラウザが付ける
	// Sec-Fetch-Site（無ければ Origin と Host の比較）で、別のサイトから送られた POST・PUT・PATCH・DELETE を断る。
	// 本番の nginx は Host をそのまま渡すので、https://juken-map.com の画面からの書き込みは同じサイトとして通る。
	// どちらのヘッダーも無いリクエスト（curl・応答一致テスト）はブラウザではないので通す。
	//
	// Node の自前 API はオリジンを確かめず、SameSite=Lax の Cookie だけで守っている。Go ではこれを足して一段強くする。
	rt := &Router{
		mux:         http.NewServeMux(),
		loadSession: load,
		crossOrigin: http.NewCrossOriginProtection(),
		rateLimiter: newUserRateLimiter(time.Now),
	}
	// どのルートにも当たらないものは 404。ServeMux の既定はテキストの「404 page not found」で、
	// JSON を読むつもりの画面が壊れるので、Node と同じ形のエラーにする。
	// メソッド違い（POST /api/dashboard など）もここに来る。ServeMux は当たるルートが1本も
	// 無いときだけ 405 を返すので、全メソッドに当たるこの "/" があると 405 にはならず、
	// Node（Fastify）と同じ 404 になる。
	rt.mux.HandleFunc("/", NotFound)
	return rt
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.mux.ServeHTTP(w, r)
}

// Public は誰でも呼べるルートを登録する。
func (rt *Router) Public(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, AccessPublic, h)
}

// PublicWithSession は誰でも開ける GET のルートを登録する。ログインしていればセッションを渡す（無ければ nil）。
// ログイン状態で行き先を変えるだけの入口（/line/settings）に使う。断るかどうかはハンドラが決める。
func (rt *Router) PublicWithSession(pattern string, h AuthHandler) {
	rt.handle(pattern, AccessPublic, func(w http.ResponseWriter, r *http.Request) {
		s, err := rt.loadSession(r)
		if err != nil {
			InternalError(w, r, err)
			return
		}
		h(w, r, s)
	})
}

// AnonymousWrite はログインせずに書き込めるルートを登録する。セッションは読まない。
// 誰が送ってきても困らないことは、ハンドラの側で保つ（DB に書かない・読む大きさに上限を置く）。
// Cookie で認証しないので、別のサイトからの書き込みも断らない（CSP の報告はブラウザが送る）。
func (rt *Router) AnonymousWrite(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, AccessAnonymousWrite, h)
}

// User はログイン必須のルートを登録する。別のサイトからの書き込みは 403、未ログインは 401、停止中は 403、
// デモアカウントの書き込み（GET・HEAD 以外）は 403、利用者の回数制限を超えたら 429 で、ハンドラまで来ない。
func (rt *Router) User(pattern string, h SessionHandler) {
	rt.handle(pattern, AccessUser, func(w http.ResponseWriter, r *http.Request) {
		if !rt.sameOrigin(w, r) {
			return
		}
		s, ok := rt.requireSession(w, r)
		if !ok {
			return
		}
		if isWrite(r) && s.Email == DemoEmail {
			WriteError(w, http.StatusForbidden, "デモアカウントは閲覧専用です")
			return
		}
		if !rt.limitUser(w, r, s) {
			return
		}
		h(w, r, s)
	})
}

// Admin は管理者だけのルートを登録する。別のサイトからの書き込みと、管理者でなければ 403。
// 回数制限は user と同じ札を使う（管理者も1人の利用者として数える）。
func (rt *Router) Admin(pattern string, h SessionHandler) {
	rt.handle(pattern, AccessAdmin, func(w http.ResponseWriter, r *http.Request) {
		if !rt.sameOrigin(w, r) {
			return
		}
		s, ok := rt.requireSession(w, r)
		if !ok {
			return
		}
		// 画面もメニューの出し分けに role を使うが、守るのはここ。
		if s.Role != "admin" {
			WriteError(w, http.StatusForbidden, "Forbidden")
			return
		}
		// Node の requireAdmin と同じく、2段階認証を通したセッションだけを通す。
		if !s.TwoFactorVerified {
			WriteJSON(w, http.StatusForbidden, map[string]string{
				"error": "管理画面を開くには、2段階認証を通してログインしてください。",
				"code":  TwoFactorRequired,
			})
			return
		}
		if !rt.limitUser(w, r, s) {
			return
		}
		h(w, r, s)
	})
}

// Job は GitHub Actions などが共有トークンで呼ぶルートを登録する（cron と sim はトークンが別）。
// `Authorization: Bearer <secret>` が合わなければ 401 で、ハンドラまで来ない。
// secret が空なら何が送られても 401（設定し忘れで誰でも呼べる状態にしない）。
// Node はトークンをハンドラの中で確かめていたが、Go では登録の時点で必ず付くようにした。
func (rt *Router) Job(pattern, secret string, h http.HandlerFunc) {
	rt.handle(pattern, AccessJob, func(w http.ResponseWriter, r *http.Request) {
		if !hasBearerToken(r, secret) {
			WriteError(w, http.StatusUnauthorized, "Unauthorized")
			return
		}
		h(w, r)
	})
}

// Webhook は外のサービスが呼ぶルートを登録する（LINE・microCMS の Webhook）。
// 署名は受け取ったままの本文で確かめる必要があり、本文を読むのはハンドラなので、確認もハンドラが行う。
// ルーターは何も断らない。種類を分けて登録するのは、一覧（TestRegisteredRoutes）で入口を取り違えないため。
func (rt *Router) Webhook(pattern string, h http.HandlerFunc) {
	rt.handle(pattern, AccessWebhook, h)
}

// OAuth は外部との連携の往復（LINE Login）のルートを登録する。ブラウザが画面遷移で開くので、
// 未ログインでも 401 にはせず、s を nil にしてハンドラへ渡す（ハンドラがログインへ 302 で送る）。
// ログイン済みなら、停止中とデモは 403（連携は書き込みなので、GET でもデモは断る。Node と同じ）。
func (rt *Router) OAuth(pattern string, h OAuthHandler) {
	rt.handle(pattern, AccessOAuth, func(w http.ResponseWriter, r *http.Request) {
		s, err := rt.loadSession(r)
		if err != nil {
			InternalError(w, r, err)
			return
		}
		if s != nil {
			// Node の oauth の入口は停止中を見ていない（停止のときに session を消すので、普通は来ない）。
			// user・admin の入口と揃えて、残っていたセッションでも連携させない。
			if s.Banned {
				WriteError(w, http.StatusForbidden, "このアカウントは利用を停止されています。")
				return
			}
			if s.Email == DemoEmail {
				WriteError(w, http.StatusForbidden, "デモアカウントは閲覧専用です")
				return
			}
		}
		h(w, r, s)
	})
}

// Auth は認証の入口を登録する。別のサイトからの書き込みは 403（ログイン・登録・ログアウト・2段階認証の確認も。
// 攻撃者のアカウントで被害者をログインさせる「ログインの CSRF」を防ぐ。10 D2）。未ログインでも断らない。
// 状態を変える処理は GET で受けない（GET は外部ログインの戻りとセッションの取得だけ）。
func (rt *Router) Auth(pattern string, h AuthHandler) {
	rt.handle(pattern, AccessAuth, func(w http.ResponseWriter, r *http.Request) {
		if !rt.sameOrigin(w, r) {
			return
		}
		s, err := rt.loadSession(r)
		if err != nil {
			InternalError(w, r, err)
			return
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
func (rt *Router) sameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if err := rt.crossOrigin.Check(r); err != nil {
		WriteError(w, http.StatusForbidden, CrossOriginMessage)
		return false
	}
	return true
}

// requireSession は Node の requireSession（context.ts）と同じ判定・同じ文言。
func (rt *Router) requireSession(w http.ResponseWriter, r *http.Request) (*Session, bool) {
	s, err := rt.loadSession(r)
	if err != nil {
		InternalError(w, r, err)
		return nil, false
	}
	if s == nil {
		WriteError(w, http.StatusUnauthorized, "Unauthorized")
		return nil, false
	}
	// 停止のときにその人の session は消しているので普通はここに来ないが、
	// 消す直前に始まっていたリクエストが残ることはある。
	if s.Banned {
		WriteError(w, http.StatusForbidden, "このアカウントは利用を停止されています。")
		return nil, false
	}
	return s, true
}

// handle はルートを登録する。pattern は "GET /api/study-logs/{id}" のようにメソッドから書く。
func (rt *Router) handle(pattern string, a Access, h http.HandlerFunc) {
	method, path, ok := strings.Cut(pattern, " ")
	// メソッドを省くと全メソッドに当たる。GET のつもりのルートが POST も受けると、
	// 「書き込みかどうか」で決めるデモの拒否がずれるので、起動の時点で落とす。
	if !ok || method == "" || !strings.HasPrefix(path, "/") {
		panic(fmt.Sprintf("ルート %q は「メソッド パス」の形で書いてください", pattern))
	}
	label := routeLabel(path)
	rt.Routes = append(rt.Routes, RouteEntry{Pattern: pattern, Access: a})
	rt.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		// 外側の observe がメトリクスの route ラベルに使う。
		if info := telemetry.RequestInfoFrom(r.Context()); info != nil {
			info.Route = label
		}
		h(w, r)
	})
}

func isWrite(r *http.Request) bool {
	return r.Method != http.MethodGet && r.Method != http.MethodHead
}

var PathParam = regexp.MustCompile(`\{([^}.]+)(?:\.\.\.)?\}`)

// routeLabel は Go の書き方（/api/study-logs/{id}）を Fastify の書き方（/api/study-logs/:id）に直す。
// メトリクスの route ラベルを Node と揃え、Grafana で同じルートとして並べられるようにする。
func routeLabel(path string) string {
	return PathParam.ReplaceAllString(path, ":$1")
}
