//go:build dbtest

package university

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	vt "github.com/shimaiku1960/juken-map/apps/api/internal/validation/validationtest"
)

// 大学・学部の操作を、本物の MySQL で確かめる。管理画面の入口を通した流れ（文言・監査ログ）は internal/app の TestAdminMastersDB にある。
func TestUniversityDB(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Open(t)
	fx := dbtest.Fixture{T: t, DB: db}
	now := time.Now().UTC().Truncate(time.Millisecond)
	universityBody := func(name string) any {
		return vt.Parse(t, `{"name":"`+name+`","prefecture":"東京都","type":"私立"}`)
	}
	facultyBody := func(universityID int64, tagIDs string) any {
		return vt.Parse(t, fmt.Sprintf(`{"name":"法学部","examDate":"2027-02-15","tagIds":%s,"universityId":%d}`, tagIDs, universityID))
	}

	name := "大学-" + dbtest.Hex(8)
	u, err := CreateUniversity(ctx, db, universityBody(name), now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fx.Exec("DELETE FROM University WHERE id = ?", u.ID) })
	if _, err := CreateUniversity(ctx, db, universityBody(u.Name), now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("同じ名前の大学: %v", err)
	}

	if _, err := CreateUniversity(ctx, db, vt.Parse(t, `{}`), now); err == nil {
		t.Error("入力の決まりを通らない本文が通った")
	}

	in := facultyBody(u.ID, "[]")
	if _, err := CreateFaculty(ctx, db, facultyBody(999999999999, "[]"), now); !errors.Is(err, ErrNotFound) {
		t.Errorf("無い大学: %v", err)
	}
	if _, err := CreateFaculty(ctx, db, facultyBody(u.ID, "[999999999999]"), now); !errors.Is(err, ErrInvalidTags) {
		t.Errorf("無いタグ: %v", err)
	}
	f, err := CreateFaculty(ctx, db, in, now)
	if err != nil || f.ExamDate != "2027-02-15" || len(f.TagIds) != 0 {
		t.Fatalf("学部: %+v, %v", f, err)
	}
	if _, err := CreateFaculty(ctx, db, in, now); !errors.Is(err, ErrDuplicate) {
		t.Errorf("同じ名前の学部: %v", err)
	}

	// 読み取り：管理画面の一覧・詳細と、利用者の画面の大学詳細
	page, err := List(ctx, db, map[string][]string{"q": {" " + name + " "}})
	if err != nil || page.Total != 1 || page.Page != 1 || len(page.Universities) != 1 || page.Universities[0].FacultyCount != 1 {
		t.Errorf("一覧: %+v, %v", page, err)
	}
	detail, err := FindDetail(ctx, db, u.ID)
	if err != nil || len(detail.Faculties) != 1 || detail.Faculties[0].ExamDate != "2027-02-15" || len(detail.Faculties[0].Tags) != 0 {
		t.Errorf("管理画面の詳細: %+v, %v", detail, err)
	}
	profile, err := FindProfile(ctx, db, u.ID)
	if err != nil || len(profile.Faculties) != 1 || profile.Faculties[0].ExamDate != "2027-02-15T00:00:00.000Z" {
		t.Errorf("大学詳細: %+v, %v", profile, err)
	}
	if _, err := FindDetail(ctx, db, 999999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("無い大学の詳細: %v", err)
	}
	if _, err := FindProfile(ctx, db, 999999999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("無い大学の大学詳細: %v", err)
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
