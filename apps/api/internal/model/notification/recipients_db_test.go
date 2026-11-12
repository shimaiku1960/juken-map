//go:build dbtest

package notification

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 毎日の通知を受け取る人の読み取りを、本物の MySQL で確かめる。
// DB はほかのテストと共有なので、作った利用者の分だけを見る。
func TestRecipientsDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	fx := dbtest.Fixture{T: t, DB: db}
	now := time.Now()
	day := time.Date(2027, 2, 20, 0, 0, 0, 0, time.UTC)
	pref := func(user string, morning, evening, lineMorning bool) {
		fx.Exec(`INSERT INTO NotificationPreference
			(userId, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, false, ?, ?)`, user, morning, evening, lineMorning, now, now)
	}
	log := func(user string, date time.Time, minutes int) {
		fx.Exec(`INSERT INTO StudyLog (userId, date, minutes, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)`,
			user, date, minutes, now, now)
	}

	morning := fx.User() // 朝のメール。予定2つと、その日の実績2つ・前の日の実績1つ
	pref(morning, true, false, false)
	fx.StudyPlan(morning)
	fx.StudyPlan(morning)
	log(morning, day, 20)
	log(morning, day, 40)
	log(morning, day.AddDate(0, 0, -1), 90)

	lineOnly := fx.User() // 朝の LINE だけ（連携あり）
	pref(lineOnly, false, false, true)
	fx.LineConnection(lineOnly)

	evening := fx.User() // 夜だけなので、朝には入らない
	pref(evening, false, true, false)

	got, err := Recipients(ctx, db, "morning", day, day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	mine := map[string]Recipient{}
	for _, r := range got {
		if r.ID == morning || r.ID == lineOnly || r.ID == evening {
			mine[r.ID] = r
		}
	}
	if len(mine) != 2 {
		t.Fatalf("受け取る人 = %d 人、want 2（夜だけの人は入らない）: %+v", len(mine), mine)
	}
	m := mine[morning]
	if !m.Morning || m.LineUserID != nil || len(m.Plans) != 2 || !slices.Equal(m.LogMinutes, []int64{20, 40}) {
		t.Errorf("朝のメールの人: %+v", m)
	}
	l := mine[lineOnly]
	if !l.LineMorn || l.LineUserID == nil || len(l.Plans) != 0 || len(l.LogMinutes) != 0 {
		t.Errorf("朝の LINE の人: %+v", l)
	}

	if _, err := Recipients(ctx, db, "noon", day, day); err == nil {
		t.Error("知らない時間帯を受け付けた")
	}
}
