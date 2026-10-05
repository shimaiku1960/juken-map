package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// シミュレーション（sim/）専用の API（JUK-80）。Node の routes/sim.ts と services/simulation-service.ts にあたる。
//
// 守りは3重：
//  1. SIMULATION_ENABLED=on のときだけ登録する。付けなければ存在しない（404）。
//  2. SIMULATION_SECRET の Bearer が必須。未設定なら常に 401（ルーターの job が確かめる）。
//  3. 触れる相手はシミュレーション用のメールアドレス（delivered+simNNNNN@resend.dev）だけ。
//     どの SQL もアドレスの形を WHERE に入れる。simSeq が付いているかだけで絞ると、何かの間違いで
//     実ユーザーに simSeq が付いたときに触ってしまうので、二重に絞る。
//
// 登録・確認メール・ログイン・学習記録などは、実際の利用者と同じ API と同じメールの経路を通る。
// ここにあるのは、シミュレーションの管理情報（連番・続き方の型・来なくなった日）の読み書きだけ。

// simEmailLike は SQL の LIKE で「シミュレーションの利用者」だけを選ぶ条件。src/shared/synthetic.ts と同じ。
const simEmailLike = "delivered+sim%@resend.dev"

var simEmailPattern = regexp.MustCompile(`^delivered\+sim\d+@resend\.dev$`)

// isSimEmail は src/shared/synthetic.ts の isSimEmail と同じ判定（大文字小文字を区別しない）。
func isSimEmail(email string) bool {
	return simEmailPattern.MatchString(strings.ToLower(email))
}

// Node の Zod（z.string().regex(/^\d{4}-\d{2}-\d{2}$/)）と同じ形。日付として正しいかは DB に任せる。
var simDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

const invalidInput = "入力が不正です"

type simStore struct {
	db *sql.DB
}

// state は合成ユーザーの一覧（連番の昇順）と、次に使う連番。
func (st *simStore) state(ctx context.Context) (SimulationState, error) {
	// DATE 列はタイムゾーンの解釈を挟まないよう、文字列のまま返す（Node と同じ）。
	rows, err := st.db.QueryContext(ctx,
		"SELECT simSeq, email, simCohort, createdAt,"+
			" DATE_FORMAT(simDormantFrom, '%Y-%m-%d') AS dormantFrom,"+
			" DATE_FORMAT(simLastActedOn, '%Y-%m-%d') AS lastActedOn"+
			" FROM `user` WHERE simSeq IS NOT NULL AND email LIKE ? ORDER BY simSeq ASC",
		simEmailLike)
	if err != nil {
		return SimulationState{}, err
	}
	defer rows.Close()

	state := SimulationState{NextSeq: 1, Users: make([]SimulationUser, 0)}
	for rows.Next() {
		var u SimulationUser
		if err := rows.Scan(&u.Seq, &u.Email, &u.Cohort, &u.CreatedAt, &u.DormantFrom, &u.LastActedOn); err != nil {
			return SimulationState{}, err
		}
		u.CreatedAt = isoFromDatetime(u.CreatedAt)
		state.Users = append(state.Users, u)
		state.NextSeq = u.Seq + 1
	}
	return state, rows.Err()
}

// markUser は登録を済ませた合成ユーザーに連番と続き方の型を付ける。
// 該当するユーザーがいなければ notFound、連番（UNIQUE）が使用済みなら duplicate。
func (st *simStore) markUser(ctx context.Context, m SimulationUserMark) (notFound, duplicate bool, err error) {
	res, err := st.db.ExecContext(ctx,
		"UPDATE `user` SET simSeq = ?, simCohort = ?, updatedAt = ? WHERE email = ? AND email LIKE ?",
		m.Seq, m.Cohort, time.Now().UTC(), m.Email, simEmailLike)
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == 1062 { // ER_DUP_ENTRY
		return false, true, nil
	}
	if err != nil {
		return false, false, err
	}
	n, err := res.RowsAffected()
	return n == 0, false, err
}

// simUpdate は PATCH の本文。キーが無い項目は変えない（set が false）。値が nil なら NULL にする。
type simUpdate struct {
	lastActedOn, dormantFrom       *string
	setLastActedOn, setDormantFrom bool
}

