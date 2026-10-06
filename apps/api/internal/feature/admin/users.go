// Package admin は管理画面の入口（/api/admin/*。利用者の管理とマスター編集）。
// ハンドラと読み取りの SQL を持ち、書き込みは internal/write の持ち主（account・university・textbookmaster）に任せる（JUK-156）。
// 全ルートを通す DB のテスト（admin_db_test.go）は、ほかの入口のキャッシュを捨てられているかも見るので、ルートを組む側に置く。
package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

// 管理者ページ（/admin）の利用者の管理（JUK-78）。Node の routes/admin.ts と services/admin-service.ts にあたる。
// どれも rt.Admin（管理者＋2段階認証を通したセッションだけ）で登録する。守るのはルーターで、
// 画面がメニューを出し分けているのは見た目のためだけ。
//
// 停止：bannedAt を書き、その人のセッション（AuthSession）を消して今の画面を落とす。次のログインは、
// ログインの入口（internal/feature/auth/handlers.go など）が bannedAt を見て断る。

const (
	adminUsersPageSize = 50
	// adminUsersMaxPage は Node の listUsersQuerySchema の page の上限。
	adminUsersMaxPage = 10_000
	// adminUserIDMax は Node の userIdParamsSchema の上限（user.id は VARCHAR(191)）。
	adminUserIDMax = 191
	// seedEmailLike は手元の負荷検証用 seed の合成ユーザーのメールの印（Node の src/shared/synthetic.ts）。
	seedEmailLike = "%@synthetic.juken-map.invalid"
)

// userKinds は種別の並び。概要ではこの順に全部並べる（Node の USER_KINDS）。
var userKinds = []apischema.UserKind{apischema.UserKindReal, apischema.UserKindSim, apischema.UserKindSeed, apischema.UserKindDemo}

// kindSQL は利用者の種別を SQL の中で決める。判定の順番に意味がある：sim は simSeq で、
// seed はメールの印で、デモは固定アドレスで見分ける。? を含むので、使うたびに kindParams を同じ位置に並べる。
const kindSQL = `CASE
  WHEN u.simSeq IS NOT NULL THEN 'sim'
  WHEN u.email LIKE ? THEN 'seed'
  WHEN u.email = ? THEN 'demo'
  ELSE 'real'
END`

var kindParams = []any{seedEmailLike, httpx.DemoEmail}

// protectedReason は、相手はいるが操作できない理由。
type protectedReason string

const (
	protectedSelf  protectedReason = "self"
	protectedAdmin protectedReason = "admin"
	protectedDemo  protectedReason = "demo"
	// protectedNoEmail は削除だけの守り。確認のメールアドレスを突き合わせられないので消さない（JUK-89）。
	protectedNoEmail protectedReason = "no_email"
)

var protectedMessages = map[protectedReason]string{
	protectedSelf:    "自分自身は停止・削除できません",
	protectedAdmin:   "他の管理者は停止・削除できません（先に権限を外してください）",
	protectedDemo:    "デモアカウントは停止・削除できません",
	protectedNoEmail: "メールアドレスの無い利用者は、本人の確認ができないため削除できません",
}

// adminTarget は停止・削除の相手。
type adminTarget struct {
	ID       string
	Email    *string
	Role     string
	BannedAt *string // ISO 文字列。null なら止まっていない
}

// protection は守られている相手なら理由を返す。守りは3つで、Node の findTarget と同じ順に見る。
//   - 自分自身：最後の管理者が自分を締め出して誰も入れなくなるのを防ぐ
//   - 他の管理者：管理者どうしで潰し合えないようにする（付け替えは pnpm admin:grant だけ）
//   - デモ：面接官向けの共有アカウント。消えると /login のデモボタンが動かなくなる
func (t *adminTarget) protection(actorID string) (protectedReason, bool) {
	switch {
	case t.ID == actorID:
		return protectedSelf, true
	case t.Role == "admin":
		return protectedAdmin, true
	case t.Email != nil && *t.Email == httpx.DemoEmail:
		return protectedDemo, true
	}
	return "", false
}

