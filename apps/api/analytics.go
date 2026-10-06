package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

// 本登録の完了を GA4 の sign_up として1回だけ数えるための問い合わせ（JUK-80）。
// Node の routes/analytics.ts と、services/user-service.ts の markSignUpTracked・findSignUpMethod にあたる。
// 二重に数えない判定はサーバーが持ち、画面（apps/web の lib/analytics.ts）は true のときだけ GA4 に送る。

type analyticsStore struct {
	db *sql.DB
}

// markSignUpTracked は「計測済み」の印を付ける。付けられたら（初回なら）true。
// 同時に2回来ても true になるのは1回だけ（account.MarkSignUpTracked）。
func (st *analyticsStore) markSignUpTracked(ctx context.Context, userID string) (bool, error) {
	return account.MarkSignUpTracked(ctx, st.db, userID, time.Now().UTC())
}

// findSignUpMethod は登録に使われた認証方法を判定する。登録と同時（1分以内）に作られた外部ログインの結びつきが
// あれば、そのプロバイダー（Google・GitHub で登録した人）。無ければメールでの登録。あとから外部ログインを
// 連携したメールの利用者を、外部ログインで登録したと数えないため。
func (st *analyticsStore) findSignUpMethod(ctx context.Context, userID string) (apischema.RegistrationTrackingMethod, error) {
	var provider string
	err := st.db.QueryRowContext(ctx,
		"SELECT i.provider FROM AuthIdentity AS i JOIN `user` AS u ON u.id = i.userId"+
			" WHERE i.userId = ? AND i.createdAt <= DATE_ADD(u.createdAt, INTERVAL 1 MINUTE) ORDER BY i.createdAt ASC LIMIT 1", userID,
	).Scan(&provider)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	switch apischema.RegistrationTrackingMethod(provider) {
	case apischema.SignUpMethodGoogle, apischema.SignUpMethodGithub:
		return apischema.RegistrationTrackingMethod(provider), nil
	}
	return apischema.SignUpMethodEmail, nil
}

type analyticsHandlers struct {
	store *analyticsStore
}

// registration は POST /api/analytics/registration。2回目以降は shouldTrack: false で黙って終わる。
func (h *analyticsHandlers) registration(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	// 本文は使わないが、Node と同じく形と大きさは確かめる（受け付けない形なら 415）。
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	first, err := h.store.markSignUpTracked(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("analytics: %w", err))
		return
	}
	if !first {
		httpx.WriteJSON(w, http.StatusOK, apischema.RegistrationTracking{ShouldTrack: false})
		return
	}
	method, err := h.store.findSignUpMethod(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("analytics: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.RegistrationTracking{ShouldTrack: true, Method: &method})
}
