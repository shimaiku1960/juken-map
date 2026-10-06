// Package database は MySQL への接続と、書き込み・読み取りの両方で使う補助を持つ。
// 書き込みの持ち主（internal/write）と読み取り（internal/feature）が同じものを使えるよう、
// package main から分けた（JUK-152、docs/architecture.md「バックエンドの構成」）。
package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"database/sql/driver"
	_ "embed"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/go-sql-driver/mysql"
	"go.opentelemetry.io/otel/trace"
)

// rdsCA は RDS（ap-northeast-1）のルート証明書。RDS の証明書は OS の信頼リストに無い AWS 独自の
// 認証局が発行するので、自分で持って確かめる。Node の mysql2 も同じ証明書を同梱している
// （ssl: "Amazon RDS"）。取得元: https://truststore.pki.rds.amazonaws.com/ap-northeast-1/ap-northeast-1-bundle.pem
//
//go:embed rds-ca-ap-northeast-1.pem
var rdsCA []byte

// Open は DATABASE_URL（mysql://user:pass@host:port/db）をドライバの設定に直し、
// 接続プールを作る。Node 側の parseDatabaseUrl と createPool（seed・テスト用の db/connection.ts）にあたる。
func Open(databaseURL string) (*sql.DB, error) {
	cfg, err := Config(databaseURL)
	if err != nil {
		return nil, err
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	// SQL 1本ずつのトレース（下の tracedSpanOptions）。送り先はアプリ全体の設定（tracing.go の setupTracing）を使い、
	// その前やコマンドで動くときは何もしない。
	db := otelsql.OpenDB(connector, tracedSpanOptions)
	// Node 側の connectionLimit（既定 15）に揃える。比べるときに条件を同じにするため。
	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(15)
	// 使い回す接続を一定時間で張り直す。MySQL（wait_timeout）やネットワークの途中の機器が
	// 黙って切った接続を掴むと、次の照会が「invalid connection」で失敗する。
	// ドライバの README が勧める「5分より短く」に従う。
	db.SetConnMaxLifetime(3 * time.Minute)

	// sql.OpenDB はまだ繋がない。起動時に一度繋いで、設定の誤りをここで落とす。
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// tracedSpanOptions は SQL のスパンの出し方。1本の照会につき1つのスパンにして、SQL の文（? のまま。
// 値はドライバが後で埋めるので入らない）を属性に残す。
var tracedSpanOptions = otelsql.WithSpanOptions(otelsql.SpanOptions{
	// 結果を読む時間・接続の使い回しの準備は、照会のスパンと別に出すと数が倍になる割に読むことが無い。
	OmitRows:             true,
	OmitConnResetSession: true,
	OmitConnPrepare:      true,
	// リクエストの外（起動時の確認など）の SQL は、親の無いスパンになって一覧を埋めるので出さない。
	SpanFilter: func(ctx context.Context, _ otelsql.Method, _ string, _ []driver.NamedValue) bool {
		return trace.SpanContextFromContext(ctx).IsValid()
	},
})

// Config は DATABASE_URL をドライバの設定に直す。繋がずに中身を確かめられるよう Open から分けた。
func Config(databaseURL string) (*mysql.Config, error) {
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
	// UPDATE の件数を「値が変わった行」ではなく「WHERE に当たった行」で数える。
	// Node の mysql2 は既定でこの数え方（FOUND_ROWS）なので、同じ値で UPDATE しても 1 になる。
	// 揃えないと、同じ日付をもう一度記録したときに Go だけが「見つからない（404）」を返す（JUK-80、sim.go）。
	cfg.ClientFoundRows = true

	// RDS へは TLS で繋ぎ、証明書とホスト名を確かめる（パスワードと利用者のデータが平文で流れない）。
	// 手元と CI の MySQL は証明書を持たないので対象外。Node の createPool と同じ条件。
	if strings.HasSuffix(u.Hostname(), ".rds.amazonaws.com") {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(rdsCA) {
			return nil, errors.New("RDS のルート証明書を読めません")
		}
		cfg.TLS = &tls.Config{RootCAs: roots, ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}
	}
	return cfg, nil
}

// ISOFromDatetime は DATETIME(3) の文字列 "2026-09-27 00:00:00.000" を
// Date#toISOString と同じ "2026-09-27T00:00:00.000Z" にする。Node 側の toIsoString と同じ。
func ISOFromDatetime(value string) string {
	return value[:10] + "T" + value[11:] + "Z"
}

// IsMySQLError は MySQL のエラー番号で見分ける。番号は下の定数で書き、1062 などの数字を直接書かない。
func IsMySQLError(err error, number uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == number
}

const (
	DuplicateEntry  = 1062 // ER_DUP_ENTRY：一意制約に当たった
	RowIsReferenced = 1451 // ER_ROW_IS_REFERENCED_2：外部キーに参照されていて消せない
)

// InTx は fn をトランザクションの中で動かす。fn がエラーを返したら取り消す。
func InTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Runner は *sql.DB と *sql.Tx の両方で使う読み書き。
type Runner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// QueryRower は *sql.DB と *sql.Tx の共通部分（トランザクションの中でも外でも読めるように）。
type QueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Placeholders は IN (…)・VALUES に並べる ? を n 個つなぐ。
func Placeholders(n int, one string) string {
	return strings.TrimSuffix(strings.Repeat(one+", ", n), ", ")
}
