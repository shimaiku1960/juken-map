package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/notification"
)

// 通知設定の読み取り（JUK-73）と保存（JUK-75）。Node の routes/notification-preferences.ts と、
// services/notification-service.ts の findNotificationPreference・findLineConnection・saveNotificationPreference にあたる。
// 保存（LINE と連携していなければ LINE 通知を ON にできない、も含む）は持ち主の internal/write/notification にある（JUK-154）。

type notificationPreferenceStore struct {
	db *sql.DB
}

// find は自分の通知設定を返す。まだ保存していなければ、全部 false（Node の DEFAULT_PREFERENCE と同じ）。
func (st *notificationPreferenceStore) find(ctx context.Context, userID string) (apischema.NotificationPreference, error) {
	var p apischema.NotificationPreference
	err := st.db.QueryRowContext(ctx,
		`SELECT morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled
		 FROM NotificationPreference WHERE userId = ?`,
		userID,
	).Scan(&p.EmailMorningEnabled, &p.EmailEveningEnabled, &p.LineMorningEnabled, &p.LineEveningEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return apischema.NotificationPreference{}, nil
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

// save は PUT /api/notification-preferences。入力チェックは Zod の notificationPreferenceSchema と同じ（4つとも真偽値）。
func (h *notificationPreferenceHandlers) save(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	in := readObject(body.value())
	input := apischema.NotificationPreference{
		EmailMorningEnabled: in.boolean("emailMorningEnabled"),
		EmailEveningEnabled: in.boolean("emailEveningEnabled"),
		LineMorningEnabled:  in.boolean("lineMorningEnabled"),
		LineEveningEnabled:  in.boolean("lineEveningEnabled"),
	}
	if in.reject(w) {
		return
	}

	err := notification.SavePreference(r.Context(), h.store.db, s.UserID, notification.Preference(input), nowMillis())
	if errors.Is(err, notification.ErrLineNotConnected) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	saved, err := h.store.find(r.Context(), s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
