package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"
)

// プロフィールの更新（JUK-75）。Node の routes/profile.ts と services/user-service.ts の updateProfile にあたる。

// 入力チェックは Zod の profileSchema（src/shared/validations/profile.ts）と同じ。
// 前後の空白を削ってから長さを確かめるので、空白だけのニックネームは「必須です」で弾く（JUK-64）。
var nicknameRule = stringRule{
	typeMessage: "ニックネームは文字列で入力してください",
	trimFirst:   true,
	min:         1,
	minMessage:  "ニックネームは必須です",
	max:         50,
	maxMessage:  "50文字以内で入力してください",
}

type userStore struct {
	db *sql.DB
}

// updateProfile はニックネームを書き換え、更新後の行を返す（Node と同じく UPDATE の後に SELECT し直す）。
func (st *userStore) updateProfile(ctx context.Context, userID string, in ProfileInput) (User, error) {
	if _, err := st.db.ExecContext(ctx,
		"UPDATE `user` SET nickname = ?, updatedAt = ? WHERE id = ?",
		in.Nickname, nowMillis(), userID,
	); err != nil {
		return User{}, err
	}

	var u User
	err := st.db.QueryRowContext(ctx,
		`SELECT id, name, email, image, nickname, createdAt, updatedAt,
		        emailVerified, firstStudyLogAt, analyticsSignUpTrackedAt
		 FROM `+"`user`"+` WHERE id = ?`,
		userID,
	).Scan(&u.ID, &u.Name, &u.Email, &u.Image, &u.Nickname, &u.CreatedAt, &u.UpdatedAt,
		&u.EmailVerified, &u.FirstStudyLogAt, &u.AnalyticsSignUpTrackedAt)
	if err != nil {
		// 行が無い（sql.ErrNoRows）も 500。Node も「見つかりません」を throw して 500 にしている。
		return User{}, err
	}
	u.CreatedAt = isoFromDatetime(u.CreatedAt)
	u.UpdatedAt = isoFromDatetime(u.UpdatedAt)
	for _, p := range []*IsoDateTime{u.FirstStudyLogAt, u.AnalyticsSignUpTrackedAt} {
		if p != nil {
			*p = isoFromDatetime(*p)
		}
	}
	return u, nil
}

type profileHandlers struct {
	store *userStore
}

// update は PUT /api/profile。
func (h *profileHandlers) update(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	in := readObject(body.value())
	input := ProfileInput{Nickname: in.string("nickname", nicknameRule)}
	if in.reject(w) {
		return
	}

	u, err := h.store.updateProfile(r.Context(), s.UserID, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("profile: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, u)
}

// nowMillis は今の時刻をミリ秒で切り捨てたもの。DB の DATETIME(3) に書く値に使う。
// Node の new Date() はミリ秒までしか持たない。Go の time.Now() はナノ秒まで持ち、そのまま渡すと
// MySQL が小数第3位へ丸める（切り上がることがある）ので、先に切り捨てて Node と同じ値にする。
func nowMillis() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}
