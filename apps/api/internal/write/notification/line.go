package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

const (
	// AttemptTTL は LINE Login の state と Account Link の nonce の寿命（セキュリティ基準 C4）。
	AttemptTTL = 10 * time.Minute
	// WebhookEventRetention は処理済みイベントの印を残す期間。LINE の再送はこれより短い。
	WebhookEventRetention = 7 * 24 * time.Hour
)

// LinkResult は Account Link の nonce で連携を確定した結果。
type LinkResult string

const (
	Linked  LinkResult = "linked"
	Taken   LinkResult = "taken"   // その LINE は別のアカウントに連携済み
	Expired LinkResult = "expired" // nonce が無い・期限切れ・使用済み
)

// Attempt は LINE Login を始めたときに残す値。戻ってきたときに state で引き当てる。
type Attempt struct {
	UserID       string
	Nonce        string
	CodeVerifier string
	RedirectURI  string
}

// IssueLinkNonce は連携開始用の nonce を1つだけ持たせる（古いものは捨てる）。
func IssueLinkNonce(ctx context.Context, db *sql.DB, userID, nonce string, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM LineLinkNonce WHERE userId = ?", userID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO LineLinkNonce (nonce, userId, expiresAt, createdAt) VALUES (?, ?, ?, ?)",
			nonce, userID, now.Add(AttemptTTL), now)
		return err
	})
}

// CompleteAccountLink は nonce の消費、「その LINE が別のアカウントに連携済みでないか」の確認、連携の書き込みを
// 1つのトランザクションで行う。分けると、確認と書き込みの間に別の連携が割り込んで上書きされうる。
func CompleteAccountLink(ctx context.Context, db *sql.DB, nonce, lineUserID string, now time.Time) (LinkResult, error) {
	var result LinkResult
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		// FOR UPDATE で nonce の行を押さえる。同じ nonce が同時に届いても、2つ目は1つ目の COMMIT を待ち、
		// そのときには行が消えているので期限切れ扱いになる。期限は DB の時計（UTC）で判定する。
		var userID string
		var expired bool
		err := tx.QueryRowContext(ctx,
			"SELECT userId, expiresAt <= UTC_TIMESTAMP(3) FROM LineLinkNonce WHERE nonce = ? FOR UPDATE", nonce,
		).Scan(&userID, &expired)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && expired) {
			result = Expired
			return nil
		}
		if err != nil {
			return err
		}
		// nonce は使い捨て。別のアカウントに連携済みで断るときも消す。
		if _, err := tx.ExecContext(ctx, "DELETE FROM LineLinkNonce WHERE nonce = ?", nonce); err != nil {
			return err
		}
		taken, err := linkedToOtherUser(ctx, tx, lineUserID, userID)
		if err != nil {
			return err
		}
		if taken {
			result = Taken
			return nil
		}
		result = Linked
		return linkConnection(ctx, tx, userID, lineUserID, now)
	})
	return result, err
}

// LinkVerifiedLineUser は LINE Login で確かめた LINE と連携する。false は別のアカウントに連携済み。
func LinkVerifiedLineUser(ctx context.Context, db *sql.DB, userID, lineUserID string, now time.Time) (bool, error) {
	linked := false
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		taken, err := linkedToOtherUser(ctx, tx, lineUserID, userID)
		if err != nil || taken {
			return err
		}
		linked = true
		return linkConnection(ctx, tx, userID, lineUserID, now)
	})
	return linked, err
}

