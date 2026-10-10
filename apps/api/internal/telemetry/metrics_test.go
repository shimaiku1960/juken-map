package telemetry

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestObserveDBExportsPoolStats(t *testing.T) {
	// sql.OpenDB はまだ繋がないので、繋げない connector でもプールの数字は読める。
	db := sql.OpenDB(nopConnector{})
	defer db.Close()
	db.SetMaxOpenConns(15)

	m := NewMetrics(nil)
	m.ObserveDB(db)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	for _, want := range []string{
		`go_sql_max_open_connections{db_name="juken_map"} 15`,
		`go_sql_wait_duration_seconds_total{db_name="juken_map"} 0`,
		`go_sql_in_use_connections{db_name="juken_map"} 0`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("/metrics に %q が無い", want)
		}
	}
}

// nopConnector は繋ごうとすると失敗する connector。プールの数字を読むだけのテストで使う。
type nopConnector struct{}

func (nopConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("not connected")
}
func (nopConnector) Driver() driver.Driver { return nil }
