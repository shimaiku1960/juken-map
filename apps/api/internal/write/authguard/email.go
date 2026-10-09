package authguard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"
)

// メールの送信の上限（06 E1）。送るたびに EmailSend へ1行残し、宛先ごとと全体の数を数える。

const (
	// EmailPerRecipientPerHour は同じ宛先へ1時間に送る数の上限。再設定を何度か頼み直す本人は困らない数。
	EmailPerRecipientPerHour = 5
	// EmailGlobalPerDay はアプリ全体で24時間に送る数の上限。Resend の無料枠は1日100通で、毎日の通知と分け合う。
	EmailGlobalPerDay = 80
	// KindAdminNewUser は運営者への新規登録の通知。宛先が1つなので、宛先ごとの上限にはかけない（全体の数には入れる）。
	KindAdminNewUser = "admin-new-user"
	// emailSendLock は「数えてから記録する」までを1件ずつ通す MySQL の名前付きロック。
	emailSendLock        = "juken-map:email-send"
	emailLockWaitSeconds = 5
)

// EmailBlock は送らずに止めた理由。空なら送ってよい（記録済み）。
type EmailBlock string

const (
	EmailAllowed     EmailBlock = ""
	BlockedLock      EmailBlock = "lock"      // 名前付きロックを待ちきれなかった
	BlockedGlobal    EmailBlock = "global"    // 全体の上限
	BlockedRecipient EmailBlock = "recipient" // 宛先ごとの上限
)

// ReserveEmail は送ってよければ記録して EmailAllowed、上限を超えるなら記録せずに止めた理由を返す。
// recipientHash は宛先を正規化した SHA-256（宛先をそのまま DB に残さない）。
//
// 数えてから記録するまでを名前付きロックで1件ずつ通す（同時に来た送信がどれも「まだ上限前」を見て
// 超えないように。JUK-107）。ロックは接続に付くので、1本の接続を借りて最後まで同じ接続で流す。
func ReserveEmail(ctx context.Context, db *sql.DB, recipientHash, kind string, now time.Time) (EmailBlock, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return EmailAllowed, err
	}
	locked := false
	defer func() {
		if locked {
			if _, err := conn.ExecContext(context.WithoutCancel(ctx), "DO RELEASE_LOCK(?)", emailSendLock); err != nil {
				// 解けなかった接続はロックを持ったまま残りうるので、プールへ戻さずに捨てる
				// （接続が切れれば MySQL がロックを解く）。
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		conn.Close()
	}()

	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", emailSendLock, emailLockWaitSeconds).Scan(&acquired); err != nil {
		return EmailAllowed, err
	}
	if acquired.Int64 != 1 {
		return BlockedLock, nil
	}
	locked = true

	if err := sweepEmailSends(ctx, conn, now.Add(-24*time.Hour)); err != nil {
		return EmailAllowed, err
	}
	var global int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM EmailSend WHERE sentAt > ?", now.Add(-24*time.Hour)).Scan(&global); err != nil {
		return EmailAllowed, err
	}
	if global >= EmailGlobalPerDay {
		return BlockedGlobal, nil
	}
	if kind != KindAdminNewUser {
		var perRecipient int
		if err := conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM EmailSend WHERE recipientHash = ? AND sentAt > ? AND kind <> ?",
			recipientHash, now.Add(-time.Hour), KindAdminNewUser).Scan(&perRecipient); err != nil {
			return EmailAllowed, err
		}
		if perRecipient >= EmailPerRecipientPerHour {
			return BlockedRecipient, nil
		}
	}
	_, err = conn.ExecContext(ctx, "INSERT INTO EmailSend (recipientHash, kind, sentAt) VALUES (?, ?, ?)", recipientHash, kind, now)
	return EmailAllowed, err
}

// sweepEmailSends は1日より古い行を少しずつ消す（主キーで消し、索引の隙間をロックしない）。
func sweepEmailSends(ctx context.Context, conn *sql.Conn, cutoff time.Time) error {
	rows, err := conn.QueryContext(ctx, "SELECT id FROM EmailSend WHERE sentAt <= ? LIMIT 100", cutoff)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := conn.ExecContext(ctx, "DELETE FROM EmailSend WHERE id = ?", id); err != nil {
			return err
		}
	}
	return nil
}
