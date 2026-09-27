package main

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// openDB は DATABASE_URL（mysql://user:pass@host:port/db）をドライバの設定に直し、
// 接続プールを作る。Node 側の parseDatabaseUrl と createPool（apps/api/src/infra/db.ts）にあたる。
func openDB(databaseURL string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL が空です")
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		return nil, err
	}

	cfg := mysql.NewConfig()
	cfg.User = u.User.Username()
	cfg.Passwd, _ = u.User.Password()
	cfg.Net = "tcp"
	cfg.Addr = u.Host
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	// DATETIME は time.Time にせず、"2026-09-27 00:00:00.000" の文字列のまま受け取る。
	// 応答は ISO 文字列なので、Time を経由すると解析と整形の往復が無駄になる
	// （Node 側も同じ理由で selectDateStrings を使っている。JUK-49）。
	cfg.ParseTime = false
	// パラメータの値に time.Time を渡したとき、UTC として書き出す。DB の値は UTC で入っている。
	cfg.Loc = time.UTC
	// ? への値の埋め込みをドライバ側で行い、SQL を1往復で流す。
	// 既定（false）ではサーバー側のプリペアドステートメントになり、準備・実行・後始末で
	// 1クエリ3往復かかる。Node の mysql2 の query() もドライバ側で埋め込んでいるので、条件を揃える。
	cfg.InterpolateParams = true

	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	// Node 側の connectionLimit（既定 15）に揃える。比べるときに条件を同じにするため。
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(15)

	// sql.OpenDB はまだ繋がない。起動時に一度繋いで、設定の誤りをここで落とす。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// isoFromDatetime は DATETIME(3) の文字列 "2026-09-27 00:00:00.000" を
// Date#toISOString と同じ "2026-09-27T00:00:00.000Z" にする。Node 側の toIsoString と同じ。
func isoFromDatetime(value string) string {
	return value[:10] + "T" + value[11:] + "Z"
}
