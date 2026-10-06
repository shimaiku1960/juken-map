//go:build dbtest

// 利用者の持ち物を変えるストアの関数が、渡された userID の人の行しか変えないことを確かめる（JUK-54）。
//
// TestA3OwnershipDB は入口（ハンドラーが先に持ち主を確かめる）を通して確かめる。こちらはその確かめを通らずに
// ストアを直接呼び、確かめを書き忘れたルートがあっても SQL の WHERE userId = ? で他人の行が守られることを見る。
package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

func TestA3StoreScopedByUserDB(t *testing.T) {
	db := dbtest.Open(t)
	facultyID, otherFacultyID := newDBFixture(t, db).University(), newDBFixture(t, db).University()
	ctx := context.Background()
	goals := &goalStore{db: db}
	textbooks := &textbookStore{db: db}
	logs := &studyLogWriteStore{db: db}
	plans := &studyPlanWriteStore{db: db}
	note := "書き換え"

	cases := []struct {
		name  string
		table string
		// write は caller の userID で holder の行（id）を書き換えようとする。エラーは見ない（DB が変わらないことだけを見る）。
		write func(fx dbFixture, caller, holder string, id int64)
		seed  func(fx dbFixture, holder string) int64
	}{
		{
			name: "志望校の学部の差し替え", table: "FinalGoal",
			seed: func(fx dbFixture, holder string) int64 { return fx.FinalGoal(holder, facultyID) },
			write: func(_ dbFixture, caller, _ string, id int64) {
				_, _ = goals.replaceFaculty(ctx, caller, id, optional[int64]{present: true, value: &otherFacultyID})
			},
		},
		{
			name: "志望校のメモ", table: "FinalGoal",
			seed: func(fx dbFixture, holder string) int64 { return fx.FinalGoal(holder, facultyID) },
			write: func(_ dbFixture, caller, _ string, id int64) {
				_ = goals.applyPatch(ctx, caller, id, goalPatch{note: optional[string]{present: true, value: &note}})
			},
		},
		{
			name: "志望校の削除", table: "FinalGoal",
			seed:  func(fx dbFixture, holder string) int64 { return fx.FinalGoal(holder, facultyID) },
			write: func(_ dbFixture, caller, _ string, id int64) { _ = goals.deleteGoal(ctx, caller, id) },
		},
		{
			name: "参考書の逆算設定", table: "Textbook",
			seed: func(fx dbFixture, holder string) int64 { return fx.Textbook(holder) },
			write: func(_ dbFixture, caller, _ string, id int64) {
				_, _ = textbooks.updateProgress(ctx, caller, id, textbookProgress{subject: optional[string]{present: true, value: &note}})
			},
		},
		{
			name: "実績の書き換え", table: "StudyLog",
			seed: func(fx dbFixture, holder string) int64 { return fx.StudyLog(holder) },
			write: func(_ dbFixture, caller, holder string, id int64) {
				current, err := findStudyLog(ctx, db, id, holder)
				if err != nil || current == nil {
					t.Fatalf("実績が読めない: %v", err)
				}
				_, _ = logs.update(ctx, caller, current, studyLogInput{date: "2027-01-01", minutes: 99})
			},
		},
		{
			name: "実績の削除", table: "StudyLog",
			seed:  func(fx dbFixture, holder string) int64 { return fx.StudyLog(holder) },
			write: func(_ dbFixture, caller, _ string, id int64) { _ = logs.delete(ctx, caller, id) },
		},
		{
			name: "予定の書き換え", table: "StudyPlan",
			seed: func(fx dbFixture, holder string) int64 { return fx.StudyPlan(holder) },
			write: func(_ dbFixture, caller, _ string, id int64) {
				_, _ = plans.update(ctx, caller, id, studyPlanUpdate{content: optional[string]{present: true, value: &note}})
			},
		},
		{
			name: "予定の削除", table: "StudyPlan",
			seed:  func(fx dbFixture, holder string) int64 { return fx.StudyPlan(holder) },
			write: func(_ dbFixture, caller, _ string, id int64) { _ = plans.delete(ctx, caller, id) },
		},
		{
			name: "予定の完了", table: "StudyPlan",
			seed: func(fx dbFixture, holder string) int64 { return fx.StudyPlan(holder) },
			write: func(_ dbFixture, caller, holder string, id int64) {
				plan, err := findStudyPlan(ctx, db, id, holder)
				if err != nil || plan == nil {
					t.Fatalf("予定が読めない: %v", err)
				}
				_, _ = plans.complete(ctx, caller, *plan, 30, nil, nil, nil, nil)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name+"：他人の行は変わらない", func(t *testing.T) {
			fx := newDBFixture(t, db)
			caller, holder := fx.User(), fx.User()
			id := c.seed(fx, holder)
			// #nosec G202 -- table はこのテストに書いた固定の名前だけ
			read := func(userID string) []string { return fx.Rows("SELECT * FROM "+c.table+" WHERE userId = ?", userID) }
			before := read(holder)

			c.write(fx, caller, holder, id)

			if after := read(holder); !reflect.DeepEqual(after, before) {
				t.Errorf("他人の行が変わった\nbefore %v\nafter  %v", before, after)
			}
			// 予定の完了は実績を作るので、caller の側にも何も残っていないことを見る。
			if logs := fx.Rows("SELECT * FROM StudyLog WHERE userId = ?", caller); len(logs) != 0 {
				t.Errorf("caller に実績が残った: %v", logs)
			}
		})

		t.Run(c.name+"：自分の行なら変わる", func(t *testing.T) {
			fx := newDBFixture(t, db)
			owner := fx.User()
			id := c.seed(fx, owner)
			// #nosec G202 -- table はこのテストに書いた固定の名前だけ
			read := func() []string { return fx.Rows("SELECT * FROM "+c.table+" WHERE userId = ?", owner) }
			before := read()

			c.write(fx, owner, owner, id)

			if after := read(); reflect.DeepEqual(after, before) {
				t.Errorf("自分の行なのに変わらなかった: %v", after)
			}
		})
	}
}
