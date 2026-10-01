package main

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
)

// 管理者ページ（/admin）の利用者の管理（JUK-78）。Node の routes/admin.ts と services/admin-service.ts にあたる。
// どれも rt.admin（管理者＋2段階認証を通したセッションだけ）で登録する。守るのはルーターで、
// 画面がメニューを出し分けているのは見た目のためだけ。
//
// 停止の分担：Go は bannedAt を書き、その人の session を消して今の画面を落とすだけ。次のログインを断るのは
// Node（Better Auth の auth.ts、session.create.before が bannedAt を見る）で、ログインは Node に残っている。

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
var userKinds = []UserKind{UserKindReal, UserKindSim, UserKindSeed, UserKindDemo}

// kindSQL は利用者の種別を SQL の中で決める。判定の順番に意味がある：sim は simSeq で、
// seed はメールの印で、デモは固定アドレスで見分ける。? を含むので、使うたびに kindParams を同じ位置に並べる。
const kindSQL = `CASE
  WHEN u.simSeq IS NOT NULL THEN 'sim'
  WHEN u.email LIKE ? THEN 'seed'
  WHEN u.email = ? THEN 'demo'
  ELSE 'real'
END`

var kindParams = []any{seedEmailLike, demoEmail}

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
	case t.Email != nil && *t.Email == demoEmail:
		return protectedDemo, true
	}
	return "", false
}

// adminUserStore は DB の読み書き。テストでは偽物を渡す（Go の CI には DB が無い）。
type adminUserStore interface {
	overview(ctx context.Context, now time.Time) (AdminOverview, error)
	listUsers(ctx context.Context, kind UserKind, q string, page int) (AdminUserList, error)
	// findTarget は相手を引く。いなければ nil。
	findTarget(ctx context.Context, id string) (*adminTarget, error)
	// ban は bannedAt を書き（すでに止まっていれば最初の日時のまま）、その人の session を消す。消した数を返す。
	ban(ctx context.Context, id string, now time.Time) (sessionsRemoved int, err error)
	unban(ctx context.Context, id string, now time.Time) error
	// deleteUser は利用者を消し、一緒に消える行の数を返す（数えるのは記録のためだけ）。
	deleteUser(ctx context.Context, id string) (removedCounts, error)
}

// removedCounts は削除で一緒に消えた行数。生成した AdminDeleteResult.Removed と同じ形。
type removedCounts = struct {
	FinalGoals int `json:"finalGoals"`
	StudyLogs  int `json:"studyLogs"`
	StudyPlans int `json:"studyPlans"`
	Textbooks  int `json:"textbooks"`
}

type adminUserHandlers struct {
	store adminUserStore
	now   func() time.Time
}

