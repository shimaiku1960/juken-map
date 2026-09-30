//go:build parity

// 書き込みの API の応答一致テスト（JUK-75）。parity_test.go と同じく parity.sh から動かす。
//
// Node と Go は同じ DB を使うので、成功する書き込みは Node → Go の順に同じ本文を送り、
// 2回目（Go）の応答が1回目（Node）と同じかを比べる。どちらも同じ値で上書きするので、比べる時点の
// DB の状態は揃っている。書き換えた行は、テストの前に控えておいた値へ最後に戻す。
package main

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

const jsonType = "application/json"

// writeCases が比べる書き込み。入力チェックで弾くものは DB を変えないので全員に送り、
// 成功するもの（firstUser）は1人だけにする。
var writeCases = []struct {
	method      string
	path        string
	contentType string // "" なら付けない
	body        string
	as          []who
	// ignore は値が毎回変わる項目（書き込んだ時刻）。有るか無いかだけを比べる。
	ignore []string
}{
	// 通知設定
	{"PUT", "/api/notification-preferences", jsonType, `{}`, []who{anonymous, forged, unknownUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType,
		`{"emailMorningEnabled":true,"emailEveningEnabled":false,"lineMorningEnabled":false,"lineEveningEnabled":true}`, []who{firstUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType,
		`{"emailMorningEnabled":false,"emailEveningEnabled":true,"lineMorningEnabled":false,"lineEveningEnabled":false}`, []who{firstUser}, nil},
	// LINE と連携していないのに LINE 通知を選んだ（合成ユーザーは連携していない）
	{"PUT", "/api/notification-preferences", jsonType,
		`{"emailMorningEnabled":false,"emailEveningEnabled":false,"lineMorningEnabled":true,"lineEveningEnabled":false}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `{"emailMorningEnabled":"yes"}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `{"emailMorningEnabled":true}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `[]`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `null`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `{a`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `{"__proto__":{"emailMorningEnabled":true}}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", "", "", []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", "", `{}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", "text/plain", `{}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", "application/x-www-form-urlencoded", `a=1`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", "APPLICATION/JSON; charset=utf-8", `{}`, []who{eachUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, "{\"a\":\"\xff\"}", []who{firstUser}, nil},
	{"PUT", "/api/notification-preferences", jsonType, `{"a":"` + strings.Repeat("x", bodyLimit) + `"}`, []who{firstUser}, nil},
	// 書き込みの無いメソッドは 404（Node にも無い）
	{"POST", "/api/notification-preferences", jsonType, `{}`, []who{firstUser}, nil},

	// プロフィール
	{"PUT", "/api/profile", jsonType, `{"nickname":"a"}`, []who{anonymous, forged, unknownUser}, nil},
	{"PUT", "/api/profile", jsonType, `{"nickname":"　応答一致　テスト　"}`, []who{firstUser}, []string{"updatedAt"}},
	// 空白だけは通って空文字で保存される（Zod の trim が長さの確かめより後にあるため）
	{"PUT", "/api/profile", jsonType, `{"nickname":"   "}`, []who{firstUser}, []string{"updatedAt"}},
	{"PUT", "/api/profile", jsonType, `{"nickname":"` + strings.Repeat("😀", 50) + `"}`, []who{firstUser}, []string{"updatedAt"}},
	{"PUT", "/api/profile", jsonType, `{"nickname":"` + strings.Repeat("😀", 51) + `"}`, []who{eachUser}, nil},
	{"PUT", "/api/profile", jsonType, `{"nickname":""}`, []who{eachUser}, nil},
	{"PUT", "/api/profile", jsonType, `{"nickname":1}`, []who{eachUser}, nil},
	{"PUT", "/api/profile", jsonType, `{}`, []who{eachUser}, nil},
	{"PUT", "/api/profile", jsonType, `"x"`, []who{eachUser}, nil},
}

func TestParityWrites(t *testing.T) {
	nodeURL, goURL := os.Getenv("PARITY_NODE_URL"), os.Getenv("PARITY_GO_URL")
	users := strings.Fields(os.Getenv("PARITY_COOKIES"))
	if nodeURL == "" || goURL == "" || len(users) == 0 {
		t.Fatal("parity.sh から動かしてください")
	}
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	restore := snapshotUser(t, db, users[0])
	defer restore()

	cookieName, _, _ := strings.Cut(users[0], "=")
	cookiesFor := func(w who) []string {
		switch w {
		case forged:
			return []string{cookieName + "=" + sign("forged-token", "not-the-secret")}
		case unknownUser:
			return []string{cookieName + "=" + sign("no-such-token", os.Getenv("BETTER_AUTH_SECRET"))}
		case eachUser:
			return users
		case firstUser:
			return users[:1]
		}
		return []string{""}
	}

	for _, c := range writeCases {
		for _, w := range c.as {
			name := c.body
			if len(name) > 60 {
				name = name[:60] + "…"
			}
			t.Run(fmt.Sprintf("%s %s %q（%s）", c.method, c.path, name, whoNames[w]), func(t *testing.T) {
				for i, cookie := range cookiesFor(w) {
					node := send(t, nodeURL, c.method, c.path, cookie, c.contentType, c.body)
					gon := send(t, goURL, c.method, c.path, cookie, c.contentType, c.body)
					for _, key := range c.ignore {
						maskKey(node.body, key)
						maskKey(gon.body, key)
					}
					if diffs := compareResponses(node, gon); len(diffs) > 0 {
						t.Fatalf("%d人目で食い違い（Node → Go）:\n  %s", i+1, strings.Join(diffs, "\n  "))
					}
				}
			})
		}
	}
}

// send は本文を付けて送る。Content-Length は本文のバイト数（Go のクライアントが付ける）。
func send(t *testing.T, base, method, path, cookie, contentType, body string) response {
	t.Helper()
	req, err := http.NewRequest(method, base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body == "" {
		// 本文が無いときは Content-Length も付けない（Node の「本文なし」の判定に合わせる）。
		req.Body, req.ContentLength = nil, 0
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept-Encoding", "identity")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s%s: %v", method, base, path, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: res.StatusCode, header: res.Header, body: decodeBody(t, raw)}
}

func decodeBody(t *testing.T, raw []byte) any {
	t.Helper()
	v, err := parseJSON(string(raw))
	if err != nil {
		t.Fatalf("本文が JSON でない: %q", raw)
	}
	return v
}

// maskKey は、値が毎回変わる項目を「有る」という印に置き換える。
func maskKey(body any, key string) {
	if m, ok := body.(map[string]any); ok {
		if _, has := m[key]; has {
			m[key] = "(present)"
		}
	}
}

// snapshotUser は、書き込みのケースが書き換える行（ニックネームと通知設定）を控え、戻す関数を返す。
func snapshotUser(t *testing.T, db *sql.DB, cookie string) func() {
	t.Helper()
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Cookie", cookie)
	s, err := (&sessionAuth{db: db, secret: []byte(os.Getenv("BETTER_AUTH_SECRET"))}).load(req)
	if err != nil || s == nil {
		t.Fatalf("1人目のセッションを読めません: %v", err)
	}

	var nickname sql.NullString
	var updatedAt string
	if err := db.QueryRow("SELECT nickname, updatedAt FROM `user` WHERE id = ?", s.UserID).Scan(&nickname, &updatedAt); err != nil {
		t.Fatal(err)
	}
	// 通知設定は、行ごと控える（無ければ最後に消す）。
	var pref []any
	row := db.QueryRow(`SELECT morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled, createdAt, updatedAt
		FROM NotificationPreference WHERE userId = ?`, s.UserID)
	var m, e, lm, le bool
	var created, updated string
	switch err := row.Scan(&m, &e, &lm, &le, &created, &updated); err {
	case nil:
		pref = []any{m, e, lm, le, created, updated}
	case sql.ErrNoRows:
	default:
		t.Fatal(err)
	}

	return func() {
		if _, err := db.Exec("UPDATE `user` SET nickname = ?, updatedAt = ? WHERE id = ?", nickname, updatedAt, s.UserID); err != nil {
			t.Errorf("ニックネームを戻せません: %v", err)
		}
		if pref == nil {
			_, err = db.Exec("DELETE FROM NotificationPreference WHERE userId = ?", s.UserID)
		} else {
			_, err = db.Exec(`UPDATE NotificationPreference SET morningEnabled = ?, eveningEnabled = ?,
				lineMorningEnabled = ?, lineEveningEnabled = ?, createdAt = ?, updatedAt = ? WHERE userId = ?`,
				append(pref, s.UserID)...)
		}
		if err != nil {
			t.Errorf("通知設定を戻せません: %v", err)
		}
		t.Logf("1人目（%s）のニックネームと通知設定を元に戻した（ニックネーム %s）", s.UserID, strconv.Quote(nickname.String))
	}
}