// Disconnect は連携を解除する。連携が消えたのに LINE 通知だけ ON のままだと、送り先の無い通知が残るので一緒に落とす。
// 連携の行を先に消す。SavePreference は連携の行 → 設定の行の順にロックするので、順をそろえて互いに待ち合わせないようにする。
func Disconnect(ctx context.Context, db *sql.DB, userID string, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{"DELETE FROM LineConnection WHERE userId = ?", []any{userID}},
			{"UPDATE NotificationPreference SET lineMorningEnabled = FALSE, lineEveningEnabled = FALSE, updatedAt = ? WHERE userId = ?",
				[]any{now, userID}},
			{"DELETE FROM LineLinkNonce WHERE userId = ?", []any{userID}},
			{"DELETE FROM LineOAuthAttempt WHERE userId = ?", []any{userID}},
		} {
			if _, err := tx.ExecContext(ctx, q.query, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
}

// StartOAuthAttempt は LINE Login の進行中の試行を1つだけ持たせる。
func StartOAuthAttempt(ctx context.Context, db *sql.DB, state string, a Attempt, now time.Time) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM LineOAuthAttempt WHERE userId = ?", a.UserID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO LineOAuthAttempt (state, userId, nonce, codeVerifier, redirectUri, expiresAt, createdAt)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			state, a.UserID, a.Nonce, a.CodeVerifier, a.RedirectURI, now.Add(AttemptTTL), now)
		return err
	})
}

// DiscardOAuthAttempt は使い終わった・使えなかった state を捨てる。
// 取得と削除の間に別のリクエストが消していても、0行の削除で終わるだけで落ちない。
func DiscardOAuthAttempt(ctx context.Context, db *sql.DB, state string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM LineOAuthAttempt WHERE state = ?", state)
	return err
}

// MarkWebhookEvent は処理する前のイベントに印を入れる。同じ ID が既にあれば duplicate が true。
// ついでに保持期間を過ぎた印を少しずつ消す（1回に100行まで）。
func MarkWebhookEvent(ctx context.Context, db *sql.DB, eventID string, now time.Time) (duplicate bool, err error) {
	if _, err := db.ExecContext(ctx,
		"DELETE FROM LineWebhookEvent WHERE createdAt < ? LIMIT 100", now.Add(-WebhookEventRetention),
	); err != nil {
		return false, fmt.Errorf("clean line webhook events: %w", err)
	}
	_, err = db.ExecContext(ctx, "INSERT INTO LineWebhookEvent (webhookEventId) VALUES (?)", eventID)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("mark line webhook event: %w", err)
	}
	return false, nil
}

// UnmarkWebhookEvent は処理できなかったイベントの印を消し、LINE の再送で処理し直せるようにする。
func UnmarkWebhookEvent(ctx context.Context, db *sql.DB, eventID string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM LineWebhookEvent WHERE webhookEventId = ?", eventID)
	return err
}

// linkedToOtherUser は、その LINE が自分以外のアカウントに連携済みか。
func linkedToOtherUser(ctx context.Context, tx *sql.Tx, lineUserID, userID string) (bool, error) {
	var owner string
	err := tx.QueryRowContext(ctx, "SELECT userId FROM LineConnection WHERE lineUserId = ?", lineUserID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return owner != userID, nil
}

// linkConnection は userID の連携を lineUserID に向ける。無ければ作る。
//
// INSERT ... ON DUPLICATE KEY UPDATE は使わない。このテーブルは userId と lineUserId の2つが UNIQUE で、
// ON DUPLICATE KEY はどちらの重複でも発動する。lineUserId が別ユーザーの行とぶつかると、エラーにならず、
// その他人の行を更新してしまう。userId で UPDATE し、1行も変わらなければ INSERT する（Node と同じ）。
// INSERT 側で lineUserId がぶつかれば ER_DUP_ENTRY になり、トランザクションごと取り消される。
func linkConnection(ctx context.Context, tx *sql.Tx, userID, lineUserID string, now time.Time) error {
	res, err := tx.ExecContext(ctx,
		"UPDATE LineConnection SET lineUserId = ?, linkedAt = ?, updatedAt = ? WHERE userId = ?",
		lineUserID, now, now, userID)
	if err != nil {
		return err
	}
	// MySQL の affectedRows は「値が変わった行」の数。同じ LINE への付け直しでも linkedAt が変わるので 1 になる。
	if n, err := res.RowsAffected(); err != nil || n > 0 {
		return err
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO LineConnection (userId, lineUserId, linkedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?)",
		userID, lineUserID, now, now, now)
	return err
}
