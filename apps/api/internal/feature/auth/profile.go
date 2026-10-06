package auth

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

// プロフィールの更新（JUK-75）。Node の routes/profile.ts と services/user-service.ts の updateProfile にあたる。

// 入力チェックは Zod の profileSchema（src/shared/validations/profile.ts）と同じ。
// 前後の空白を削ってから長さを確かめるので、空白だけのニックネームは「必須です」で弾く（JUK-64）。
var nicknameRule = httpx.StringRule{
	TypeMessage: "ニックネームは文字列で入力してください",
	TrimFirst:   true,
	Min:         1,
	MinMessage:  "ニックネームは必須です",
	Max:         50,
	MaxMessage:  "50文字以内で入力してください",
}

type userStore struct {
	db *sql.DB
}

// updateProfile はニックネームを書き換え、更新後の行を返す（Node と同じく UPDATE の後に SELECT し直す）。
func (st *userStore) updateProfile(ctx context.Context, userID string, in apischema.ProfileInput) (apischema.User, error) {
	if err := account.SetNickname(ctx, st.db, userID, in.Nickname, dates.NowMillis()); err != nil {
		return apischema.User{}, err
	}

	var u apischema.User
	err := st.db.QueryRowContext(ctx,
		`SELECT id, name, email, image, nickname, createdAt, updatedAt,
		        emailVerified, firstStudyLogAt, analyticsSignUpTrackedAt
		 FROM `+"`user`"+` WHERE id = ?`,
		userID,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Image, &u.Nickname, &u.CreatedAt, &u.UpdatedAt,
		&u.EmailVerified, &u.FirstStudyLogAt, &u.AnalyticsSignUpTrackedAt)
	if err != nil {
		// 行が無い（sql.ErrNoRows）も 500。Node も「見つかりません」を throw して 500 にしている。
		return apischema.User{}, err
	}
	u.CreatedAt = database.ISOFromDatetime(u.CreatedAt)
	u.UpdatedAt = database.ISOFromDatetime(u.UpdatedAt)
	for _, p := range []*apischema.IsoDateTime{u.FirstStudyLogAt, u.AnalyticsSignUpTrackedAt} {
		if p != nil {
			*p = database.ISOFromDatetime(*p)
		}
	}
	return u, nil
}

type ProfileHandlers struct {
	store *userStore
}

// NewProfileHandlers はプロフィールの入口を組み立てる。
func NewProfileHandlers(db *sql.DB) *ProfileHandlers {
	return &ProfileHandlers{store: &userStore{db: db}}
}

// Update は PUT /api/profile。
func (h *ProfileHandlers) Update(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	in := httpx.ReadObject(body.Value())
	input := apischema.ProfileInput{Nickname: in.String("nickname", nicknameRule)}
	if in.Reject(w) {
		return
	}

	u, err := h.store.updateProfile(r.Context(), s.UserID, input)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("profile: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, u)
}
