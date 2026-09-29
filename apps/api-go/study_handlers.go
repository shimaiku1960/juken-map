package main

import (
	"fmt"
	"net/http"
	"time"
)

// 学習記録・予定の一覧の API（JUK-73）。Node の routes/study-logs.ts・study-plans.ts の GET にあたる。
// 書き込み（POST・PATCH・DELETE）は Node に残っていて、nginx が GET と HEAD だけを Go へ送る
// （infra/nginx/juken-map-go-routes.conf）。

// 期間を省いて呼ばれたときの既定。画面はどれも明示して呼ぶので、これは古いクライアントや
// 手で叩いたときのためのもの。Node と同じ値。
const (
	defaultLogDays        = 90  // 実績：今日を含めて遡る日数
	defaultDailyDays      = 365 // 日別の合計：今日を含めて遡る日数
	defaultPlanPastDays   = 90  // 予定：今日から遡る日数
	defaultPlanFutureDays = 90  // 予定：今日から先の日数
)

type studyHandlers struct {
	store *studyStore
}

// listLogs は GET /api/study-logs。from を省くと直近90日、to を省くと上限なし。
func (h *studyHandlers) listLogs(w http.ResponseWriter, r *http.Request, s *session) {
	q, ok := readDateRangeQuery(w, r)
	if !ok {
		return
	}
	today := dateOnTokyo(time.Now())
	rng, ok := q.resolve(addDays(today, -(defaultLogDays-1)), nil)
	if !ok {
		writeJSON(w, http.StatusOK, []studyLogDTO{})
		return
	}
	logs, err := h.store.listStudyLogs(r.Context(), s.UserID, rng)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-logs: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

// listDaily は GET /api/study-logs/daily。日ごとの合計だけを返す軽い方（連続記録日数が使う）。
func (h *studyHandlers) listDaily(w http.ResponseWriter, r *http.Request, s *session) {
	q, ok := readDateRangeQuery(w, r)
	if !ok {
		return
	}
	today := dateOnTokyo(time.Now())
	rng, ok := q.resolve(addDays(today, -(defaultDailyDays-1)), nil)
	if !ok {
		writeJSON(w, http.StatusOK, []dailyMinutesDTO{})
		return
	}
	daily, err := h.store.listDailyStudyMinutes(r.Context(), s.UserID, rng)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-logs/daily: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, daily)
}

// listPlans は GET /api/study-plans。予定は未来にもあるので、省くと今日の前後90日。
func (h *studyHandlers) listPlans(w http.ResponseWriter, r *http.Request, s *session) {
	q, ok := readDateRangeQuery(w, r)
	if !ok {
		return
	}
	today := dateOnTokyo(time.Now())
	defaultTo := addDays(today, defaultPlanFutureDays)
	rng, ok := q.resolve(addDays(today, -defaultPlanPastDays), &defaultTo)
	if !ok {
		writeJSON(w, http.StatusOK, []studyPlanDTO{})
		return
	}
	plans, err := h.store.listStudyPlans(r.Context(), s.UserID, rng)
	if err != nil {
		internalError(w, r, fmt.Errorf("study-plans: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, plans)
}
