//go:build dbtest

package university

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// 大学・学部の操作を、本物の MySQL で確かめる。管理画面の入口を通した流れ（文言・監査ログ）は package main の TestAdminMastersDB にある。
func TestUniversityDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	fx := dbtest.Fixture{T: t, DB: db}
	now := time.Now().UTC().Truncate(time.Millisecond)
	examDate := time.Date(2027, 2, 15, 0, 0, 0, 0, time.UTC)

	u, err := CreateUniversity(ctx, db, UniversityInput{Name: "大学-" + dbtest.Hex(8), Prefecture: "東京都", Type: "私立"}, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fx.Exec("DELETE FROM University WHERE id = ?", u.ID) })
	if _, err := CreateUniversity(ctx, db, UniversityInput{Name: u.Name, Prefecture: "東京都", Type: "私立"}, now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("同じ名前の大学: %v", err)
	}

	in := FacultyInput{Name: "法学部", ExamDate: examDate, UniversityID: u.ID}
	if _, err := CreateFaculty(ctx, db, FacultyInput{Name: "法学部", ExamDate: examDate, UniversityID: -1}, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("無い大学: %v", err)
	}
	if _, err := CreateFaculty(ctx, db, FacultyInput{Name: "法学部", ExamDate: examDate, UniversityID: u.ID, TagIDs: []int64{-1}}, now); !errors.Is(err, ErrInvalidTags) {
		t.Errorf("無いタグ: %v", err)
	}
	f, err := CreateFaculty(ctx, db, in, now)
	if err != nil || f.ExamDate != "2027-02-15" || len(f.TagIds) != 0 {
		t.Fatalf("学部: %+v, %v", f, err)
	}
	if _, err := CreateFaculty(ctx, db, in, now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("同じ名前の学部: %v", err)
	}

	user := fx.User()
	fx.FinalGoal(user, f.ID)
	var inUse *InUseError
	if _, err := DeleteFaculty(ctx, db, f.ID); !errors.As(err, &inUse) || inUse.Count != 1 {
		t.Errorf("志望校に使われている学部: %v", err)
	}
	if _, err := DeleteUniversity(ctx, db, u.ID); !errors.As(err, &inUse) || inUse.Count != 1 {
		t.Errorf("志望校に使われている大学: %v", err)
	}

	fx.Exec("DELETE FROM FinalGoal WHERE userId = ?", user)
	deleted, err := DeleteUniversity(ctx, db, u.ID)
	if err != nil || deleted.FacultyCount != 1 {
		t.Errorf("大学を消す: %+v, %v", deleted, err)
	}
	if _, err := UpdateFaculty(ctx, db, f.ID, in); !errors.Is(err, ErrNotFound) {
		t.Errorf("大学ごと消えた学部: %v", err)
	}
}