// adminUserStore は DB の読み書き。テストでは偽物を渡す（Go の CI には DB が無い）。
type adminUserStore interface {
	overview(ctx context.Context, now time.Time) (apischema.AdminOverview, error)
	listUsers(ctx context.Context, kind apischema.UserKind, q string, page int) (apischema.AdminUserList, error)
	// findTarget は相手を引く。いなければ nil。
	findTarget(ctx context.Context, id string) (*adminTarget, error)
	// Ban は bannedAt を書き（すでに止まっていれば最初の日時のまま）、その人の session を消す（account.Suspend）。
	// 相手がいなければ account.ErrNotFound。
	ban(ctx context.Context, id string, now time.Time) (account.Suspension, error)
	unban(ctx context.Context, id string, now time.Time) error
	// DeleteUser は利用者を消し、一緒に消える行の数を返す（数えるのは記録のためだけ）。
	deleteUser(ctx context.Context, id string) (removedCounts, error)
}

// removedCounts は削除で一緒に消えた行数。生成した AdminDeleteResult.Removed と同じ形。
type removedCounts = struct {
	FinalGoals int `json:"finalGoals"`
	StudyLogs  int `json:"studyLogs"`
	StudyPlans int `json:"studyPlans"`
	Textbooks  int `json:"textbooks"`
}

type UserHandlers struct {
	store adminUserStore
	now   func() time.Time
}

func NewUserHandlers(db *sql.DB) *UserHandlers {
	return &UserHandlers{store: &sqlAdminUserStore{db: db}, now: time.Now}
}

// Overview は GET /api/admin/overview。
func (h *UserHandlers) Overview(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	o, err := h.store.overview(r.Context(), h.now())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin overview: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, o)
}

