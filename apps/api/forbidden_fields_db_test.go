//go:build dbtest

// 利用者が変えてはいけない項目（ロール・持ち主・停止状態・メール確認状態など）を、書き込みの本文に混ぜても
// 書き換わらないことを確かめる（セキュリティ基準 06 の A4、JUK-97）。Node の forbidden-fields.test.ts（JUK-68）を
// 本番で動いている Go へ移したもの。
//
// registerRoutes に登録された user の書き込みを全件集め、下の表 forbiddenWrites に1本ずつ「成功する本文」を書かせる。
// 表に無いルートがあれば落ちるので、ルートを足して確かめ忘れることがない。各ルートに、成功する本文＋禁止項目を送り、
//   - 2xx で通る（禁止項目のせいで 400 になっただけなら、何も確かめていない）
//   - 呼んだ人の role・bannedAt・emailVerified・email が変わらない
//   - 別の人の行が1つも増えず変わらない（userId に別の人を入れて付け替える攻撃）
//   - 呼んだ人の行の id が、送った id になっていない
//
// を見る。Better Auth の登録・更新は Node の auth.forbidden-fields.test.ts が見る。
package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// どのテーブルにも無い大きさの id。これが行の id になっていたら、本文の id が使われた。
const forbiddenID = 2_000_000_000

// forbiddenFields は本文に混ぜる禁止項目。userId には別の人（victim）を入れる。
func forbiddenFields(victim string) map[string]any {
	return map[string]any{
		"id":            forbiddenID,
		"userId":        victim,
		"role":          "admin",
		"bannedAt":      "2000-01-01T00:00:00.000Z",
		"emailVerified": true,
		"email":         "taken-over@example.test",
	}
}

// forbiddenArrange は caller が成功するはずの書き込みを作る。本文の最上位には呼び出し側で禁止項目を足す。
// 配列の中の要素にも項目を持つ本文（予定の作成）は、ここで forbidden を混ぜる。
type forbiddenArrange func(fx dbFixture, caller string, forbidden map[string]any) (url string, body map[string]any)

func forbiddenWrites(facultyID int64) map[string]forbiddenArrange {
	path := func(format string, id int64) string { return fmt.Sprintf(format, id) }
	return map[string]forbiddenArrange{
		// 志望校
		"POST /api/goals": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/goals", map[string]any{"facultyId": facultyID}
		},
		"PUT /api/goals/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/goals/%d", fx.finalGoal(caller, facultyID)), map[string]any{"status": "candidate"}
		},
		"PATCH /api/goals/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/goals/%d", fx.finalGoal(caller, facultyID)), map[string]any{"note": "メモ"}
		},
		"DELETE /api/goals/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/goals/%d", fx.finalGoal(caller, facultyID)), nil
		},

		// 実績
		"POST /api/study-logs": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/study-logs", map[string]any{"date": todayTokyo(), "minutes": 30}
		},
		"PATCH /api/study-logs/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/study-logs/%d", fx.studyLog(caller)), map[string]any{"date": todayTokyo(), "minutes": 45}
		},
		"DELETE /api/study-logs/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/study-logs/%d", fx.studyLog(caller)), nil
		},

		// 予定
		"POST /api/study-plans": func(_ dbFixture, _ string, forbidden map[string]any) (string, map[string]any) {
			item := map[string]any{"content": "予定"}
			maps.Copy(item, forbidden)
			return "/api/study-plans", map[string]any{"date": "2027-02-20", "items": []any{item}}
		},
		"PATCH /api/study-plans/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/study-plans/%d", fx.studyPlan(caller)), map[string]any{"content": "書き換え"}
		},
		"DELETE /api/study-plans/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/study-plans/%d", fx.studyPlan(caller)), nil
		},
		"POST /api/study-plans/{id}/complete": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/study-plans/%d/complete", fx.studyPlan(caller)), map[string]any{"minutes": 30}
		},

		// 参考書
		"POST /api/textbooks": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/textbooks", map[string]any{"name": "参考書"}
		},
		"PATCH /api/textbooks/{id}": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			return path("/api/textbooks/%d", fx.textbook(caller)), map[string]any{"totalAmount": 100}
		},

		// 設定・連携
		"PUT /api/profile": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/profile", map[string]any{"nickname": "なまえ"}
		},
		"PUT /api/notification-preferences": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/notification-preferences", map[string]any{
				"emailMorningEnabled": true, "emailEveningEnabled": false,
				"lineMorningEnabled": false, "lineEveningEnabled": false,
			}
		},
		"POST /api/line/account-link": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/line/account-link", map[string]any{"linkToken": "link-token"}
		},
		"DELETE /api/line/connection": func(fx dbFixture, caller string, _ map[string]any) (string, map[string]any) {
			fx.lineConnection(caller)
			return "/api/line/connection", nil
		},
		"POST /api/analytics/registration": func(dbFixture, string, map[string]any) (string, map[string]any) {
			return "/api/analytics/registration", nil
		},
	}
}

