package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
)

func strp(s string) *string { return &s }

func TestTokyoDateRange(t *testing.T) {
	// UTC の日付ではなく日本時間の一日を返す（Node の dailyNotification.test.ts と同じ）
	day := tokyoDateRange(time.Date(2026, 8, 30, 16, 0, 0, 0, time.UTC))
	if day.date != "2026-08-31" ||
		!day.start.Equal(time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)) ||
		!day.end.Equal(time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("tokyoDateRange = %+v", day)
	}
}

func TestBuildDailyNotification(t *testing.T) {
	t.Run("朝は今日の予定を具体的に伝える", func(t *testing.T) {
		m := buildDailyNotification(apischema.NotificationSlotMorning, "育朗",
			[]planSummary{{Subject: strp("english"), TextbookName: strp("英単語帳")}}, nil)
		for _, want := range []string{"今日の予定は1件", "英単語帳"} {
			if !strings.Contains(m.HTML, want) {
				t.Errorf("HTML に %q が無い: %s", want, m.HTML)
			}
		}
		if !strings.Contains(m.Subject, "今日の学習予定") {
			t.Errorf("Subject = %q", m.Subject)
		}
	})
	t.Run("夜は学習時間と予定達成数を伝える", func(t *testing.T) {
		m := buildDailyNotification(apischema.NotificationSlotEvening, "育朗",
			[]planSummary{{Done: true, Content: strp("過去問")}, {Content: strp("復習")}}, []int64{25, 35})
		for _, want := range []string{"60分", "予定2件中1件"} {
			if !strings.Contains(m.HTML, want) {
				t.Errorf("HTML に %q が無い: %s", want, m.HTML)
			}
		}
	})
	t.Run("実績0でも責めない文面にし、名前は HTML で無害にする", func(t *testing.T) {
		m := buildDailyNotification(apischema.NotificationSlotEvening, "<ユーザー>", nil, nil)
		for _, want := range []string{"短い時間でも、記録から再開できます", "&lt;ユーザー&gt;"} {
			if !strings.Contains(m.HTML, want) {
				t.Errorf("HTML に %q が無い: %s", want, m.HTML)
			}
		}
		// テキストの本文はそのまま
		if !strings.HasPrefix(m.Text, "<ユーザー>さん") {
			t.Errorf("Text = %q", m.Text)
		}
	})
	t.Run("予定は先頭の5件まで並べ、見出しは参考書名・内容・科目の順に使う", func(t *testing.T) {
		plans := []planSummary{
			{Content: strp("内容"), TextbookName: strp("参考書")}, {Content: strp("内容2"), Subject: strp("math")},
			{Subject: strp("math")}, {}, {Content: strp("5件目")}, {Content: strp("6件目")},
		}
		m := buildDailyNotification(apischema.NotificationSlotMorning, "a", plans, nil)
		if !strings.Contains(m.Text, "\n・参考書\n・内容2\n・math\n・学習予定\n・5件目\n\n") || strings.Contains(m.Text, "6件目") {
			t.Errorf("Text = %q", m.Text)
		}
		if !strings.Contains(m.HTML, "今日の予定は6件です。") {
			t.Errorf("HTML = %q", m.HTML)
		}
	})
}

// fakeStore は DB の代わり。配信記録を (userId, channel) で持つ。
type fakeStore struct {
	mu         sync.Mutex
	recipients []recipient
	marked     map[string]int64
	nextID     int64
	unmarked   []int64
	markErr    error
}

func newFakeStore(rs ...recipient) *fakeStore {
	return &fakeStore{recipients: rs, marked: map[string]int64{}}
}

func (s *fakeStore) findRecipients(context.Context, apischema.NotificationSlot, time.Time, time.Time) ([]recipient, error) {
	return s.recipients, nil
}

func (s *fakeStore) markDelivery(_ context.Context, userID string, _ time.Time, _ apischema.NotificationSlot, ch deliveryChannel) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.markErr != nil {
		return 0, false, s.markErr
	}
	key := userID + "/" + string(ch)
	if _, ok := s.marked[key]; ok {
		return 0, true, nil
	}
	s.nextID++
	s.marked[key] = s.nextID
	return s.nextID, false, nil
}

func (s *fakeStore) unmarkDelivery(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.marked {
		if v == id {
			delete(s.marked, k)
		}
	}
	s.unmarked = append(s.unmarked, id)
	return nil
}