// ListUsers は GET /api/admin/users。
func (h *UserHandlers) ListUsers(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	kind, q, page, issue := readAdminUsersQuery(httpx.ParseQuery(r.URL.RawQuery))
	if issue != nil {
		issue.Write(w)
		return
	}
	list, err := h.store.listUsers(r.Context(), kind, q, page)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin users: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

// Ban は POST /api/admin/users/{id}/ban。
func (h *UserHandlers) Ban(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	// 本文は使わないが、Node（Fastify）はハンドラより先に本文を読むので、受け付けない形なら同じく 415・413 にする。
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	target, ok := h.operableTarget(w, r, id, s.UserID)
	if !ok {
		return
	}

	// 押し直しても最初に止めた日時を保つ（Node の COALESCE と同じ。account.Suspend が DB の値を返す）。
	banned, err := h.store.ban(r.Context(), id, dates.NowMillis())
	if errors.Is(err, account.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin ban: %w", err))
		return
	}
	removed := int(banned.SessionsRemoved)
	logAdminUserAction(r.Context(), s.UserID, "ban", target, "sessionsRemoved", removed)
	httpx.WriteJSON(w, http.StatusOK, apischema.AdminBanResult{ID: id, Email: target.Email, BannedAt: banned.BannedAt, SessionsRemoved: removed})
}

// Unban は POST /api/admin/users/{id}/unban。守りは見ない（Node と同じ。止まっていなければ何も変わらない）。
func (h *UserHandlers) Unban(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	target, err := h.store.findTarget(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin unban: %w", err))
		return
	}
	if target == nil {
		httpx.WriteError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return
	}
	err = h.store.unban(r.Context(), id, dates.NowMillis())
	if errors.Is(err, account.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin unban: %w", err))
		return
	}
	logAdminUserAction(r.Context(), s.UserID, "unban", target)
	httpx.WriteJSON(w, http.StatusOK, apischema.AdminUserRef{ID: id, Email: target.Email})
}

// DeleteUser は DELETE /api/admin/users/{id}。取り消せないので、画面で打ち込んだメールアドレスが
// 本人のものと一致しないと消さない（一覧が古いまま別の行を消す事故を、id だけに頼らず止める）。
func (h *UserHandlers) DeleteUser(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	// Node と同じく path を先に、本文を後に確かめる。
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	in := httpx.ReadObject(body.Value())
	email := in.String("email", httpx.StringRule{
		Min: 1, MinMessage: "Too small: expected string to have >=1 characters",
		Max: adminUserIDMax, MaxMessage: "Too big: expected string to have <=191 characters",
	})
	if in.Reject(w) {
		return
	}

	target, ok := h.operableTarget(w, r, id, s.UserID)
	if !ok {
		return
	}
	// メールの無い相手は、空白だけを打てば空文字どうしで一致してしまうので、比べる前に断る（Node と同じ）。
	if target.Email == nil || *target.Email == "" {
		httpx.WriteError(w, http.StatusConflict, protectedMessages[protectedNoEmail])
		return
	}
	if !strings.EqualFold(*target.Email, httpx.JSTrim(email)) {
		httpx.WriteError(w, http.StatusBadRequest, "メールアドレスが一致しません")
		return
	}

	removed, err := h.store.deleteUser(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin delete user: %w", err))
		return
	}
	logAdminUserAction(r.Context(), s.UserID, "delete", target, "removed", removed)
	httpx.WriteJSON(w, http.StatusOK, apischema.AdminDeleteResult{ID: id, Email: target.Email, Removed: removed})
}

// operableTarget は停止・削除できる相手を引く。いなければ 404、守られていれば 409 を送って false を返す。
func (h *UserHandlers) operableTarget(w http.ResponseWriter, r *http.Request, id, actorID string) (*adminTarget, bool) {
	target, err := h.store.findTarget(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin find user: %w", err))
		return nil, false
	}
	if target == nil {
		httpx.WriteError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return nil, false
	}
	if reason, protected := target.protection(actorID); protected {
		httpx.WriteError(w, http.StatusConflict, protectedMessages[reason])
		return nil, false
	}
	return target, true
}

// logAdminUserAction は、誰が・誰に・何をしたかを構造化ログに残す（監査ログ、セキュリティ基準 H4）。
// Node の logUserAction と同じ項目・同じ文言（warn）なので、Grafana の Loki で Node の記録と並べて追える。
func logAdminUserAction(ctx context.Context, adminID, action string, target *adminTarget, detail ...any) {
	// *string のまま渡すと、ログの形式によってはアドレスが出る。値（無ければ null）にしてから渡す。
	var email any
	if target.Email != nil {
		email = *target.Email
	}
	attrs := append([]any{"adminId", adminID, "action", action, "targetId", target.ID, "targetEmail", email}, detail...)
	slog.WarnContext(ctx, "admin user action", attrs...)
}

// adminUserID は path の {id} を Node の z.string().min(1).max(191) と同じく確かめる。
// Node はこれより先に Fastify の maxParamLength（既定 100文字）で 414 を返すので、101〜191文字の ID は
// Node と結果が違う（Go は 404 など）。正しい ID は32文字で、画面の操作では起きないので揃えていない。
func adminUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	switch {
	case id == "":
		(&httpx.ValidationIssue{Code: "too_small", Field: "id", Message: "Too small: expected string to have >=1 characters"}).Write(w)
		return "", false
	case httpx.CodePointLength(id) > adminUserIDMax:
		(&httpx.ValidationIssue{Code: "too_big", Field: "id", Message: "Too big: expected string to have <=191 characters"}).Write(w)
		return "", false
	}
	return id, true
}

