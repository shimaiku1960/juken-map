// Package goals は志望校の入口（GET・POST /api/goals、GET /api/goals/first-choice、PUT・PATCH・DELETE /api/goals/{id}）。
// ハンドラと読み取りの SQL を持ち、書き込みは internal/write/goal に任せる（JUK-156）。
package goals

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 志望校の読み取り（JUK-73）。Node の routes/goals.ts・home.ts の GET と、
// services/goal-service.ts の listGoals・findFirstChoiceGoal にあたる。
// 書き込み（POST /api/goals・PUT/PATCH/DELETE /api/goals/:id）は goal_writes.go。

// 応答の型は openapi/openapi.yaml から生成した Goal（一覧。学部のタグつき）と
// FirstChoiceGoal（第一志望。タグは無く、キーごと出さない）。Node の pickGoal・pickFaculty と同じ形。
// 日時は Date を JSON にしたときと同じ ISO 文字列。

// 志望校 → 学部 → 大学は「多対1」の連なりなので、JOIN しても行は増えない。
// 学部 → タグだけが1対多（中間テーブル _FacultyToTag、A = Faculty.id, B = Tag.id）。
const goalColumns = `
  g.id, g.createdAt, g.userId, g.facultyId, g.isFirstChoice, g.note, g.status,
  f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
  f.createdAt AS f_createdAt, f.universityId AS f_universityId,
  u.id AS u_id, u.name AS u_name, u.prefecture AS u_prefecture,
  u.type AS u_type, u.createdAt AS u_createdAt`

const fromGoalWithFaculty = `
  FROM FinalGoal AS g
  JOIN Faculty AS f ON f.id = g.facultyId
  JOIN University AS u ON u.id = f.universityId`

// goalDest は goalColumns の順に Scan の受け皿を並べる。日時は文字列で受けるので、
// 読み終えたら fixGoalDates で ISO にする。
func goalDest(g *apischema.FirstChoiceGoal) []any {
	f, u := &g.Faculty, &g.Faculty.University
	return []any{
		&g.ID, &g.CreatedAt, &g.UserID, &g.FacultyID, &g.IsFirstChoice, &g.Note, &g.Status,
		&f.ID, &f.Name, &f.ExamDate, &f.CreatedAt, &f.UniversityID,
		&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.CreatedAt,
	}
}

func fixGoalDates(g *apischema.FirstChoiceGoal) {
	g.CreatedAt = database.ISOFromDatetime(g.CreatedAt)
	g.Faculty.ExamDate = database.ISOFromDatetime(g.Faculty.ExamDate)
	g.Faculty.CreatedAt = database.ISOFromDatetime(g.Faculty.CreatedAt)
	g.Faculty.University.CreatedAt = database.ISOFromDatetime(g.Faculty.University.CreatedAt)
}

// withTags は第一志望の形に、学部のタグを足して一覧の1件にする。
// タグが無い学部でも [] を返す（Node と同じ）ので、空のスライスで始める。
func withTags(g apischema.FirstChoiceGoal) apischema.Goal {
	f := g.Faculty
	return apischema.Goal{
		ID: g.ID, CreatedAt: g.CreatedAt, UserID: g.UserID, FacultyID: g.FacultyID,
		IsFirstChoice: g.IsFirstChoice, Note: g.Note, Status: g.Status,
		Faculty: apischema.FacultyWithUniversityAndTags{
			ID: f.ID, Name: f.Name, ExamDate: f.ExamDate, CreatedAt: f.CreatedAt,
			UniversityID: f.UniversityID, University: f.University, Tags: make([]apischema.Tag, 0),
		},
	}
}

type store struct {
	db *sql.DB
}

// listGoals は志望校ページ用。学部・大学に加え、学部のタグまで引く。
// 作った順に並べ、同じ日時どうしは id で、タグは id で並べる（Node と同じ）。
func (st *store) listGoals(ctx context.Context, userID string) ([]apischema.Goal, error) {
	rows, err := st.db.QueryContext(ctx,
		"SELECT"+goalColumns+`,
		        t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt`+
			fromGoalWithFaculty+`
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 WHERE g.userId = ?
		 ORDER BY g.createdAt ASC, g.id ASC, t.id ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]apischema.Goal, 0)
	for rows.Next() {
		var (
			row      apischema.FirstChoiceGoal
			tagID    *int64
			tagName  *string
			tagAdded *string
		)
		if err := rows.Scan(append(goalDest(&row), &tagID, &tagName, &tagAdded)...); err != nil {
			return nil, err
		}
		// ORDER BY で同じ志望校の行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(goals) == 0 || goals[len(goals)-1].ID != row.ID {
			fixGoalDates(&row)
			goals = append(goals, withTags(row))
		}
		// タグが1つも無い学部は、タグの列が NULL の行が1行だけ来る
		if tagID != nil {
			last := &goals[len(goals)-1]
			last.Faculty.Tags = append(last.Faculty.Tags, apischema.Tag{
				ID: *tagID, Name: *tagName, CreatedAt: database.ISOFromDatetime(*tagAdded),
			})
		}
	}
	return goals, rows.Err()
}

// findFirstChoiceGoal はトップの「第一志望」表示専用。タグは画面で使わないので引かない。
// 無ければ nil（JSON では null）。
func (st *store) findFirstChoiceGoal(ctx context.Context, userID string) (*apischema.FirstChoiceGoal, error) {
	// 第一志望は1ユーザー1校（Node の applyGoalPatch が保つ）。DB の制約ではないので、
	// 万一2校あっても結果が揺れないよう id で並べて1件にする。
	var g apischema.FirstChoiceGoal
	err := st.db.QueryRowContext(ctx,
		"SELECT"+goalColumns+fromGoalWithFaculty+`
		 WHERE g.userId = ? AND g.status = 'decided' AND g.isFirstChoice = TRUE
		 ORDER BY g.id ASC
		 LIMIT 1`,
		userID,
	).Scan(goalDest(&g)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fixGoalDates(&g)
	return &g, nil
}

type Handlers struct {
	store *store
}

func New(db *sql.DB) *Handlers { return &Handlers{store: &store{db: db}} }

// List は GET /api/goals。
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	goals, err := h.store.listGoals(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("goals: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, goals)
}

// FirstChoice は GET /api/goals/first-choice。第一志望が無ければ null を返す（Node と同じ）。
func (h *Handlers) FirstChoice(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	goal, err := h.store.findFirstChoiceGoal(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("goals/first-choice: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, goal)
}