// fakeMessenger は送信先の代わり。送った宛先と、同時に送っていた数の最大を残す。
type fakeMessenger struct {
	mu       sync.Mutex
	emails   []string
	lines    []string
	emailAt  []time.Time
	failTo   string
	delay    time.Duration
	inFlight atomic.Int64
	maxIn    atomic.Int64
}

func (m *fakeMessenger) enter() func() {
	n := m.inFlight.Add(1)
	for {
		cur := m.maxIn.Load()
		if n <= cur || m.maxIn.CompareAndSwap(cur, n) {
			break
		}
	}
	time.Sleep(m.delay)
	return func() { m.inFlight.Add(-1) }
}

func (m *fakeMessenger) sendEmail(_ context.Context, to string, _ dailyMessage) error {
	defer m.enter()()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emails = append(m.emails, to)
	m.emailAt = append(m.emailAt, time.Now())
	if to == m.failTo {
		return errors.New("unavailable")
	}
	return nil
}

func (m *fakeMessenger) pushLine(_ context.Context, lineUserID, _ string) error {
	defer m.enter()()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = append(m.lines, lineUserID)
	return nil
}

func testNotifier(store notificationStore, m messenger) *dailyNotifier {
	n := newDailyNotifier(store, m)
	n.emailPace = newPacer(0)
	return n
}

var testNow = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) // 日本時間 8/30 21:00

