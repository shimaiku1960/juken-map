//go:build dbtest

// 利用者の API（入口 E3）で、他人の持ち物の ID を渡すと断られることを確かめる（セキュリティ基準 06 の A3、JUK-97）。
// Node の ownership.test.ts（JUK-67）を、本番で動いている Go へ移したもの。
//
// registerRoutes に登録された user のルートは、1本残らず下の表 ownershipTable で分類させる。
// ルートを足して表に書かなければ落ちるので、持ち主の確認を持たないルートが否定テスト無しで紛れ込むことがない。
// path に ID を取るルートは「持ち物」か「マスター」のどちらかでなければならない。
//
// 持ち物のケースは同じ形のリクエストを2回送る。ID が他人のものなら 4xx で DB が変わらず、自分のものなら 2xx になる。
// 後者が無いと、本文の誤りで返る 400 でも「断られた」ことになり、持ち主の確認を外しても気づけない。
package app

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
)

// ownershipAttempt は1回分のリクエストと、断られたあとに DB が変わっていないかを見るための読み取り。
type ownershipAttempt struct {
	url  string
	body any
	read func() []string
}

// ownershipArrange は、caller（ログインしている人）が holder の持ち物の ID を渡して叩くリクエストを作る。
type ownershipArrange func(fx dbFixture, caller, holder string) ownershipAttempt

type ownershipKind int

const (
	kindNone   ownershipKind = iota // ID を受け取らず、セッションの人のものだけを扱う
	kindMaster                      // 受け取る ID は全員で共有するマスター（大学・学部・参考書マスター）で、持ち主がいない
	kindOwned                       // 持ち物の ID を受け取る。ケースごとに他人の ID で叩く
)

type ownershipClass struct {
	kind   ownershipKind
	reason string
	cases  map[string]ownershipArrange
}

func none(reason string) ownershipClass   { return ownershipClass{kind: kindNone, reason: reason} }
func master(reason string) ownershipClass { return ownershipClass{kind: kindMaster, reason: reason} }
func owned(cases map[string]ownershipArrange) ownershipClass {
	return ownershipClass{kind: kindOwned, cases: cases}
}

