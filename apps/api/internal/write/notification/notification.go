// Package notification は通知（LINE の連携・通知の設定・送った印）への書き込みの持ち主。
// 持ち主の一覧と決まりは docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
//   - line.go：LINE の連携（Account Link の nonce・LINE Login の試行・連携・解除・Webhook の処理済みの印）
//   - このファイル：通知の設定と、毎日の通知を送った印
//
// 「LINE 通知は連携しているときだけ ON にできる」はここで守る。設定の保存は連携の行を共有ロックで読んでから書き、
// 解除は連携の行を消してから設定を落とすので、どちらが先に来ても、連携が無いのに LINE 通知が ON の行は残らない。
//
// 入力の形の確かめ（Zod と同じ 400）は入口が行い、ここには確かめ済みの値が来る。
package notification

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// ErrLineNotConnected は、LINE と連携していないのに LINE 通知を ON にしようとしたこと。Error() は利用者に見せる文言。
var ErrLineNotConnected = errors.New("LINEと連携してからLINE通知を選択してください")

// Preference は通知の設定。
// 項目の名前・型・並びを画面に返す形（apischema の NotificationPreference）とそろえ、入口が型の変換だけで渡せるようにしている。
type Preference struct {
	EmailEveningEnabled bool
	EmailMorningEnabled bool
	LineEveningEnabled  bool
	LineMorningEnabled  bool
}

// SavePreference は通知の設定を保存する（無ければ作る）。LINE 通知を ON にするなら、連携していなければ ErrLineNotConnected。
func SavePreference(ctx context.Context, db *sql.DB, userID string, p Preference, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if p.LineMorningEnabled || p.LineEveningEnabled {
			// FOR SHARE で連携の行を押さえ、保存し終わるまで解除（行の削除）を待たせる。
			var one int
			err := tx.QueryRowContext(ctx, "SELECT 1 FROM LineConnection WHERE userId = ? FOR SHARE", userID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrLineNotConnected
			}
			if err != nil {
				return err
			}
		}
		// 「無ければ INSERT、あれば UPDATE」を1文で行う（userId に UNIQUE 制約がある）。SQL は Node と同じ。
		// new は「INSERT しようとした行」の別名で、createdAt は更新しない。
		_, err := tx.ExecContext(ctx,
			`INSERT INTO NotificationPreference
			   (userId, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled,
			    createdAt, updatedAt)
			 VALUES (?, ?, ?, ?, ?, ?, ?) AS new
			 ON DUPLICATE KEY UPDATE
			   morningEnabled = new.morningEnabled,
			   eveningEnabled = new.eveningEnabled,
			   lineMorningEnabled = new.lineMorningEnabled,
			   lineEveningEnabled = new.lineEveningEnabled,
			   updatedAt = new.updatedAt`,
			userID, p.EmailMorningEnabled, p.EmailEveningEnabled, p.LineMorningEnabled, p.LineEveningEnabled, now, now)
		return err
	})
}

// Delivery は毎日の通知の「この日・この時間帯・この経路」。
type Delivery struct {
	UserID  string
	Date    time.Time
	Slot    string // morning・evening
	Channel string // email・line
}

// MarkDelivery は送る前に「送った」印を入れる。同じ組み合わせが既にあれば duplicate が true（UNIQUE 制約）。
// 送れなかったら UnmarkDelivery で消し、次の実行で再び送れるようにする。
func MarkDelivery(ctx context.Context, db *sql.DB, d Delivery, now time.Time) (id int64, duplicate bool, err error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO NotificationDelivery (userId, date, slot, channel, createdAt) VALUES (?, ?, ?, ?, ?)`,
		d.UserID, d.Date, d.Slot, d.Channel, now)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return 0, true, nil
	}
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	return id, false, err
}

// UnmarkDelivery は MarkDelivery の印を消す。
func UnmarkDelivery(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM NotificationDelivery WHERE id = ?", id)
	return err
}
