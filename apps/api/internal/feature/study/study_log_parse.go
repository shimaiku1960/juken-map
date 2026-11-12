package study

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/aiparse"
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
)

// AIクイック記録（JUK-180）。一文を Claude に読ませ、実績の項目に分けて返す。記録はしない
// （画面がフォームに入れ、利用者が確かめてから POST /api/study-logs で記録する）。
//
// 一文の決まりと、Claude の出力を記録の決まりに照らして確かめるのはモデル（internal/model/studyrecord の quick.go）。
// ここは回数の上限・待つ長さ・Claude の呼び出しと、HTTP の応答を受け持つ。

const (
	// parseUserDaily は1人あたり1日（日本時間）の回数。記録は1日に数件なので、試し直しを含めても足りる。
	parseUserDaily = 20
	// parseTotalDaily は全員を合わせた1日の回数。アカウントを多く作って叩かれても、1日の費用を
	// 数十円（1回0.01円台）に抑える。最後の守りは Anthropic の前払い残高（自動チャージ無し）。
	parseTotalDaily = 2000
	// parseTextbookLimit はモデルに見せる参考書の数。多い人でも入力のトークン（費用）が膨らまないように。
	parseTextbookLimit = 100
	// parseTimeout は Claude の応答を待つ長さ。ふだんは2秒ほどで返る（2026-10-11 実測）が、スキーマを
	// 変えた直後の初回だけは、API 側が文法を作るので10秒ほどかかる。
	parseTimeout = 20 * time.Second
)

const (
	parseUnavailableMessage = "AI入力はこの環境では使えません"
	parseFailedMessage      = "AIで読み取れませんでした。少し待ってからもう一度お試しいただくか、手で入力してください"
	parseUserLimitMessage   = "今日のAI入力の回数の上限に達しました。手で入力してください"
	parseTotalLimitMessage  = "今日はAI入力が混み合っているため使えません。手で入力してください"
)

// ParseHandler は POST /api/study-logs/parse。
type ParseHandler struct {
	// parser は Claude。鍵が設定されていない環境では nil で、503 を返す。
	parser    aiparse.Parser
	textbooks func(ctx context.Context, userID string) ([]aiparse.Textbook, error)
	now       func() time.Time
	quota     *dailyQuota
}

func NewParseHandler(db *sql.DB, parser aiparse.Parser) *ParseHandler {
	return &ParseHandler{
		parser: parser,
		textbooks: func(ctx context.Context, userID string) ([]aiparse.Textbook, error) {
			refs, err := studyrecord.RecentTextbooks(ctx, db, userID, parseTextbookLimit)
			books := make([]aiparse.Textbook, len(refs))
			for i, ref := range refs {
				books[i] = aiparse.Textbook(ref)
			}
			return books, err
		},
		now:   time.Now,
		quota: &dailyQuota{userMax: parseUserDaily, totalMax: parseTotalDaily},
	}
}

func (h *ParseHandler) Serve(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	if h.parser == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, parseUnavailableMessage)
		return
	}
	text, err := studyrecord.ReadQuickText(body.Value())
	if httpx.RejectIssue(w, err) {
		return
	}

	today := dates.YMD(dates.OnTokyo(h.now()))
	// 回数は呼ぶ前に数える（失敗も1回）。後で数えると、同時に来た分が全部「まだ上限前」を見て通る。
	switch h.quota.take(today, s.UserID) {
	case quotaUserFull:
		httpx.WriteError(w, http.StatusTooManyRequests, parseUserLimitMessage)
		return
	case quotaTotalFull:
		slog.WarnContext(r.Context(), "study-logs parse: daily total limit reached", "limit", parseTotalDaily)
		httpx.WriteError(w, http.StatusTooManyRequests, parseTotalLimitMessage)
		return
	}

	books, err := h.textbooks(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("study-logs parse textbooks: %w", err))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), parseTimeout)
	defer cancel()
	got, err := h.parser.Parse(ctx, aiparse.Input{Text: text, Today: today, Textbooks: books})
	if err != nil {
		slog.ErrorContext(r.Context(), "study-logs parse failed", "err", err.Error(), "statusCode", http.StatusBadGateway)
		httpx.WriteError(w, http.StatusBadGateway, parseFailedMessage)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, checkParseResult(got, books, today))
}

// checkParseResult は Claude の出力のうち、記録の決まりに合う項目だけを残して応答の形にする。
func checkParseResult(got aiparse.Result, books []aiparse.Textbook, today string) apischema.StudyLogParseResult {
	refs := make([]studyrecord.TextbookRef, len(books))
	for i, b := range books {
		refs[i] = studyrecord.TextbookRef(b)
	}
	d := studyrecord.CheckDraft(studyrecord.Draft(got), refs, today)
	out := apischema.StudyLogParseResult{Date: d.Date, Minutes: d.Minutes, TextbookID: d.TextbookID}
	if d.Subject != nil {
		subject := apischema.StudyLogParseResultSubject(*d.Subject)
		out.Subject = &subject
	}
	return out
}

// dailyQuota は1日（日本時間）の回数を、1人ごとと全体で数える。
//
// 数はこのプロセスのメモリに置く（httpx の利用者単位の回数制限と同じ考え）。再起動・デプロイで数が消え、
// その日のうちに上限までもう一度使えるが、1回の費用が小さいので許す。DB に置くと表を足すことになり、
// その割に守れるものが増えない。日付が変わったら全部捨てる。
type dailyQuota struct {
	userMax, totalMax int

	mu    sync.Mutex
	day   string
	users map[string]int
	total int
}

type quotaResult int

const (
	quotaOK quotaResult = iota
	quotaUserFull
	quotaTotalFull
)

func (q *dailyQuota) take(day, userID string) quotaResult {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.day != day {
		q.day, q.users, q.total = day, map[string]int{}, 0
	}
	if q.users[userID] >= q.userMax {
		return quotaUserFull
	}
	if q.total >= q.totalMax {
		return quotaTotalFull
	}
	q.users[userID]++
	q.total++
	return quotaOK
}
