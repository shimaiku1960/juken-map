//go:build dbtest

package studyrecord

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/opt"
)

// 学習記録の操作を、本物の MySQL で確かめる。画面の形（入力チェックの 400・応答の形）は
// internal/feature/study の study_*_writes のテストにある。

// textbookWithSettings は逆算設定（単位 page・総量 300）を持つ参考書を作る。
func textbookWithSettings(fx dbtest.Fixture, userID string) int64 {
	now := time.Now()
	return fx.Insert("INSERT INTO Textbook (userId, name, rangeUnit, totalAmount, createdAt, updatedAt) VALUES (?, ?, 'page', 300, ?, ?)",
		userID, "参考書", now, now)
}

// firstStudyLogAt は利用者の初回記録の日時を、Date#toISOString の形で返す（無ければ nil）。
func firstStudyLogAt(t *testing.T, db *sql.DB, userID string) *string {
	t.Helper()
	var at *string
	if err := db.QueryRow("SELECT firstStudyLogAt FROM `user` WHERE id = ?", userID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if at != nil {
		*at = database.ISOFromDatetime(*at)
	}
	return at
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ptr[T any](v T) *T { return &v }

func TestStudyRecordDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	t.Run("初回記録の印は最初の1件でだけ付く", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		first, err := CreateLog(ctx, db, user, LogInput{Date: day, Minutes: 10}, now)
		if err != nil || !first.IsFirstStudyLog {
			t.Fatalf("1件目: %+v, %v", first, err)
		}
		second, err := CreateLog(ctx, db, user, LogInput{Date: day, Minutes: 20}, now.Add(time.Hour))
		if err != nil || second.IsFirstStudyLog {
			t.Fatalf("2件目: %+v, %v", second, err)
		}
		if at, want := firstStudyLogAt(t, db, user), first.CreatedAt; at == nil || *at != want {
			t.Errorf("firstStudyLogAt = %v, want %s", at, want)
		}
	})

	t.Run("他人の参考書や範囲の誤りでは、実績も初回の印も残らない", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user, other := fx.User(), fx.User()
		_, err := CreateLog(ctx, db, user, LogInput{Date: day, Minutes: 10,
			TextbookID: opt.Field[int64]{Present: true, Value: ptr(fx.Textbook(other))}}, now)
		if !errors.Is(err, ErrTextbookNotOwned) {
			t.Errorf("他人の参考書: %v", err)
		}
		_, err = CreateLog(ctx, db, user, LogInput{Date: day, Minutes: 10,
			TextbookID: opt.Field[int64]{Present: true, Value: ptr(textbookWithSettings(fx, user))},
			RangeStart: opt.Field[int64]{Present: true, Value: ptr(int64(1))},
			RangeEnd:   opt.Field[int64]{Present: true, Value: ptr(int64(301))},
			RangeUnit:  opt.Field[string]{Present: true, Value: ptr("page")}}, now)
		var rangeErr *RangeError
		if !errors.As(err, &rangeErr) {
			t.Errorf("総量を超える範囲: %v", err)
		}
		if n := count(t, db, "SELECT COUNT(*) FROM StudyLog WHERE userId = ?", user); n != 0 {
			t.Errorf("実績が %d 件残った", n)
		}
		if at := firstStudyLogAt(t, db, user); at != nil {
			t.Errorf("初回の印が残った: %s", *at)
		}
	})

	t.Run("予定の完了は、実績・予定の完了・初回の印を一緒に確定させる", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		planID := fx.StudyPlan(user)
		done, err := CompletePlan(ctx, db, user, planID, Completion{Minutes: 30}, now)
		if err != nil {
			t.Fatal(err)
		}
		if !done.Plan.Done || done.Log.StudyPlanID == nil || *done.Log.StudyPlanID != planID || !done.IsFirstStudyLog {
			t.Errorf("完了の結果: %+v", done)
		}
		if done.Log.Date != "2027-02-20T00:00:00.000Z" {
			t.Errorf("実績の日付は予定の日付: %s", done.Log.Date)
		}
		if _, err := CompletePlan(ctx, db, user, planID, Completion{Minutes: 30}, now); !errors.Is(err, ErrAlreadyCompleted) {
			t.Errorf("2回目の完了: %v", err)
		}
		if _, err := UpdatePlan(ctx, db, user, planID, PlanPatch{Done: opt.Field[bool]{Present: true, Value: ptr(false)}}, now); !errors.Is(err, ErrPlanHasLog) {
			t.Errorf("実績のある予定を未完了に戻す: %v", err)
		}
	})

	t.Run("範囲の誤りで完了できなければ、何も残らない", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		planID := fx.StudyPlan(user)
		fx.Exec("UPDATE StudyPlan SET textbookId = ? WHERE id = ?", textbookWithSettings(fx, user), planID)
		_, err := CompletePlan(ctx, db, user, planID, Completion{Minutes: 30,
			RangeStart: opt.Field[int64]{Present: true, Value: ptr(int64(1))},
			RangeEnd:   opt.Field[int64]{Present: true, Value: ptr(int64(10))},
			RangeUnit:  opt.Field[string]{Present: true, Value: ptr("question")}}, now)
		var rangeErr *RangeError
		if !errors.As(err, &rangeErr) {
			t.Fatalf("単位が違う範囲: %v", err)
		}
		if n := count(t, db, "SELECT COUNT(*) FROM StudyLog WHERE userId = ?", user); n != 0 {
			t.Errorf("実績が %d 件残った", n)
		}
		if n := count(t, db, "SELECT COUNT(*) FROM StudyPlan WHERE id = ? AND done", planID); n != 0 {
			t.Error("予定が完了になった")
		}
		if at := firstStudyLogAt(t, db, user); at != nil {
			t.Errorf("初回の印が残った: %s", *at)
		}
	})

	t.Run("予定から作った実績は、日付・科目・参考書を変えない", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user := fx.User()
		done, err := CompletePlan(ctx, db, user, fx.StudyPlan(user), Completion{Minutes: 30}, now)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := UpdateLog(ctx, db, user, done.Log.ID, LogInput{Date: day, Minutes: 45,
			Subject:    opt.Field[string]{Present: true, Value: ptr("english")},
			TextbookID: opt.Field[int64]{Present: true, Value: ptr(fx.Textbook(user))}}, now)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Minutes != 45 || updated.Date != done.Log.Date || updated.Subject != nil || updated.TextbookID != nil {
			t.Errorf("変えた後: %+v", updated)
		}
	})

	t.Run("無い・他人の行は ErrNotFound", func(t *testing.T) {
		fx := dbtest.Fixture{T: t, DB: db}
		user, other := fx.User(), fx.User()
		logID, planID := fx.StudyLog(other), fx.StudyPlan(other)
		for name, err := range map[string]error{
			"実績の変更": func() error {
				_, err := UpdateLog(ctx, db, user, logID, LogInput{Date: day, Minutes: 1}, now)
				return err
			}(),
			"実績の削除": DeleteLog(ctx, db, user, logID),
			"予定の変更": func() error { _, err := UpdatePlan(ctx, db, user, planID, PlanPatch{}, now); return err }(),
			"予定の削除": DeletePlan(ctx, db, user, planID),
			"予定の完了": func() error { _, err := CompletePlan(ctx, db, user, planID, Completion{Minutes: 1}, now); return err }(),
		} {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}
