package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
)

// 通知設定の読み取り（JUK-73）と保存（JUK-75）。Node の routes/notification-preferences.ts と、
// services/notification-service.ts の findNotificationPreference・findLineConnection・saveNotificationPreference にあたる。

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

// hasLineConnection は LINE と連携済みか。
func (st *notificationPreferenceStore) hasLineConnection(ctx context.Context, userID string) (bool, error) {
	var id int64
	err := st.db.QueryRowContext(ctx, "SELECT id FROM LineConnection WHERE userId = ?", userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// save は通知設定を保存し（無ければ作る）、保存した後の値を返す。
// 「無ければ INSERT、あれば UPDATE」を1文で行う（userId に UNIQUE 制約がある）。SQL は Node と同じ。
// new は「INSERT しようとした行」の別名で、createdAt は更新しない。
func (st *notificationPreferenceStore) save(ctx context.Context, userID string, p NotificationPreference) (NotificationPreference, error) {
	now := nowMillis()
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO NotificationPreference
		   (userId, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled,
		    createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?) AS new
		 ON DUPLICATE KEY UPDATE
		   morningEnabled = new.morningEnabled,
		   eveningEnabled = new.eveningEnabled,
		   lineMorningEnabled = new.lineMorningEnabled,
		   lineEveningEnabled = new.lineEveningEnabled,
		   updatedAt = new.updatedAt`,
		userID, p.EmailMorningEnabled, p.EmailEveningEnabled, p.LineMorningEnabled, p.LineEveningEnabled, now, now,
	); err != nil {
		return NotificationPreference{}, err
	}
	return st.find(ctx, userID)
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
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	in := readObject(body)
	input := NotificationPreference{
		EmailMorningEnabled: in.boolean("emailMorningEnabled"),
		EmailEveningEnabled: in.boolean("emailEveningEnabled"),
		LineMorningEnabled:  in.boolean("lineMorningEnabled"),
		LineEveningEnabled:  in.boolean("lineEveningEnabled"),
	}
	if in.reject(w) {
		return
	}

	if input.LineMorningEnabled || input.LineEveningEnabled {
		connected, err := h.store.hasLineConnection(r.Context(), s.UserID)
		if err != nil {
			internalError(w, r, fmt.Errorf("notification-preferences: %w", err))
			return
		}
		if !connected {
			writeError(w, http.StatusBadRequest, "LINEと連携してからLINE通知を選択してください")
			return
		}
	}

	saved, err := h.store.save(r.Context(), s.UserID, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("notification-preferences: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, saved)
}
