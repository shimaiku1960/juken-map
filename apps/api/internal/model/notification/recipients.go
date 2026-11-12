package notification

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// 毎日の学習通知（JUK-74）を受け取る人の読み取り。

// Recipient はその時間帯に通知を受け取る設定の利用者と、その日の予定・実績。
type Recipient struct {
	ID         string
	Email      *string
	Name       *string
	Nickname   *string
	Morning    bool // メール（朝）
	Evening    bool // メール（夜）
	LineMorn   bool
	LineEven   bool
	LineUserID *string
	Plans      []Plan
	LogMinutes []int64
}

// Plan は通知の文面に載せるその日の予定。
type Plan struct {
	Done         bool
	Content      *string
	Subject      *string
	TextbookName *string
}

// slotColumns は時間帯（契約の NotificationSlot の値）ごとの設定の列名。列名は ? で渡せない（値ではなく識別子なので）。
// 利用者の入力ではなく、この固定の名前だけを SQL に埋め込む。
var slotColumns = map[string][2]string{
	"morning": {"morningEnabled", "lineMorningEnabled"},
	"evening": {"eveningEnabled", "lineEveningEnabled"},
}

// Recipients は時間帯 slot に通知を受け取る利用者と、[start, end) の予定・実績を返す。
// SQL を3本に分ける。利用者から見て予定と実績はどちらも1対多なので、
// 1本の JOIN にすると（予定の数 × 実績の数）の行に膨らみ、学習時間が重複して数えられる。
func Recipients(ctx context.Context, db *sql.DB, slot string, start, end time.Time) ([]Recipient, error) {
	cols, ok := slotColumns[slot]
	if !ok {
		return nil, fmt.Errorf("unknown slot %q", slot)
	}
	// 通知設定の無い利用者は対象外なので、設定とは内部結合（JOIN）。
	// LINE は未連携でもメールだけ受け取れるので、外部結合（LEFT JOIN）。
	// #nosec G202 -- 列名は slotColumns に書いた固定の名前だけ。値は渡していない
	rows, err := db.QueryContext(ctx,
		`SELECT u.id, u.email, u.name, u.nickname,
		        np.morningEnabled, np.eveningEnabled, np.lineMorningEnabled, np.lineEveningEnabled,
		        lc.lineUserId
		 FROM `+"`user`"+` AS u
		 JOIN NotificationPreference AS np ON np.userId = u.id
		 LEFT JOIN LineConnection AS lc ON lc.userId = u.id
		 WHERE np.`+cols[0]+` = TRUE OR np.`+cols[1]+` = TRUE
		 ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	var users []Recipient
	byID := map[string]int{}
	for rows.Next() {
		var u Recipient
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Nickname,
			&u.Morning, &u.Evening, &u.LineMorn, &u.LineEven, &u.LineUserID); err != nil {
			rows.Close()
			return nil, err
		}
		byID[u.ID] = len(users)
		users = append(users, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// IN () は空だと SQL の構文エラーになるので、先に抜ける。
	if len(users) == 0 {
		return nil, nil
	}

	// IN (?, ?, …) の ? を人数ぶん並べる（ドライバは配列を展開しない）。
	in := strings.TrimSuffix(strings.Repeat("?,", len(users)), ",")
	args := make([]any, 0, len(users)+2)
	for _, u := range users {
		args = append(args, u.ID)
	}
	args = append(args, start, end)

	// #nosec G202 -- in は人数ぶん並べた ? だけ。値は args で渡す
	planRows, err := db.QueryContext(ctx,
		`SELECT p.userId, p.done, p.content, p.subject, t.name AS textbookName
		 FROM StudyPlan AS p
		 LEFT JOIN Textbook AS t ON t.id = p.textbookId
		 WHERE p.userId IN (`+in+`) AND p.date >= ? AND p.date < ?
		 ORDER BY p.id`, args...)
	if err != nil {
		return nil, err
	}
	for planRows.Next() {
		var (
			userID string
			p      Plan
		)
		if err := planRows.Scan(&userID, &p.Done, &p.Content, &p.Subject, &p.TextbookName); err != nil {
			planRows.Close()
			return nil, err
		}
		u := &users[byID[userID]]
		u.Plans = append(u.Plans, p)
	}
	planRows.Close()
	if err := planRows.Err(); err != nil {
		return nil, err
	}

	// #nosec G202 -- in は人数ぶん並べた ? だけ。値は args で渡す
	logRows, err := db.QueryContext(ctx,
		`SELECT userId, minutes FROM StudyLog
		 WHERE userId IN (`+in+`) AND date >= ? AND date < ?
		 ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer logRows.Close()
	for logRows.Next() {
		var (
			userID  string
			minutes int64
		)
		if err := logRows.Scan(&userID, &minutes); err != nil {
			return nil, err
		}
		u := &users[byID[userID]]
		u.LogMinutes = append(u.LogMinutes, minutes)
	}
	return users, logRows.Err()
}