// overview は GET /api/admin/overview。
func (h *adminUserHandlers) overview(w http.ResponseWriter, r *http.Request, _ *session) {
	o, err := h.store.overview(r.Context(), h.now())
	if err != nil {
		internalError(w, r, fmt.Errorf("admin overview: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// listUsers は GET /api/admin/users。
func (h *adminUserHandlers) listUsers(w http.ResponseWriter, r *http.Request, _ *session) {
	kind, q, page, issue := readAdminUsersQuery(parseQuery(r.URL.RawQuery))
	if issue != nil {
		issue.write(w)
		return
	}
	list, err := h.store.listUsers(r.Context(), kind, q, page)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin users: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// ban は POST /api/admin/users/{id}/ban。
func (h *adminUserHandlers) ban(w http.ResponseWriter, r *http.Request, s *session) {
	// 本文は使わないが、Node（Fastify）はハンドラより先に本文を読むので、受け付けない形なら同じく 415・413 にする。
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
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

	now := nowMillis()
	removed, err := h.store.ban(r.Context(), id, now)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin ban: %w", err))
		return
	}
	// 押し直しても最初に止めた日時を保つ（Node の COALESCE と同じ）。
	bannedAt := isoMillis(now)
	if target.BannedAt != nil {
		bannedAt = *target.BannedAt
	}
	logAdminUserAction(r.Context(), s.UserID, "ban", target, "sessionsRemoved", removed)
	writeJSON(w, http.StatusOK, AdminBanResult{ID: id, Email: target.Email, BannedAt: bannedAt, SessionsRemoved: removed})
}

// unban は POST /api/admin/users/{id}/unban。守りは見ない（Node と同じ。止まっていなければ何も変わらない）。
func (h *adminUserHandlers) unban(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	target, err := h.store.findTarget(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin unban: %w", err))
		return
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return
	}
	if err := h.store.unban(r.Context(), id, nowMillis()); err != nil {
		internalError(w, r, fmt.Errorf("admin unban: %w", err))
		return
	}
	logAdminUserAction(r.Context(), s.UserID, "unban", target)
	writeJSON(w, http.StatusOK, AdminUserRef{ID: id, Email: target.Email})
}

// deleteUser は DELETE /api/admin/users/{id}。取り消せないので、画面で打ち込んだメールアドレスが
// 本人のものと一致しないと消さない（一覧が古いまま別の行を消す事故を、id だけに頼らず止める）。
func (h *adminUserHandlers) deleteUser(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	// Node と同じく path を先に、本文を後に確かめる。
	id, ok := adminUserID(w, r)
	if !ok {
		return
	}
	in := readObject(body.value())
	email := in.string("email", stringRule{
		min: 1, minMessage: "Too small: expected string to have >=1 characters",
		max: adminUserIDMax, maxMessage: "Too big: expected string to have <=191 characters",
	})
	if in.reject(w) {
		return
	}

	target, ok := h.operableTarget(w, r, id, s.UserID)
	if !ok {
		return
	}
	// メールの無い相手は、空白だけを打てば空文字どうしで一致してしまうので、比べる前に断る（Node と同じ）。
	if target.Email == nil || *target.Email == "" {
		writeError(w, http.StatusConflict, protectedMessages[protectedNoEmail])
		return
	}
	if strings.ToLower(*target.Email) != strings.ToLower(jsTrim(email)) {
		writeError(w, http.StatusBadRequest, "メールアドレスが一致しません")
		return
	}

	removed, err := h.store.deleteUser(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin delete user: %w", err))
		return
	}
	logAdminUserAction(r.Context(), s.UserID, "delete", target, "removed", removed)
	writeJSON(w, http.StatusOK, AdminDeleteResult{ID: id, Email: target.Email, Removed: removed})
}

// operableTarget は停止・削除できる相手を引く。いなければ 404、守られていれば 409 を送って false を返す。
func (h *adminUserHandlers) operableTarget(w http.ResponseWriter, r *http.Request, id, actorID string) (*adminTarget, bool) {
	target, err := h.store.findTarget(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin find user: %w", err))
		return nil, false
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "ユーザーが見つかりません")
		return nil, false
	}
	if reason, protected := target.protection(actorID); protected {
		writeError(w, http.StatusConflict, protectedMessages[reason])
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
		(&validationIssue{code: "too_small", field: "id", message: "Too small: expected string to have >=1 characters"}).write(w)
		return "", false
	case codePointLength(id) > adminUserIDMax:
		(&validationIssue{code: "too_big", field: "id", message: "Too big: expected string to have <=191 characters"}).write(w)
		return "", false
	}
	return id, true
}

// readAdminUsersQuery は Node の listUsersQuerySchema と同じ規則でクエリを読む。Zod と同じく kind・q・page の順に
// 確かめ、最初の1件で止める。Fastify は同じキーが2つ以上あると値を配列にするので、値の数で見分ける。
func readAdminUsersQuery(query map[string][]string) (UserKind, string, int, *validationIssue) {
	kind := UserKindReal // z.enum(USER_KINDS).default("real")
	if values, ok := query["kind"]; ok {
		if len(values) != 1 || !UserKind(values[0]).Valid() {
			return "", "", 0, &validationIssue{code: "invalid_value", field: "kind",
				message: `Invalid option: expected one of "real"|"sim"|"seed"|"demo"`}
		}
		kind = UserKind(values[0])
	}

	q := "" // z.string().max(191).optional()
	if values, ok := query["q"]; ok {
		if len(values) != 1 {
			return "", "", 0, invalidType("q", "string", []any{})
		}
		if codePointLength(values[0]) > 191 {
			return "", "", 0, &validationIssue{code: "too_big", field: "q", message: "Too big: expected string to have <=191 characters"}
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
func readPageQuery(query map[string][]string, max float64) (int, *validationIssue) {
	values, ok := query["page"]
	if !ok {
		return 1, nil
	}
	// z.coerce.number() は Number(値)。配列は "1,2" のように , でつないだ文字列として数に直る（["3"] は 3）。
	f := jsNumberFromString(strings.Join(values, ","))
	if math.IsNaN(f) {
		return 0, &validationIssue{code: "invalid_type", field: "page", message: "Invalid input: expected number, received NaN"}
	}
	n, issue := checkNumber("page", jsonNumberOf(f), numberRule{int: true, positive: true, max: max})
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
	s = jsTrim(s)
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

// jsonNumberOf は数を checkNumber に渡せる形（JSON から読んだ数）にする。
func jsonNumberOf(f float64) any {
	if math.IsInf(f, 0) {
		// checkNumber は JSON から読んだ ±Infinity を json.Number の "±Inf" で受け取る（jsNumber が ParseFloat で読む）。
		if f > 0 {
			return json.Number("+Inf")
		}
		return json.Number("-Inf")
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
}

// isoMillis は時刻を Node の Date#toISOString と同じ形にする。
func isoMillis(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// ここから下は本物の DB。

type sqlAdminUserStore struct {
	db *sql.DB
}

func (st *sqlAdminUserStore) overview(ctx context.Context, now time.Time) (AdminOverview, error) {
	since7, since30 := now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour)
	var o AdminOverview

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
	byKind := map[UserKind]KindStats{}
	for rows.Next() {
		var k KindStats
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
		Count int     `json:"count"`
		Date  IsoDate `json:"date"`
	}, 0)
	for signups.Next() {
		var day struct {
			Count int     `json:"count"`
			Date  IsoDate `json:"date"`
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

func (st *sqlAdminUserStore) listUsers(ctx context.Context, kind UserKind, q string, page int) (AdminUserList, error) {
	list := AdminUserList{Users: []AdminUser{}, Page: page, PageSize: adminUsersPageSize}
	where := kindSQL + " = ?"
	whereParams := append(append([]any{}, kindParams...), kind)
	if q = jsTrim(q); q != "" {
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
		        (SELECT GROUP_CONCAT(DISTINCT a.providerId ORDER BY a.providerId)
		           FROM account a WHERE a.userId = u.id),
		        (SELECT MAX(s.createdAt) FROM session s WHERE s.userId = u.id),
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
		var u AdminUser
		var providers sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.Nickname, &u.Name, &u.Kind, &u.Role, &u.EmailVerified,
			&u.BannedAt, &u.CreatedAt, &providers, &u.LastLoginAt, &u.StudyLogCount, &u.LastStudyLogAt); err != nil {
			return list, err
		}
		u.CreatedAt = isoFromDatetime(u.CreatedAt)
		for _, p := range []*IsoDateTime{u.BannedAt, u.LastLoginAt, u.LastStudyLogAt} {
			if p != nil {
				*p = isoFromDatetime(*p)
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
		*t.BannedAt = isoFromDatetime(*t.BannedAt)
	}
	return &t, nil
}

// ban は停止の印と、今つながっている画面を落とすための session の削除を1つのトランザクションで行う。
// 両方そろって初めて「止まった」と言える（次のログインは Node の auth.ts が bannedAt を見て断る）。
func (st *sqlAdminUserStore) ban(ctx context.Context, id string, now time.Time) (int, error) {
	var removed int64
	err := inTx(ctx, st.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"UPDATE `user` SET bannedAt = COALESCE(bannedAt, ?), updatedAt = ? WHERE id = ?", now, now, id); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "DELETE FROM session WHERE userId = ?", id)
		if err != nil {
			return err
		}
		removed, err = res.RowsAffected()
		return err
	})
	return int(removed), err
}

func (st *sqlAdminUserStore) unban(ctx context.Context, id string, now time.Time) error {
	_, err := st.db.ExecContext(ctx, "UPDATE `user` SET bannedAt = NULL, updatedAt = ? WHERE id = ?", now, id)
	return err
}

// deleteUser は利用者を消す。ぶら下がっている行（StudyLog・StudyPlan・Textbook・FinalGoal・session・account・
// 通知・LINE 関連など）は外部キーの ON DELETE CASCADE で一緒に消える。
func (st *sqlAdminUserStore) deleteUser(ctx context.Context, id string) (removedCounts, error) {
	var c removedCounts
	err := inTx(ctx, st.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx,
			`SELECT (SELECT COUNT(*) FROM StudyLog WHERE userId = ?),
			        (SELECT COUNT(*) FROM StudyPlan WHERE userId = ?),
			        (SELECT COUNT(*) FROM Textbook WHERE userId = ?),
			        (SELECT COUNT(*) FROM FinalGoal WHERE userId = ?)`,
			id, id, id, id,
		).Scan(&c.StudyLogs, &c.StudyPlans, &c.Textbooks, &c.FinalGoals); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM `user` WHERE id = ?", id)
		return err
	})
	return c, err
}