// readAdminUsersQuery は Node の listUsersQuerySchema と同じ規則でクエリを読む。Zod と同じく kind・q・page の順に
// 確かめ、最初の1件で止める。Fastify は同じキーが2つ以上あると値を配列にするので、値の数で見分ける。
func readAdminUsersQuery(query map[string][]string) (apischema.UserKind, string, int, *httpx.ValidationIssue) {
	kind := apischema.UserKindReal // z.enum(USER_KINDS).default("real")
	if values, ok := query["kind"]; ok {
		if len(values) != 1 || !apischema.UserKind(values[0]).Valid() {
			return "", "", 0, &httpx.ValidationIssue{Code: "invalid_value", Field: "kind",
				Message: `Invalid option: expected one of "real"|"sim"|"seed"|"demo"`}
		}
		kind = apischema.UserKind(values[0])
	}

	q := "" // z.string().max(191).optional()
	if values, ok := query["q"]; ok {
		if len(values) != 1 {
			return "", "", 0, httpx.InvalidType("q", "string", []any{})
		}
		if httpx.CodePointLength(values[0]) > 191 {
			return "", "", 0, &httpx.ValidationIssue{Code: "too_big", Field: "q", Message: "Too big: expected string to have <=191 characters"}
		}
		q = values[0]
	}

	page, issue := readPageQuery(query, adminUsersMaxPage)
	if issue != nil {
		return "", "", 0, issue
	}
	return kind, q, page, nil
}

// readPageQuery は z.coerce.number().int().positive().max(max).default(1) の page を読む。
func readPageQuery(query map[string][]string, max float64) (int, *httpx.ValidationIssue) {
	values, ok := query["page"]
	if !ok {
		return 1, nil
	}
	// z.coerce.number() は Number(値)。配列は "1,2" のように , でつないだ文字列として数に直る（["3"] は 3）。
	f := jsNumberFromString(strings.Join(values, ","))
	if math.IsNaN(f) {
		return 0, &httpx.ValidationIssue{Code: "invalid_type", Field: "page", Message: "Invalid input: expected number, received NaN"}
	}
	n, issue := httpx.CheckNumber("page", jsonNumberOf(f), httpx.NumberRule{Int: true, Positive: true, Max: max})
	if issue != nil {
		return 0, issue
	}
	return int(n), nil
}