// updateUser は最後に操作した日・来なくなった日を記録する。見つからなければ false。
// 変える項目が無ければ、その連番が無くても true（Node と同じく SQL を流さない）。
func (st *simStore) updateUser(ctx context.Context, seq int64, u simUpdate) (bool, error) {
	var sets []string
	var args []any
	if u.setLastActedOn {
		sets = append(sets, "simLastActedOn = ?")
		args = append(args, u.lastActedOn)
	}
	if u.setDormantFrom {
		sets = append(sets, "simDormantFrom = ?")
		args = append(args, u.dormantFrom)
	}
	if len(sets) == 0 {
		return true, nil
	}
	// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（sets）。値は args で ? として渡す
	res, err := st.db.ExecContext(ctx,
		"UPDATE `user` SET "+strings.Join(sets, ", ")+" WHERE simSeq = ? AND email LIKE ?",
		append(args, seq, simEmailLike)...)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ここから下は、本文とパスの値を Node（Zod と Number()）と同じ規則で読む部分。

// maxSafeInteger は JavaScript の Number.MAX_SAFE_INTEGER。Zod の int() はこれを超える数を弾く。
const maxSafeInteger = 1<<53 - 1

// positiveInt は Zod の z.number().int().positive() と同じ判定（1.0 は整数、1e20 は安全な範囲の外）。
func positiveInt(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	if err != nil || f != math.Trunc(f) || f <= 0 || f > maxSafeInteger {
		return 0, false
	}
	return int64(f), true
}

// parseSimMark は POST /api/sim/users の本文を読む。余計なキーは無視する（Zod の object と同じ）。
func parseSimMark(body any) (SimulationUserMark, bool) {
	m, ok := body.(map[string]any)
	if !ok {
		return SimulationUserMark{}, false
	}
	email, ok := m["email"].(string)
	// シミュレーション用のアドレスは、Zod の email() も必ず通る形なので、この判定だけでよい。
	if !ok || !isSimEmail(email) {
		return SimulationUserMark{}, false
	}
	seq, ok := positiveInt(m["seq"])
	if !ok {
		return SimulationUserMark{}, false
	}
	cohort, ok := m["cohort"].(string)
	if !ok || !SimulationCohort(cohort).Valid() {
		return SimulationUserMark{}, false
	}
	return SimulationUserMark{Email: email, Seq: seq, Cohort: SimulationCohort(cohort)}, true
}

// parseSimUpdate は PATCH /api/sim/users/{seq} の本文を読む。どちらの日付も、無い・null・"YYYY-MM-DD" のどれか。
func parseSimUpdate(body any) (simUpdate, bool) {
	m, ok := body.(map[string]any)
	if !ok {
		return simUpdate{}, false
	}
	var u simUpdate
	for key, dst := range map[string]struct {
		value **string
		set   *bool
	}{
		"lastActedOn": {&u.lastActedOn, &u.setLastActedOn},
		"dormantFrom": {&u.dormantFrom, &u.setDormantFrom},
	} {
		v, has := m[key]
		if !has {
			continue
		}
		*dst.set = true
		if v == nil {
			continue
		}
		s, ok := v.(string)
		if !ok || !simDatePattern.MatchString(s) {
			return simUpdate{}, false
		}
		*dst.value = &s
	}
	return u, true
}

// parseSeq はパスの連番を Node と同じく Number() で読む（"1e3" も 1000 になる）。
// 16進（"0x10"）のような書き方は Go の ParseFloat が読まないので 400 になる（Node は 16 と読む）。
// 整数で安全な範囲を超えるものは、どの利用者にも当たらない連番として -1 を返す（Node は SQL まで行って見つからない）。
func parseSeq(s string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsInf(f, 0) || f != math.Trunc(f) || f <= 0 {
		return 0, false
	}
	if f > math.MaxInt64/2 {
		return -1, true
	}
	return int64(f), true
}

type simHandlers struct {
	store *simStore
}

// state は GET /api/sim/state。
func (h *simHandlers) state(w http.ResponseWriter, r *http.Request) {
	state, err := h.store.state(r.Context())
	if err != nil {
		internalError(w, r, fmt.Errorf("sim state: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// markUser は POST /api/sim/users。
func (h *simHandlers) markUser(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	m, ok := parseSimMark(body.json)
	if !ok {
		writeError(w, http.StatusBadRequest, invalidInput)
		return
	}
	notFound, duplicate, err := h.store.markUser(r.Context(), m)
	switch {
	case err != nil:
		internalError(w, r, fmt.Errorf("sim mark user: %w", err))
	case notFound:
		writeError(w, http.StatusNotFound, "ユーザーが見つかりません")
	case duplicate:
		writeError(w, http.StatusConflict, "この連番はすでに使われています")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// updateUser は PATCH /api/sim/users/{seq}。
func (h *simHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	seq, seqOK := parseSeq(r.PathValue("seq"))
	u, bodyOK := parseSimUpdate(body.json)
	if !seqOK || !bodyOK {
		writeError(w, http.StatusBadRequest, invalidInput)
		return
	}
	found, err := h.store.updateUser(r.Context(), seq, u)
	switch {
	case err != nil:
		internalError(w, r, fmt.Errorf("sim update user: %w", err))
	case !found:
		writeError(w, http.StatusNotFound, "ユーザーが見つかりません")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
