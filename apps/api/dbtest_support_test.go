//go:build dbtest

// 本物の MySQL に流すテスト（dbtest タグ、JUK-97）のうち、package main のテストだけが使う下ごしらえ。
// DB への繋ぎ方とテストデータの作り方は internal/dbtest にある（JUK-158）。
//
// DB は db/ のテストと同じ juken_map_test を使う。本番と同じマイグレーションが当たっていて、
// 繋ぐのは本番と同じ権限（DML だけ）のユーザー。先に `pnpm --filter @juken-map/db test-db:prepare` で用意する。
//
// テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる（db/test-db/fixtures.ts と同じ）。
// ユーザーを消せば、志望校・予定・実績などは外部キーの CASCADE で一緒に消える。
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// dbFixture は dbtest.Fixture に、package main のテストだけが使う作り方（ログイン・運用のコマンドなど）を足したもの。
type dbFixture struct{ dbtest.Fixture }

func newDBFixture(t *testing.T, db *sql.DB) dbFixture {
	return dbFixture{dbtest.Fixture{T: t, DB: db}}
}

// todayTokyo は日本時間の今日（YYYY-MM-DD）。学習記録は未来の日付を断るので、本文にはこれを使う。
func todayTokyo() string {
	return time.Now().In(dates.Tokyo).Format("2006-01-02")
}

// dbTestApp は本番と同じ registerRoutes で組んだルーター。セッションは Cookie「test」の値を利用者 ID として読む
// （Better Auth の Cookie の署名と session 表は auth_test.go が確かめるので、ここでは省く）。
type dbTestApp struct {
	rt *httpx.Router
}

func newDBTestApp(db *sql.DB) dbTestApp {
	rt := httpx.NewRouter(func(r *http.Request) (*httpx.Session, error) {
		c, err := r.Cookie("test")
		if err != nil {
			return nil, nil
		}
		return &httpx.Session{UserID: c.Value, Email: c.Value + "@example.test", Role: "user"}, nil
	})
	registerRoutes(rt, db, jobConfig{}, lineConfig{webOrigin: "https://juken-map.com"}, microcmsWebhookConfig{})
	return dbTestApp{rt: rt}
}

// userRoutes は入口が user のルート（"METHOD /path"）。skip に入れたメソッドは除く。
func (app dbTestApp) userRoutes(skip ...string) []string {
	var out []string
	for _, r := range app.rt.Routes {
		if r.Access != httpx.AccessUser {
			continue
		}
		method, _, _ := strings.Cut(r.Pattern, " ")
		skipped := false
		for _, s := range skip {
			skipped = skipped || method == s
		}
		if !skipped {
			out = append(out, r.Pattern)
		}
	}
	return out
}

// send は userID の利用者として叩く。body が nil でなければ JSON にして送る。
func (app dbTestApp) send(method, url string, body any, userID string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, url, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: "test", Value: userID})
	rec := httptest.NewRecorder()
	app.rt.ServeHTTP(rec, req)
	return rec
}