func TestDailyNotifierSend(t *testing.T) {
	t.Run("その時間帯がオンの経路だけに送り、LINE は未連携なら送らない", func(t *testing.T) {
		store := newFakeStore(
			recipient{ID: "both", Email: strp("both@example.com"), Morning: true, LineMorn: true, LineUserID: strp("U1")},
			recipient{ID: "evening-only", Email: strp("e@example.com"), Evening: true},
			recipient{ID: "line-unlinked", Email: strp("l@example.com"), LineMorn: true},
		)
		m := &fakeMessenger{}
		got, err := testNotifier(store, m).send(context.Background(), apischema.NotificationSlotMorning, testNow)
		if err != nil {
			t.Fatal(err)
		}
		want := apischema.NotificationSummary{Date: "2026-08-30", Slot: apischema.NotificationSlotMorning, Eligible: 2, Sent: 2}
		if got != want {
			t.Errorf("summary = %+v, want %+v", got, want)
		}
		if len(m.emails) != 1 || m.emails[0] != "both@example.com" || len(m.lines) != 1 || m.lines[0] != "U1" {
			t.Errorf("emails = %v, lines = %v", m.emails, m.lines)
		}
	})

	t.Run("同じ日・同じ時間帯の2回目は送らない", func(t *testing.T) {
		store := newFakeStore(recipient{ID: "u", Email: strp("u@example.com"), Morning: true})
		m := &fakeMessenger{}
		n := testNotifier(store, m)
		if _, err := n.send(context.Background(), apischema.NotificationSlotMorning, testNow); err != nil {
			t.Fatal(err)
		}
		second, _ := n.send(context.Background(), apischema.NotificationSlotMorning, testNow)
		if second.Skipped != 1 || second.Sent != 0 || len(m.emails) != 1 {
			t.Errorf("second = %+v, emails = %v", second, m.emails)
		}
	})

	t.Run("送信に失敗したら配信記録を消し、次の実行で再び送れる", func(t *testing.T) {
		store := newFakeStore(recipient{ID: "u", Email: strp("u@example.com"), Morning: true})
		m := &fakeMessenger{failTo: "u@example.com"}
		n := testNotifier(store, m)
		failed, err := n.send(context.Background(), apischema.NotificationSlotMorning, testNow)
		if err != nil || failed.Failed != 1 || len(store.unmarked) != 1 || len(store.marked) != 0 {
			t.Fatalf("failed = %+v, err = %v, unmarked = %v", failed, err, store.unmarked)
		}
		m.failTo = ""
		again, _ := n.send(context.Background(), apischema.NotificationSlotMorning, testNow)
		if again.Sent != 1 || len(m.emails) != 2 {
			t.Errorf("again = %+v, emails = %v", again, m.emails)
		}
	})

	t.Run("配信記録を書けなければ止めてエラーを返す", func(t *testing.T) {
		store := newFakeStore(recipient{ID: "u", Email: strp("u@example.com"), Morning: true})
		store.markErr = errors.New("db down")
		m := &fakeMessenger{}
		if _, err := testNotifier(store, m).send(context.Background(), apischema.NotificationSlotMorning, testNow); err == nil {
			t.Fatal("エラーにならなかった")
		}
		if len(m.emails) != 0 {
			t.Errorf("送ってしまった: %v", m.emails)
		}
	})

	t.Run("同時に送る数は workers まで。並べた分だけ早く終わる", func(t *testing.T) {
		var rs []recipient
		for i := range 20 {
			rs = append(rs, recipient{ID: string(rune('a' + i)), LineEven: true, LineUserID: strp("U")})
		}
		m := &fakeMessenger{delay: 20 * time.Millisecond}
		n := testNotifier(newFakeStore(rs...), m)
		started := time.Now()
		got, err := n.send(context.Background(), apischema.NotificationSlotEvening, testNow)
		elapsed := time.Since(started)
		if err != nil || got.Sent != 20 {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
		if max := m.maxIn.Load(); max > notificationWorkers || max < 2 {
			t.Errorf("同時に送っていた数の最大 = %d, want 2〜%d", max, notificationWorkers)
		}
		// 1件ずつなら 20 × 20ms = 400ms。5本並べれば 80ms 前後
		if elapsed > 300*time.Millisecond {
			t.Errorf("elapsed = %v（並べて送れていない）", elapsed)
		}
	})

	t.Run("メールは emailPace の間隔を空けて送る", func(t *testing.T) {
		var rs []recipient
		for i := range 4 {
			rs = append(rs, recipient{ID: string(rune('a' + i)), Email: strp(string(rune('a'+i)) + "@example.com"), Morning: true})
		}
		m := &fakeMessenger{}
		n := testNotifier(newFakeStore(rs...), m)
		n.emailPace = newPacer(30 * time.Millisecond)
		if _, err := n.send(context.Background(), apischema.NotificationSlotMorning, testNow); err != nil {
			t.Fatal(err)
		}
		// 並べて送っても、送った時刻の差は間隔より大きい（タイマーの誤差ぶん少し甘く見る）
		for i := 1; i < len(m.emailAt); i++ {
			if gap := m.emailAt[i].Sub(m.emailAt[i-1]); gap < 25*time.Millisecond {
				t.Errorf("%d通目との間隔 = %v", i+1, gap)
			}
		}
	})
}

func TestCronHandler(t *testing.T) {
	rt := newRouter(fakeSessions(nil))
	store := newFakeStore(recipient{ID: "u", Email: strp("u@example.com"), Morning: true})
	h := &cronHandler{notifier: testNotifier(store, &fakeMessenger{}), now: func() time.Time { return testNow }}
	rt.job("POST /api/cron/daily-study-notifications", "secret", h.dailyNotifications)

	tests := []struct {
		name, auth, body string
		wantStatus       int
		wantBody         string
	}{
		{"トークンが無ければ 401", "", `{"slot":"morning"}`, 401, `{"error":"Unauthorized"}`},
		{"トークンが違えば 401", "Bearer wrong", `{"slot":"morning"}`, 401, `{"error":"Unauthorized"}`},
		{"slot が不正なら 400", "Bearer secret", `{"slot":"noon"}`, 400, `{"error":"Invalid slot"}`},
		{"JSON でなければ 400", "Bearer secret", `slot=morning`, 400, `{"error":"Invalid slot"}`},
		{"送って件数を返す", "Bearer secret", `{"slot":"morning"}`, 200,
			`{"date":"2026-08-30","slot":"morning","eligible":1,"sent":1,"skipped":0,"failed":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/cron/daily-study-notifications", strings.NewReader(tt.body))
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			res := httptest.NewRecorder()
			rt.ServeHTTP(res, req)
			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", res.Code, tt.wantStatus, res.Body)
			}
			assertJSONEqual(t, res.Body.String(), tt.wantBody)
		})
	}
}

func TestJobWithoutSecret(t *testing.T) {
	// 秘密値を設定し忘れたら、何を送っても 401（誰でも呼べる状態にしない）
	rt := newRouter(fakeSessions(nil))
	rt.job("POST /api/job", "", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	for _, auth := range []string{"", "Bearer ", "Bearer undefined"} {
		req := httptest.NewRequest("POST", "/api/job", nil)
		req.Header.Set("Authorization", auth)
		res := httptest.NewRecorder()
		rt.ServeHTTP(res, req)
		if res.Code != http.StatusUnauthorized {
			t.Errorf("Authorization %q: status = %d, want 401", auth, res.Code)
		}
	}
}
