//go:build dbtest

// 一覧の件数の上限（セキュリティ基準 06 の E2）を本物の DB で確かめる。
//
// 学習記録・予定の一覧は、利用者が期間（?from=&to=）を好きなだけ広く指定できる。その代わり SQL の LIMIT で
// 1000 件（maxLogs・maxPlans）に切り詰め、1回の応答の重さに上限を置いている。上限を超える件数を入れて、
// 超えた分が返らないこと（どちらの端が切れるか）を見る。LIMIT を外すと、どれも 1001 件が返って落ちる。
package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

func TestE2ListLimitsDB(t *testing.T) {
	db := dbtest.Open(t)
	fx := newDBFixture(t, db)
	app := newDBTestApp(db)
	userID := fx.User()
	today := dates.OnTokyo(time.Now())
	const n = maxLogs + 1

	// 学習記録は今日から過去へ、予定は今日から先へ、1日1件ずつ上限＋1件入れる（日別の合計も上限＋1日になる）
	insertDaily(t, db, `INSERT INTO StudyLog (userId, date, minutes, createdAt, updatedAt) VALUES (?, ?, 30, NOW(3), NOW(3))`,
		userID, today, -1, n)
	insertDaily(t, db, `INSERT INTO StudyPlan (userId, date, content, done, createdAt, updatedAt) VALUES (?, ?, '予定', false, NOW(3), NOW(3))`,
		userID, today, 1, maxPlans+1)

	ymd := func(d time.Time) string { return d.Format("2006-01-02") }
	wide := "from=" + ymd(dates.AddDays(today, -2*n)) + "&to=" + ymd(dates.AddDays(today, 2*n))
	tests := []struct {
		name      string
		url       string
		want      int
		wantFirst time.Time
		wantLast  time.Time
	}{
		// 新しい日付から並ぶので、いちばん古い1件が切れる
		{"学習記録", "/api/study-logs?" + wide, maxLogs, today, dates.AddDays(today, -(maxLogs - 1))},
		{"日別の合計", "/api/study-logs/daily?" + wide, maxLogs, today, dates.AddDays(today, -(maxLogs - 1))},
		// 古い日付から並ぶので、いちばん先の1件が切れる
		{"予定", "/api/study-plans?" + wide, maxPlans, today, dates.AddDays(today, maxPlans-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := app.send(http.MethodGet, tt.url, nil, userID)
			if res.Code != http.StatusOK {
				t.Fatalf("status = %d（本文 %s）", res.Code, res.Body)
			}
			var items []struct {
				Date string `json:"date"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &items); err != nil {
				t.Fatal(err)
			}
			if len(items) != tt.want {
				t.Fatalf("件数 = %d, want %d（上限で切り詰める）", len(items), tt.want)
			}
			first, last := items[0].Date[:10], items[len(items)-1].Date[:10]
			if first != ymd(tt.wantFirst) || last != ymd(tt.wantLast) {
				t.Errorf("先頭・末尾 = %s・%s, want %s・%s", first, last, ymd(tt.wantFirst), ymd(tt.wantLast))
			}
		})
	}
}

// insertDaily は from から step 日ずつずらした日付で count 件入れる。行は利用者を消すと CASCADE で消える。
func insertDaily(t *testing.T, db *sql.DB, query, userID string, from time.Time, step, count int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(query)
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	for i := range count {
		if _, err := stmt.Exec(userID, dates.AddDays(from, i*step)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
