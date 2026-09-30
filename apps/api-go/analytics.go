package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// 本登録の完了を GA4 の sign_up として1回だけ数えるための問い合わせ（JUK-80）。
// Node の routes/analytics.ts と、services/user-service.ts の markSignUpTracked・findSignUpMethod にあたる。
// 二重に数えない判定はサーバーが持ち、画面（apps/web の lib/analytics.ts）は true のときだけ GA4 に送る。

type analyticsStore struct {
	db *sql.DB
}

// markSignUpTracked は「計測済み」の印を付ける。付けられたら（初回なら）true。
// 印が無い行だけを UPDATE するので、同時に2回来ても true になるのは1回だけ。
func (st *analyticsStore) markSignUpTracked(ctx context.Context, userID string) (bool, error) {
	now := time.Now().UTC()
	res, err := st.db.ExecContext(ctx,
		"UPDATE `user` SET analyticsSignUpTrackedAt = ?, updatedAt = ? WHERE id = ? AND analyticsSignUpTrackedAt IS NULL",
		now, now, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// findSignUpMethod は登録に使われた認証方法を、最初に作られたアカウントから判定する。
// account は Better Auth が作るテーブルで、メール登録なら providerId は "credential"。
func (st *analyticsStore) findSignUpMethod(ctx context.Context, userID string) (RegistrationTrackingMethod, error) {
	var provider string
	err := st.db.QueryRowContext(ctx,
		"SELECT providerId FROM account WHERE userId = ? ORDER BY createdAt ASC LIMIT 1", userID,
	).Scan(&provider)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	switch RegistrationTrackingMethod(provider) {
	case SignUpMethodGoogle, SignUpMethodGithub:
		return RegistrationTrackingMethod(provider), nil
	}
	return SignUpMethodEmail, nil
}

type analyticsHandlers struct {
	store *analyticsStore
}

// registration は POST /api/analytics/registration。2回目以降は shouldTrack: false で黙って終わる。
func (h *analyticsHandlers) registration(w http.ResponseWriter, r *http.Request, s *session) {
	// 本文は使わないが、Node と同じく形と大きさは確かめる（受け付けない形なら 415）。
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	first, err := h.store.markSignUpTracked(r.Context(), s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("analytics: %w", err))
		return
	}
	if !first {
		writeJSON(w, http.StatusOK, RegistrationTracking{ShouldTrack: false})
		return
	}
	method, err := h.store.findSignUpMethod(r.Context(), s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("analytics: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, RegistrationTracking{ShouldTrack: true, Method: &method})
}
