package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
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
func (h *notificationPreferenceHandlers) get(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	p, err := h.store.find(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// save は PUT /api/notification-preferences。入力チェックは Zod の notificationPreferenceSchema と同じ（4つとも真偽値）。
func (h *notificationPreferenceHandlers) save(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	in := httpx.ReadObject(body.Value())
	input := apischema.NotificationPreference{
		EmailMorningEnabled: in.Boolean("emailMorningEnabled"),
		EmailEveningEnabled: in.Boolean("emailEveningEnabled"),
		LineMorningEnabled:  in.Boolean("lineMorningEnabled"),
		LineEveningEnabled:  in.Boolean("lineEveningEnabled"),
	}
	if in.Reject(w) {
		return
	}

	err := notification.SavePreference(r.Context(), h.store.db, s.UserID, notification.Preference(input), dates.NowMillis())
	if errors.Is(err, notification.ErrLineNotConnected) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	saved, err := h.store.find(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, saved)
}
