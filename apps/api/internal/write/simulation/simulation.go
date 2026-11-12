// Package simulation は負荷のシミュレーション（sim/、/admin/sim）の利用者の印（user.simSeq・simCohort・
// simLastActedOn・simDormantFrom と、UserSimulation）への書き込みの持ち主。持ち主の一覧と決まりは docs/architecture.md
// 「バックエンドの構成」（JUK-148・JUK-150）。
//
// 印は user の列から 1対1 の UserSimulation へ移している途中（JUK-228）。今は user の列が正で、書き込みのたびに
// その利用者の行を UserSimulation へ写す（copyToTable）。次の段階で UserSimulation だけを読み書きする。
//
// 触れる相手はシミュレーション用のメールアドレス（delivered+simNNNNN@resend.dev）だけ。どの SQL もアドレスの形を
// WHERE に入れる。simSeq が付いているかだけで絞ると、何かの間違いで実ユーザーに simSeq が付いたときに触ってしまう。
//
// 入力の形の確かめは入口が行い、ここには確かめ済みの値が来る。
package simulation

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

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

// Mark は登録を済ませた合成ユーザーに連番と続き方の型を付ける。
func Mark(ctx context.Context, db *sql.DB, email string, seq int64, cohort string, now time.Time) error {
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			"UPDATE `user` SET simSeq = ?, simCohort = ?, updatedAt = ? WHERE email = ? AND email LIKE ?",
			seq, cohort, now, email, EmailLike)
		if err != nil {
			return err
		}
		if err := foundOrErr(res); err != nil {
			return err
		}
		return copyToTable(ctx, tx, "email = ?", email)
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
		{"simLastActedOn", a.LastActedOn},
		{"simDormantFrom", a.DormantFrom},
	} {
		if c.field.Present {
			sets = append(sets, c.column+" = ?")
			args = append(args, c.field.Value)
		}
	}
	if len(sets) == 0 {
		return nil
	}
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（sets）。値は args で ? として渡す
		res, err := tx.ExecContext(ctx,
			"UPDATE `user` SET "+strings.Join(sets, ", ")+" WHERE simSeq = ? AND email LIKE ?",
			append(args, seq, EmailLike)...)
		if err != nil {
			return err
		}
		if err := foundOrErr(res); err != nil {
			return err
		}
		return copyToTable(ctx, tx, "simSeq = ?", seq)
	})
}

// copyToTable は、user の列の印を UserSimulation へ写す（JUK-228 の「足す」の段階）。where は user を1人に絞る
// 固定の条件で、値は arg で渡す。差分ではなく行ごと写し直すので、この段階の前に印が付いた利用者も、
// 次に書き込んだときに UserSimulation へ入る。
//
// 一度消してから入れる。INSERT ... ON DUPLICATE KEY UPDATE は、連番がほかの利用者の行とぶつかったときに
// その行のほうを書き換えてしまうため使わない。消してから入れれば、ぶつかったときは DuplicateEntry で止まる。
func copyToTable(ctx context.Context, tx *sql.Tx, where string, arg any) error {
	// #nosec G202 -- where は呼び出し元に書いた固定の条件だけ。値は arg で ? として渡す
	if _, err := tx.ExecContext(ctx,
		"DELETE s FROM UserSimulation s JOIN `user` u ON u.id = s.userId WHERE u."+where+" AND u.email LIKE ?",
		arg, EmailLike); err != nil {
		return err
	}
	// #nosec G202 -- 同上
	_, err := tx.ExecContext(ctx,
		"INSERT INTO UserSimulation (userId, seq, cohort, dormantFrom, lastActedOn)"+
			" SELECT id, simSeq, simCohort, simDormantFrom, simLastActedOn FROM `user`"+
			" WHERE "+where+" AND email LIKE ? AND simSeq IS NOT NULL AND simCohort IS NOT NULL",
		arg, EmailLike)
	return err
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
