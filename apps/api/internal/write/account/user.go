package account

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 利用者の行：外部ログインの連携（作成を含む）・削除・ニックネーム・登録の計測の印。

// Identity は外部ログイン（Google・GitHub）で確かめた相手。メールアドレスは確認済みのものだけが来る。
type Identity struct {
	Provider string
	Subject  string // Google の sub、GitHub の数値の ID
	Email    string
	Name     string
	Image    string
}

// LinkEvent は LinkOAuthIdentity が何をしたか。
type LinkEvent string

const (
	LinkCreated LinkEvent = "created" // 利用者を新しく作った
	LinkLinked  LinkEvent = "linked"  // 確認済みの利用者に結びつけた
	// LinkClaimedUnverified は、未確認の利用者のパスワード・セッション・トークンを消してから結びつけ、確認済みにした。
	// 他人のメールアドレスで先に登録しただけの人に、本人のアカウントを使わせないため。
	LinkClaimedUnverified LinkEvent = "claimed_unverified"
)

// LinkOAuthIdentity は、メールアドレスが同じ利用者に外部ログインを結びつける。同じメールアドレスの利用者の行を
// ロックして（FOR UPDATE）、1つのトランザクションで LinkEvent のどれかをする。
func LinkOAuthIdentity(ctx context.Context, db *sql.DB, ident Identity, now time.Time) (userID string, event LinkEvent, err error) {
	err = database.InTx(ctx, db, func(tx *sql.Tx) error {
		var verified bool
		err := tx.QueryRowContext(ctx, "SELECT id, emailVerified FROM `user` WHERE email = ? FOR UPDATE", ident.Email).Scan(&userID, &verified)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			userID = NewUserID()
			name := ident.Name
			if name == "" {
				name = ident.Email
			}
			var image any
			if ident.Image != "" {
				image = truncate(ident.Image, 191)
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO `user` (id, name, email, image, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, ?, true, ?, ?)",
				userID, truncate(name, 191), ident.Email, image, now, now); err != nil {
				return err
			}
			event = LinkCreated
		case err != nil:
			return err
		case verified:
			event = LinkLinked
		default:
			for _, q := range []string{
				"DELETE FROM AuthPassword WHERE userId = ?",
				"DELETE FROM AuthSession WHERE userId = ?",
				"DELETE FROM AuthToken WHERE userId = ?",
			} {
				if _, err := tx.ExecContext(ctx, q, userID); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, "UPDATE `user` SET emailVerified = true, updatedAt = ? WHERE id = ?", now, userID); err != nil {
				return err
			}
			event = LinkClaimedUnverified
		}
		_, err = tx.ExecContext(ctx,
			"INSERT INTO AuthIdentity (provider, providerUserId, userId, createdAt) VALUES (?, ?, ?, ?)",
			ident.Provider, ident.Subject, userID, now)
		return err
	})
	if err != nil {
		return "", "", err
	}
	return userID, event, nil
}

// NewUserID は利用者の ID を作る（16バイトの乱数の16進）。
func NewUserID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Removed は DeleteUser で一緒に消えた、本人のデータの数（管理画面に出す）。
type Removed struct {
	FinalGoals int
	StudyLogs  int
	StudyPlans int
	Textbooks  int
}

// DeleteUser は利用者を消す。本人の退会と管理者の削除の両方が使う。いなければ ErrNotFound。
//
// 利用者を指す表は、すべて外部キーの ON DELETE CASCADE で一緒に消える。すべての表に本人の行が
// 残らないことは package main の TestAuthDBDeleteLeavesNoRows が確かめる。
func DeleteUser(ctx context.Context, db *sql.DB, userID string) (Removed, error) {
	var c Removed
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT id FROM `user` WHERE id = ? FOR UPDATE", userID).Scan(new(string)); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT (SELECT COUNT(*) FROM FinalGoal WHERE userId = ?),
			        (SELECT COUNT(*) FROM StudyLog WHERE userId = ?),
			        (SELECT COUNT(*) FROM StudyPlan WHERE userId = ?),
			        (SELECT COUNT(*) FROM Textbook WHERE userId = ?)`,
			userID, userID, userID, userID,
		).Scan(&c.FinalGoals, &c.StudyLogs, &c.StudyPlans, &c.Textbooks); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM `user` WHERE id = ?", userID)
		return err
	})
	if err != nil {
		return Removed{}, err
	}
	return c, nil
}

// SetNickname はニックネームを書き換える。
func SetNickname(ctx context.Context, db *sql.DB, userID, nickname string, now time.Time) error {
	_, err := db.ExecContext(ctx, "UPDATE `user` SET nickname = ?, updatedAt = ? WHERE id = ?", nickname, now, userID)
	return err
}

// MarkSignUpTracked は登録を計測した印を付ける。付けられたら（初回なら）true。
// 印が無い行だけを UPDATE するので、同時に2回来ても true になるのは1回だけ。
func MarkSignUpTracked(ctx context.Context, db *sql.DB, userID string, now time.Time) (bool, error) {
	return updatedOne(db.ExecContext(ctx,
		"UPDATE `user` SET analyticsSignUpTrackedAt = ?, updatedAt = ? WHERE id = ? AND analyticsSignUpTrackedAt IS NULL",
		now, now, userID))
}

// truncate は UTF-8 を壊さずに n バイト以内に切る（列の長さに収める）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