// jsNumberFromString は JavaScript の Number(文字列) と同じ規則で数にする。読めなければ NaN。
//   - 前後の空白を削り、空なら 0
//   - Infinity（符号つきも）
//   - 0x・0o・0b で始まる整数（符号は付けられない）
//   - 10進の数（1e3・.5・5. も可）。Go の ParseFloat だけが読む書き方（1_0・inf・0x1p3）は NaN
func jsNumberFromString(s string) float64 {
	s = httpx.JSTrim(s)
	switch s {
	case "":
		return 0
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(s) > 2 && s[0] == '0' {
		base := map[byte]int{'x': 16, 'X': 16, 'o': 8, 'O': 8, 'b': 2, 'B': 2}[s[1]]
		if base != 0 {
			n, ok := new(big.Int).SetString(s[2:], base)
			// SetString は "_" や符号も受け付けるので、数字だけかを先に見る。
			if !ok || strings.ContainsAny(s[2:], "_+-") {
				return math.NaN()
			}
			f, _ := new(big.Float).SetInt(n).Float64()
			return f
		}
	}
	if !jsDecimalPattern.MatchString(s) {
		return math.NaN()
	}
	// 範囲外は ±Inf（ErrRange）になり、Number() と同じ。
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

var jsDecimalPattern = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// jsonNumberOf は数を httpx.CheckNumber に渡せる形（JSON から読んだ数）にする。
func jsonNumberOf(f float64) any {
	if math.IsInf(f, 0) {
		// httpx.CheckNumber は JSON から読んだ ±Infinity を json.Number の "±Inf" で受け取る（jsNumber が ParseFloat で読む）。
		if f > 0 {
			return json.Number("+Inf")
		}
		return json.Number("-Inf")
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
}

// ここから下は本物の DB。

type sqlAdminUserStore struct {
	db *sql.DB
}

func (st *sqlAdminUserStore) overview(ctx context.Context, now time.Time) (apischema.AdminOverview, error) {
	since7, since30 := now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour)
	var o apischema.AdminOverview

	// 「記録した人」は StudyLog を作った日時（createdAt）で数える。学習した日（date）は
	// 「あとから記録」で過去日にもなるので、来訪の指標には使わない。
	// EXISTS は StudyLog の userId 索引で1人ずつ引くので、全件を集計しない。
	rows, err := st.db.QueryContext(ctx,
		`SELECT `+kindSQL+` AS kind,
		        COUNT(*),
		        SUM(u.emailVerified),
		        SUM(u.createdAt >= ?),
		        SUM(EXISTS (SELECT 1 FROM StudyLog l WHERE l.userId = u.id AND l.createdAt >= ?)),
		        SUM(EXISTS (SELECT 1 FROM StudyLog l WHERE l.userId = u.id AND l.createdAt >= ?))
		 FROM `+"`user`"+` u
		 GROUP BY kind`,
		append(append([]any{}, kindParams...), since7, since7, since30)...)
	if err != nil {
		return o, err
	}
	byKind := map[apischema.UserKind]apischema.KindStats{}
	for rows.Next() {
		var k apischema.KindStats
		var verified, new7, active7, active30 sql.NullInt64
		if err := rows.Scan(&k.Kind, &k.Total, &verified, &new7, &active7, &active30); err != nil {
			rows.Close()
			return o, err
		}
		k.Verified, k.NewLast7Days = int(verified.Int64), int(new7.Int64)
		k.ActiveLast7Days, k.ActiveLast30Days = int(active7.Int64), int(active30.Int64)
		byKind[k.Kind] = k
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return o, err
	}
	// 1人もいない種別も 0 で並べ、画面の並びを毎回同じにする。
	for _, kind := range userKinds {
		k := byKind[kind]
		k.Kind = kind
		o.Kinds = append(o.Kinds, k)
	}

	// 日付は利用者の感覚に合わせて Asia/Tokyo で区切る。DATETIME は UTC で保存している。
	// CONVERT_TZ に地域名ではなく時差を渡すのは、MySQL のタイムゾーン表が無くても動くようにするため。
	signups, err := st.db.QueryContext(ctx,
		`SELECT DATE_FORMAT(CONVERT_TZ(u.createdAt, '+00:00', '+09:00'), '%Y-%m-%d') AS date, COUNT(*)
		 FROM `+"`user`"+` u
		 WHERE `+kindSQL+` = 'real' AND u.createdAt >= ?
		 GROUP BY date
		 ORDER BY date`,
		append(append([]any{}, kindParams...), since30)...)
	if err != nil {
		return o, err
	}
	defer signups.Close()
	o.RealSignupsByDay = make([]struct {
		Count int               `json:"count"`
		Date  apischema.IsoDate `json:"date"`
	}, 0)
	for signups.Next() {
		var day struct {
			Count int               `json:"count"`
			Date  apischema.IsoDate `json:"date"`
		}
		if err := signups.Scan(&day.Date, &day.Count); err != nil {
			return o, err
		}
		o.RealSignupsByDay = append(o.RealSignupsByDay, day)
	}
	return o, signups.Err()
}

// escapeLike は LIKE の % と _ を文字として扱う（検索語に含まれても全件一致にならないように）。
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (st *sqlAdminUserStore) listUsers(ctx context.Context, kind apischema.UserKind, q string, page int) (apischema.AdminUserList, error) {
	list := apischema.AdminUserList{Users: []apischema.AdminUser{}, Page: page, PageSize: adminUsersPageSize}
	where := kindSQL + " = ?"
	whereParams := append(append([]any{}, kindParams...), kind)
	if q = httpx.JSTrim(q); q != "" {
		where += " AND u.email LIKE ?"
		whereParams = append(whereParams, "%"+escapeLike(q)+"%")
	}

	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `user` u WHERE "+where, whereParams...).Scan(&list.Total); err != nil {
		return list, err
	}

	// 先に50人だけを切り出し（内側）、その50人にだけ件数や最終ログインを数える（外側）。
	// 内側に LIMIT があるので MySQL は派生表を外へ展開せず、集計は50人分で済む。
	params := append(append([]any{}, kindParams...), whereParams...)
	params = append(params, adminUsersPageSize, (page-1)*adminUsersPageSize)
	rows, err := st.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.nickname, u.name, u.kind, u.role, u.emailVerified, u.bannedAt, u.createdAt,
		        NULLIF(CONCAT_WS(',',
		          IF(EXISTS(SELECT 1 FROM AuthPassword p WHERE p.userId = u.id), 'credential', NULL),
		          (SELECT GROUP_CONCAT(i.provider ORDER BY i.provider) FROM AuthIdentity i WHERE i.userId = u.id)), ''),
		        (SELECT MAX(s.createdAt) FROM AuthSession s WHERE s.userId = u.id),
		        (SELECT COUNT(*) FROM StudyLog l WHERE l.userId = u.id),
		        (SELECT MAX(l.createdAt) FROM StudyLog l WHERE l.userId = u.id)
		 FROM (
		   SELECT u.id, u.email, u.nickname, u.name, u.role, u.emailVerified, u.bannedAt, u.createdAt,
		          `+kindSQL+` AS kind
		   FROM `+"`user`"+` u
		   WHERE `+where+`
		   ORDER BY u.createdAt DESC, u.id
		   LIMIT ? OFFSET ?
		 ) u
		 ORDER BY u.createdAt DESC, u.id`,
		params...)
	if err != nil {
		return list, err
	}
	defer rows.Close()
	for rows.Next() {
		var u apischema.AdminUser
		var providers sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.Nickname, &u.Name, &u.Kind, &u.Role, &u.EmailVerified,
			&u.BannedAt, &u.CreatedAt, &providers, &u.LastLoginAt, &u.StudyLogCount, &u.LastStudyLogAt); err != nil {
			return list, err
		}
		u.CreatedAt = database.ISOFromDatetime(u.CreatedAt)
		for _, p := range []*apischema.IsoDateTime{u.BannedAt, u.LastLoginAt, u.LastStudyLogAt} {
			if p != nil {
				*p = database.ISOFromDatetime(*p)
			}
		}
		u.Providers = []string{}
		if providers.Valid && providers.String != "" {
			u.Providers = strings.Split(providers.String, ",")
		}
		list.Users = append(list.Users, u)
	}
	return list, rows.Err()
}

func (st *sqlAdminUserStore) findTarget(ctx context.Context, id string) (*adminTarget, error) {
	var t adminTarget
	err := st.db.QueryRowContext(ctx, "SELECT id, email, role, bannedAt FROM `user` WHERE id = ?", id).
		Scan(&t.ID, &t.Email, &t.Role, &t.BannedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if t.BannedAt != nil {
		*t.BannedAt = database.ISOFromDatetime(*t.BannedAt)
	}
	return &t, nil
}

// Ban・Unban は account の操作を呼ぶだけ。interface にしているのは、DB を使わないテストで結果を作るため。
func (st *sqlAdminUserStore) ban(ctx context.Context, id string, now time.Time) (account.Suspension, error) {
	return account.Suspend(ctx, st.db, id, now, nil)
}

func (st *sqlAdminUserStore) unban(ctx context.Context, id string, now time.Time) error {
	return account.Unsuspend(ctx, st.db, id, now, nil)
}

// DeleteUser は利用者を消す。消し方は本人の退会と同じ account.DeleteUser。
func (st *sqlAdminUserStore) deleteUser(ctx context.Context, id string) (removedCounts, error) {
	removed, err := account.DeleteUser(ctx, st.db, id)
	return removedCounts(removed), err
}
