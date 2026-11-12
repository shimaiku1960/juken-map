package notifications

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/notification"
)

// 通知設定の読み取り（JUK-73）と保存（JUK-75）のコントローラー。
// 入力の決まり・読み取り・保存（LINE と連携していなければ LINE 通知を ON にできない、も含む）は
// モデルの internal/model/notification にある（JUK-225）。

type PreferenceHandlers struct {
	db *sql.DB
}

// NewPreferenceHandlers は通知設定の入口を組み立てる。
func NewPreferenceHandlers(db *sql.DB) *PreferenceHandlers {
	return &PreferenceHandlers{db: db}
}

// preferenceView は通知の設定を画面に返す形にする。項目は同じ並びなので型の変換だけ。
func preferenceView(p notification.Preference) apischema.NotificationPreference {
	return apischema.NotificationPreference(p)
}

// Get は GET /api/notification-preferences。
func (h *PreferenceHandlers) Get(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	p, err := notification.FindPreference(r.Context(), h.db, s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, preferenceView(p))
}

// Save は PUT /api/notification-preferences。
func (h *PreferenceHandlers) Save(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	saved, err := notification.SavePreference(r.Context(), h.db, s.UserID, body.Value(), dates.NowMillis())
	if httpx.RejectIssue(w, err) {
		return
	}
	if errors.Is(err, notification.ErrLineNotConnected) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, preferenceView(saved))
}
