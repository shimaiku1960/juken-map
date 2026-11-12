// Package simulation は負荷のシミュレーション（sim/、/admin/sim）の利用者の印（UserSimulation）への書き込みの持ち主。
// 持ち主の一覧と決まりは docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
// 印は user と 1対1 の UserSimulation に持つ（JUK-228）。この表に行がある利用者がシミュレーション用。
// 前は user の列（simSeq など）に持っていた。その列は読み書きしなくなり、次のデプロイで消す。
//
// 触れる相手はシミュレーション用のメールアドレス（delivered+simNNNNN@resend.dev）だけ。どの SQL もアドレスの形を
// WHERE に入れる。UserSimulation に行があるかだけで絞ると、何かの間違いで実ユーザーに行ができたときに触ってしまう。
//
// 入力の形の確かめは入口が行い、ここには確かめ済みの値が来る。
package simulation

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// EmailLike は SQL の LIKE で「シミュレーションの利用者」だけを選ぶ条件。src/shared/synthetic.ts と同じ。
const EmailLike = "delivered+sim%@resend.dev"

// 操作が断る理由。Error() は利用者に見せる文言。
var (
	// ErrNotFound は、シミュレーションの利用者が見つからないこと。
	ErrNotFound = errors.New("ユーザーが見つかりません")
	// ErrDuplicate は、連番（UNIQUE）がもう使われていること。
	ErrDuplicate = errors.New("この連番はすでに使われています")
)

// Mark は登録を済ませた合成ユーザーに連番と続き方の型を付ける。付け直すときは、最後に操作した日・来なくなった日を残す。
func Mark(ctx context.Context, db *sql.DB, email string, seq int64, cohort string) error {
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var userID string
		err := tx.QueryRowContext(ctx,
			"SELECT id FROM `user` WHERE email = ? AND email LIKE ? FOR UPDATE", email, EmailLike).Scan(&userID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// 行があれば書き換え、無ければ入れる。INSERT ... ON DUPLICATE KEY UPDATE は、連番がほかの利用者の行と
		// ぶつかったときにその行のほうを書き換えてしまうため使わない。こう書けば、ぶつかったときは DuplicateEntry で止まる。
		res, err := tx.ExecContext(ctx, "UPDATE UserSimulation SET seq = ?, cohort = ? WHERE userId = ?", seq, cohort, userID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n > 0 {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO UserSimulation (userId, seq, cohort) VALUES (?, ?, ?)", userID, seq, cohort)
		return err
	})
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return ErrDuplicate
	}
	return err
}

// Activity は最後に操作した日・来なくなった日（"YYYY-MM-DD"）。Present の項目だけを書き換え、Value が nil なら NULL にする。
type Activity struct {
	LastActedOn opt.Field[string]
	DormantFrom opt.Field[string]
}

// RecordActivity は連番の利用者に Activity を記録する。
// 変える項目が無ければ、その連番が無くても成功にする（SQL を流さない）。
func RecordActivity(ctx context.Context, db *sql.DB, seq int64, a Activity) error {
	var sets []string
	var args []any
	for _, c := range []struct {
		column string
		field  opt.Field[string]
	}{
		{"s.lastActedOn", a.LastActedOn},
		{"s.dormantFrom", a.DormantFrom},
	} {
		if c.field.Present {
			sets = append(sets, c.column+" = ?")
			args = append(args, c.field.Value)
		}
	}
	if len(sets) == 0 {
		return nil
	}
	// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（sets）。値は args で ? として渡す
	res, err := db.ExecContext(ctx,
		"UPDATE UserSimulation s JOIN `user` u ON u.id = s.userId SET "+strings.Join(sets, ", ")+
			" WHERE s.seq = ? AND u.email LIKE ?",
		append(args, seq, EmailLike)...)
	if err != nil {
		return err
	}
	return foundOrErr(res)
}

// foundOrErr は UPDATE が1行も当たらなければ ErrNotFound。
func foundOrErr(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