// ownershipTable は user のルートの分類。facultyID は志望校を作るときの学部（テストの最初に作る）。
func ownershipTable(facultyID int64) map[string]ownershipClass {
	goalsOf := func(fx dbFixture, user string) func() []string {
		return func() []string { return fx.Rows("SELECT * FROM FinalGoal WHERE userId = ?", user) }
	}
	logsOf := func(fx dbFixture, user string) func() []string {
		return func() []string { return fx.Rows("SELECT * FROM StudyLog WHERE userId = ?", user) }
	}
	plansOf := func(fx dbFixture, user string) func() []string {
		return func() []string { return fx.Rows("SELECT * FROM StudyPlan WHERE userId = ?", user) }
	}
	logByID := func(fx dbFixture, id int64) func() []string {
		return func() []string { return fx.Rows("SELECT * FROM StudyLog WHERE id = ?", id) }
	}
	planByID := func(fx dbFixture, id int64) func() []string {
		return func() []string { return fx.Rows("SELECT * FROM StudyPlan WHERE id = ?", id) }
	}
	otherGoal := func(body any) map[string]ownershipArrange {
		return map[string]ownershipArrange{
			"他人の志望校": func(fx dbFixture, _, holder string) ownershipAttempt {
				id := fx.FinalGoal(holder, facultyID)
				return ownershipAttempt{url: fmt.Sprintf("/api/goals/%d", id), body: body, read: goalsOf(fx, holder)}
			},
		}
	}

	return map[string]ownershipClass{
		"GET /api/dashboard": none("自分の集計"),

		// 志望校
		"GET /api/goals":              none("自分の志望校の一覧"),
		"GET /api/goals/first-choice": none("自分の第一志望"),
		"POST /api/goals":             master("facultyId は学部マスター"),
		"PUT /api/goals/{id}":         owned(otherGoal(map[string]any{"status": "candidate"})),
		"PATCH /api/goals/{id}":       owned(otherGoal(map[string]any{"note": "書き換え"})),
		"DELETE /api/goals/{id}":      owned(otherGoal(nil)),

		// 実績
		"GET /api/study-logs":       none("自分の実績の一覧"),
		"GET /api/study-logs/daily": none("自分の日ごとの学習時間"),
		"POST /api/study-logs": owned(map[string]ownershipArrange{
			"他人の参考書": func(fx dbFixture, caller, holder string) ownershipAttempt {
				textbookID := fx.Textbook(holder)
				return ownershipAttempt{url: "/api/study-logs",
					body: map[string]any{"date": todayTokyo(), "minutes": 30, "textbookId": textbookID}, read: logsOf(fx, caller)}
			},
		}),
		"PATCH /api/study-logs/{id}": owned(map[string]ownershipArrange{
			"他人の実績": func(fx dbFixture, _, holder string) ownershipAttempt {
				id := fx.StudyLog(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-logs/%d", id),
					body: map[string]any{"date": todayTokyo(), "minutes": 45}, read: logByID(fx, id)}
			},
			"他人の参考書": func(fx dbFixture, caller, holder string) ownershipAttempt {
				id := fx.StudyLog(caller)
				textbookID := fx.Textbook(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-logs/%d", id),
					body: map[string]any{"date": todayTokyo(), "minutes": 30, "textbookId": textbookID}, read: logByID(fx, id)}
			},
		}),
		"DELETE /api/study-logs/{id}": owned(map[string]ownershipArrange{
			"他人の実績": func(fx dbFixture, _, holder string) ownershipAttempt {
				id := fx.StudyLog(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-logs/%d", id), read: logByID(fx, id)}
			},
		}),

		// 予定
		"GET /api/study-plans": none("自分の予定の一覧"),
		"POST /api/study-plans": owned(map[string]ownershipArrange{
			"他人の参考書": func(fx dbFixture, caller, holder string) ownershipAttempt {
				textbookID := fx.Textbook(holder)
				return ownershipAttempt{url: "/api/study-plans",
					body: map[string]any{"date": "2027-02-20", "items": []any{map[string]any{"textbookId": textbookID}}},
					read: plansOf(fx, caller)}
			},
		}),
		"PATCH /api/study-plans/{id}": owned(map[string]ownershipArrange{
			"他人の予定": func(fx dbFixture, _, holder string) ownershipAttempt {
				id := fx.StudyPlan(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-plans/%d", id),
					body: map[string]any{"content": "書き換え"}, read: planByID(fx, id)}
			},
			"他人の参考書": func(fx dbFixture, caller, holder string) ownershipAttempt {
				id := fx.StudyPlan(caller)
				textbookID := fx.Textbook(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-plans/%d", id),
					body: map[string]any{"textbookId": textbookID}, read: planByID(fx, id)}
			},
		}),
		"DELETE /api/study-plans/{id}": owned(map[string]ownershipArrange{
			"他人の予定": func(fx dbFixture, _, holder string) ownershipAttempt {
				id := fx.StudyPlan(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-plans/%d", id), read: planByID(fx, id)}
			},
		}),
		"POST /api/study-plans/{id}/complete": owned(map[string]ownershipArrange{
			"他人の予定": func(fx dbFixture, caller, holder string) ownershipAttempt {
				id := fx.StudyPlan(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/study-plans/%d/complete", id),
					body: map[string]any{"minutes": 30},
					read: func() []string {
						return append(append(planByID(fx, id)(), logsOf(fx, caller)()...), logsOf(fx, holder)()...)
					}}
			},
		}),

		// 参考書
		"GET /api/textbooks":        none("自分の参考書の一覧"),
		"GET /api/textbook-masters": none("参考書マスターの一覧"),
		"POST /api/textbooks":       master("masterId は参考書マスター（名前で作るときは ID を取らない）"),
		"PATCH /api/textbooks/{id}": owned(map[string]ownershipArrange{
			"他人の参考書": func(fx dbFixture, _, holder string) ownershipAttempt {
				id := fx.Textbook(holder)
				return ownershipAttempt{url: fmt.Sprintf("/api/textbooks/%d", id),
					body: map[string]any{"totalAmount": 100},
					read: func() []string { return fx.Rows("SELECT * FROM Textbook WHERE userId = ?", holder) }}
			},
		}),

		// 大学
		"GET /api/universities":      none("大学マスターの検索"),
		"GET /api/universities/{id}": master("大学マスター。登録済みかどうかはセッションの人で絞る"),

		// 設定・連携
		"PUT /api/profile":                  none("自分のプロフィール"),
		"GET /api/notification-preferences": none("自分の通知設定"),
		"PUT /api/notification-preferences": none("自分の通知設定"),
		"POST /api/line/account-link":       none("linkToken は LINE が発行したもので、nonce はセッションの人に結ぶ"),
		"GET /api/line/connection":          none("自分の LINE 連携"),
		"DELETE /api/line/connection":       none("自分の LINE 連携"),
		"POST /api/analytics/registration":  none("自分の登録計測"),
	}
}

func TestA3OwnershipDB(t *testing.T) {
	db := dbtest.Open(t)
	fx := newDBFixture(t, db)
	app := newDBTestApp(db)
	table := ownershipTable(fx.University())

	t.Run("A3 利用者の API は全件が表で分類されていて、表に余りも無い", func(t *testing.T) {
		routes := app.userRoutes()
		// 0件のまま通る（何も突き合わせていない）状態を防ぐ。
		if len(routes) <= 25 {
			t.Fatalf("user のルートが %d 本しかない", len(routes))
		}
		var missing, extra []string
		registered := map[string]bool{}
		for _, r := range routes {
			registered[r] = true
			if _, ok := table[r]; !ok {
				missing = append(missing, r)
			}
		}
		for r := range table {
			if !registered[r] {
				extra = append(extra, r)
			}
		}
		sort.Strings(extra)
		if len(missing) > 0 || len(extra) > 0 {
			t.Errorf("表に無いルート: %v\n登録されていない表の行: %v", missing, extra)
		}
	})

	t.Run("A3 path に ID を取るルートは「持ち物」か「マスター」に分類されている", func(t *testing.T) {
		withParam := 0
		for r, c := range table {
			if strings.Contains(r, "{") {
				withParam++
				if c.kind == kindNone {
					t.Errorf("%s は ID を取るのに「ID なし」になっている", r)
				}
			}
		}
		if withParam == 0 {
			t.Fatal("ID を取るルートが1本も無い")
		}
	})

	for route, class := range table {
		if class.kind != kindOwned {
			continue
		}
		method, _, _ := strings.Cut(route, " ")
		for name, arrange := range class.cases {
			t.Run(fmt.Sprintf("A3 %s：%s なら断り、DB を変えない", route, name), func(t *testing.T) {
				fx := newDBFixture(t, db)
				caller, holder := fx.User(), fx.User()
				attempt := arrange(fx, caller, holder)
				before := attempt.read()

				res := app.send(method, attempt.url, attempt.body, caller)

				if res.Code != http.StatusBadRequest && res.Code != http.StatusForbidden && res.Code != http.StatusNotFound {
					t.Errorf("status = %d（400・403・404 のどれかのはず）: %s", res.Code, res.Body)
				}
				if after := attempt.read(); !reflect.DeepEqual(after, before) {
					t.Errorf("DB が変わった\nbefore %v\nafter  %v", before, after)
				}
			})

			t.Run(fmt.Sprintf("A3 %s：%s を自分の ID に替えると通る", route, name), func(t *testing.T) {
				fx := newDBFixture(t, db)
				caller := fx.User()
				attempt := arrange(fx, caller, caller)

				res := app.send(method, attempt.url, attempt.body, caller)

				if res.Code < 200 || res.Code >= 300 {
					t.Errorf("status = %d: %s", res.Code, res.Body)
				}
			})
		}
	}
}