func TestA4ForbiddenFieldsDB(t *testing.T) {
	db := openTestDB(t)
	fx := dbFixture{t: t, db: db}
	app := newDBTestApp(db)
	writes := forbiddenWrites(fx.university())

	// 持ち主の列（userId）を持つテーブルは information_schema から引くので、あとからテーブルを足しても自動で対象に入る。
	var tablesWithUserID []string
	for _, row := range fx.rows(`SELECT TABLE_NAME AS name FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND COLUMN_NAME = 'userId'`) {
		var r struct{ Name string }
		json.Unmarshal([]byte(row), &r)
		tablesWithUserID = append(tablesWithUserID, r.Name)
	}

	// rowsOf は、その人の行を user と userId を持つ全テーブルから集める。
	rowsOf := func(fx dbFixture, user string) map[string][]string {
		rows := map[string][]string{"user": fx.rows("SELECT * FROM `user` WHERE id = ?", user)}
		for _, table := range tablesWithUserID {
			rows[table] = fx.rows(fmt.Sprintf("SELECT * FROM `%s` WHERE userId = ?", table), user)
		}
		return rows
	}
	protectedColumnsOf := func(fx dbFixture, user string) []string {
		return fx.rows("SELECT role, bannedAt, emailVerified, email FROM `user` WHERE id = ?", user)
	}

	t.Run("A4 利用者の API の書き込みは全件が表にあり、表に余りも無い", func(t *testing.T) {
		routes := app.userRoutes("GET", "HEAD")
		// 0件のまま通る（何も突き合わせていない）状態を防ぐ。
		if len(routes) <= 15 {
			t.Fatalf("user の書き込みが %d 本しかない", len(routes))
		}
		var missing, extra []string
		registered := map[string]bool{}
		for _, r := range routes {
			registered[r] = true
			if _, ok := writes[r]; !ok {
				missing = append(missing, r)
			}
		}
		for r := range writes {
			if !registered[r] {
				extra = append(extra, r)
			}
		}
		sort.Strings(extra)
		if len(missing) > 0 || len(extra) > 0 {
			t.Errorf("表に無いルート: %v\n登録されていない表の行: %v", missing, extra)
		}
	})

	t.Run("A4 持ち主の列を持つテーブルを DB から拾えている", func(t *testing.T) {
		for _, want := range []string{"FinalGoal", "StudyLog", "StudyPlan", "Textbook", "NotificationPreference", "LineConnection"} {
			found := false
			for _, table := range tablesWithUserID {
				found = found || table == want
			}
			if !found {
				t.Errorf("%s が無い（拾えたもの %v）", want, tablesWithUserID)
			}
		}
	})

	for route, arrange := range writes {
		method, _, _ := strings.Cut(route, " ")
		t.Run("A4 "+route+"：禁止項目を混ぜても通り、書き換わらない", func(t *testing.T) {
			fx := dbFixture{t: t, db: db}
			caller, victim := fx.user(), fx.user()
			forbidden := forbiddenFields(victim)
			url, body := arrange(fx, caller, forbidden)
			sent := map[string]any{}
			maps.Copy(sent, body)
			maps.Copy(sent, forbidden)
			callerBefore := protectedColumnsOf(fx, caller)
			victimBefore := rowsOf(fx, victim)

			res := app.send(method, url, sent, caller)

			if res.Code < 200 || res.Code >= 300 {
				t.Fatalf("status = %d: %s", res.Code, res.Body)
			}
			if after := protectedColumnsOf(fx, caller); !reflect.DeepEqual(after, callerBefore) {
				t.Errorf("呼んだ人の守る列が変わった\nbefore %v\nafter  %v", callerBefore, after)
			}
			if after := rowsOf(fx, victim); !reflect.DeepEqual(after, victimBefore) {
				t.Errorf("別の人の行が変わった\nbefore %v\nafter  %v", victimBefore, after)
			}
			for table, rows := range rowsOf(fx, caller) {
				for _, row := range rows {
					var r struct {
						ID any `json:"id"`
					}
					json.Unmarshal([]byte(row), &r)
					if fmt.Sprint(r.ID) == fmt.Sprint(forbiddenID) {
						t.Errorf("%s の行の id が本文の id になった: %s", table, row)
					}
				}
			}
		})
	}
}
