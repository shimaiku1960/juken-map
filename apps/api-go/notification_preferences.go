package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// 通知設定の読み取り（JUK-73）。Node の routes/notification-preferences.ts の GET と、
// services/notification-service.ts の findNotificationPreference にあたる。
// 書き込み（PUT /api/notification-preferences）は Node に残っていて、nginx が GET と HEAD だけを Go へ送る。

type notificationPreferenceStore struct {
	db *sql.DB
}

// find は自分の通知設定を返す。まだ保存していなければ、全部 false（Node の DEFAULT_PREFERENCE と同じ）。
func (st *notificationPreferenceStore) find(ctx context.Context, userID string) (NotificationPreference, error) {
	var p NotificationPreference
	err := st.db.QueryRowContext(ctx,
		`SELECT morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled
		 FROM NotificationPreference WHERE userId = ?`,
		userID,
	).Scan(&p.EmailMorningEnabled, &p.EmailEveningEnabled, &p.LineMorningEnabled, &p.LineEveningEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return NotificationPreference{}, nil
	}
	return p, err
}

type notificationPreferenceHandlers struct {
	store *notificationPreferenceStore
}

// get は GET /api/notification-preferences。
func (h *notificationPreferenceHandlers) get(w http.ResponseWriter, r *http.Request, s *session) {
	p, err := h.store.find(r.Context(), s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, p)
}
