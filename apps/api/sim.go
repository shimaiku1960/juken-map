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

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/simulation"
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
// 書き込みは持ち主の internal/write/simulation にある（JUK-154）。

// simEmailLike は SQL の LIKE で「シミュレーションの利用者」だけを選ぶ条件。src/shared/synthetic.ts と同じ。
const simEmailLike = simulation.EmailLike

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
func (st *simStore) state(ctx context.Context) (apischema.SimulationState, error) {
	// DATE 列はタイムゾーンの解釈を挟まないよう、文字列のまま返す（Node と同じ）。
	rows, err := st.db.QueryContext(ctx,
		"SELECT simSeq, email, simCohort, createdAt,"+
			" DATE_FORMAT(simDormantFrom, '%Y-%m-%d') AS dormantFrom,"+
			" DATE_FORMAT(simLastActedOn, '%Y-%m-%d') AS lastActedOn"+
			" FROM `user` WHERE simSeq IS NOT NULL AND email LIKE ? ORDER BY simSeq ASC",
		simEmailLike)
	if err != nil {
		return apischema.SimulationState{}, err
	}
	defer rows.Close()

	state := apischema.SimulationState{NextSeq: 1, Users: make([]apischema.SimulationUser, 0)}
	for rows.Next() {
		var u apischema.SimulationUser
		if err := rows.Scan(&u.Seq, &u.Email, &u.Cohort, &u.CreatedAt, &u.DormantFrom, &u.LastActedOn); err != nil {
			return apischema.SimulationState{}, err
		}
		u.CreatedAt = database.ISOFromDatetime(u.CreatedAt)
		state.Users = append(state.Users, u)
		state.NextSeq = u.Seq + 1
	}
	return state, rows.Err()
}

// simUpdate は PATCH の本文。キーが無い項目は変えない（set が false）。値が nil なら NULL にする。
type simUpdate struct {
	lastActedOn, dormantFrom       *string
	setLastActedOn, setDormantFrom bool
}

// activity は持ち主に渡す形にする。
func (u simUpdate) activity() simulation.Activity {
	return simulation.Activity{
		LastActedOn: opt.Field[string]{Present: u.setLastActedOn, Value: u.lastActedOn},
		DormantFrom: opt.Field[string]{Present: u.setDormantFrom, Value: u.dormantFrom},
	}
}

// ここから下は、本文とパスの値を Node（Zod と Number()）と同じ規則で読む部分。

// positiveInt は Zod の z.number().int().positive() と同じ判定（1.0 は整数、1e20 は安全な範囲の外）。
func positiveInt(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	if err != nil || f != math.Trunc(f) || f <= 0 || f > httpx.MaxSafeInteger {
		return 0, false
	}
	return int64(f), true
}

// parseSimMark は POST /api/sim/users の本文を読む。余計なキーは無視する（Zod の object と同じ）。
func parseSimMark(body any) (apischema.SimulationUserMark, bool) {
	m, ok := body.(map[string]any)
	if !ok {
		return apischema.SimulationUserMark{}, false
	}
	email, ok := m["email"].(string)
	// シミュレーション用のアドレスは、Zod の email() も必ず通る形なので、この判定だけでよい。
	if !ok || !isSimEmail(email) {
		return apischema.SimulationUserMark{}, false
	}
	seq, ok := positiveInt(m["seq"])
	if !ok {
		return apischema.SimulationUserMark{}, false
	}
	cohort, ok := m["cohort"].(string)
	if !ok || !apischema.SimulationCohort(cohort).Valid() {
		return apischema.SimulationUserMark{}, false
	}
	return apischema.SimulationUserMark{Email: email, Seq: seq, Cohort: apischema.SimulationCohort(cohort)}, true
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
		httpx.InternalError(w, r, fmt.Errorf("sim state: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, state)
}

// markUser は POST /api/sim/users。
func (h *simHandlers) markUser(w http.ResponseWriter, r *http.Request) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	m, ok := parseSimMark(body.JSON)
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, invalidInput)
		return
	}
	err := simulation.Mark(r.Context(), h.store.db, m.Email, m.Seq, string(m.Cohort), time.Now().UTC())
	if !writeSimError(w, r, "sim mark user", err) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// updateUser は PATCH /api/sim/users/{seq}。
func (h *simHandlers) updateUser(w http.ResponseWriter, r *http.Request) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	seq, seqOK := parseSeq(r.PathValue("seq"))
	u, bodyOK := parseSimUpdate(body.JSON)
	if !seqOK || !bodyOK {
		httpx.WriteError(w, http.StatusBadRequest, invalidInput)
		return
	}
	err := simulation.RecordActivity(r.Context(), h.store.db, seq, u.activity())
	if !writeSimError(w, r, "sim update user", err) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeSimError は持ち主の失敗を返して true を返す。失敗でなければ false。
func writeSimError(w http.ResponseWriter, r *http.Request, op string, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, simulation.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, simulation.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		httpx.InternalError(w, r, fmt.Errorf("%s: %w", op, err))
	}
	return true
}
