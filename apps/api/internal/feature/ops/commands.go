// Package ops は運用のコマンド（incident・grant-admin・chaos）。サーバーとしては動かず、internal/app の cli.go が引数を見て呼ぶ。
// 変える操作は internal/write/account を呼び、OpsAuditLog に残す。
package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
)

const incidentUsage = `使い方: incident <操作> [メールアドレス]
  sessions <メール>   ログイン中のセッションを見る
  revoke <メール>     セッションをすべて消す
  ban <メール>        止めて、セッションをすべて消す
  unban <メール>      止めたのを戻す
  revoke-admins       管理者全員のセッションを消す
  revoke-all          全員のセッションを消す（全員がログインし直し）
  reset-2fa <メール>  2段階認証を設定前に戻し、セッションをすべて消す
  log                 運用コマンドで変えたことの記録を、新しい順に50件見る`

// chaos stop はデプロイの前後に .github/scripts/deploy-ec2.sh が呼ぶ（JUK-176）。デプロイ中に障害を起こさないため。
const chaosUsage = `使い方: chaos stop   実行中の障害注入の実験を止める（デプロイが呼ぶ）`

const grantAdminUsage = "使い方: grant-admin <メールアドレス> [--revoke] / grant-admin --list"

// opsLogLimit は incident log が出す件数。運用コマンドはめったに使わないので、これで1年分に足りる見込み。
const opsLogLimit = 50

// Run は incident・grant-admin を db に対して動かし、終了コードを返す。
func Run(ctx context.Context, db *sql.DB, args []string, stdout, stderr io.Writer) int {
	return dispatchCommand(ctx, incidentStore{db: db, now: time.Now}, args, stdout, stderr)
}

// dispatchCommand は DB を開いた後の本体。テストは本物の DB の incidentStore を渡して呼ぶ。
func dispatchCommand(ctx context.Context, st incidentStore, args []string, stdout, stderr io.Writer) int {
	var err error
	switch args[0] {
	case "incident":
		err = runIncident(ctx, st, args[1:], stdout)
	case "grant-admin":
		err = runGrantAdmin(ctx, st, args[1:], stdout)
	case "chaos":
		err = runChaos(ctx, st, args[1:], stdout)
	default:
		err = usageError(fmt.Sprintf("知らないコマンドです: %s（incident・grant-admin・chaos・migrate）", args[0]))
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(stderr, err)
	return 1
}

type usageError string

func (e usageError) Error() string { return string(e) }

// userNotFoundError は、いないメールアドレスを渡されたときの文言。
func userNotFoundError(email string) error {
	return fmt.Errorf("%s のユーザーが見つかりません", email)
}

func runIncident(ctx context.Context, st incidentStore, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usageError(incidentUsage)
	}
	command := args[0]
	switch command {
	case "revoke-admins":
		admins, removed, err := st.revokeAdmins(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "管理者 %d 人のセッションを %d 件消しました\n", len(admins), removed)
		for _, a := range admins {
			fmt.Fprintf(out, "  %s\n", a)
		}
		return nil
	case "revoke-all":
		removed, err := st.revokeAll(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "全員のセッションを %d 件消しました\n", removed)
		return nil
	case "log":
		entries, err := st.listOps(ctx, opsLogLimit)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Fprintln(out, "記録はありません")
		}
		for _, e := range entries {
			fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\n", e.CreatedAt, e.Action, orDash(e.TargetID), e.Host, e.Detail)
		}
		return nil
	}

	if len(args) < 2 {
		return usageError(incidentUsage)
	}
	email := args[1]
	switch command {
	case "sessions":
		u, sessions, err := st.listSessions(ctx, email)
		if err != nil {
			return describeNotFound(err, email)
		}
		banned := ""
		if u.BannedAt != nil {
			banned = "（停止中: " + *u.BannedAt + "）"
		}
		fmt.Fprintf(out, "%s  role=%s%s  セッション %d 件\n", email, u.Role, banned, len(sessions))
		for _, s := range sessions {
			twoFactor := "2FAなし"
			if s.TwoFactorVerified {
				twoFactor = "2FA済み"
			}
			fmt.Fprintf(out, "  作成 %s  最終 %s  期限 %s  %s  %s  %s\n",
				s.CreatedAt, s.LastUsedAt, s.ExpiresAt, twoFactor, orDash(s.IPAddress), orDash(s.UserAgent))
		}
	case "revoke":
		removed, err := st.revokeSessions(ctx, email)
		if err != nil {
			return describeNotFound(err, email)
		}
		fmt.Fprintf(out, "%s のセッションを %d 件消しました\n", email, removed)
	case "ban":
		removed, err := st.ban(ctx, email)
		if err != nil {
			return describeNotFound(err, email)
		}
		fmt.Fprintf(out, "%s を止め、セッションを %d 件消しました\n", email, removed)
	case "unban":
		if err := st.unban(ctx, email); err != nil {
			return describeNotFound(err, email)
		}
		fmt.Fprintf(out, "%s の停止を戻しました\n", email)
	case "reset-2fa":
		removed, err := st.resetTwoFactor(ctx, email)
		if err != nil {
			return describeNotFound(err, email)
		}
		fmt.Fprintf(out, "%s の2段階認証を設定前に戻し、セッションを %d 件消しました\n", email, removed)
	default:
		return usageError(incidentUsage)
	}
	return nil
}

func runGrantAdmin(ctx context.Context, st incidentStore, args []string, out io.Writer) error {
	if slices.Contains(args, "--list") {
		admins, err := st.listAdmins(ctx)
		if err != nil {
			return err
		}
		if len(admins) == 0 {
			fmt.Fprintln(out, "管理者はいません")
		}
		for _, a := range admins {
			twoFactor, password := "2段階認証: 未設定", "パスワード: なし"
			if a.TwoFactorEnabled {
				twoFactor = "2段階認証: 有効"
			}
			if a.HasPassword {
				password = "パスワード: あり"
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", a.Email, twoFactor, password)
		}
		return nil
	}

	email := ""
	for _, a := range args {
		if len(a) > 0 && a[0] != '-' {
			email = a
			break
		}
	}
	if email == "" {
		return usageError(grantAdminUsage)
	}
	role := "admin"
	if slices.Contains(args, "--revoke") {
		role = "user"
	}
	previous, removed, err := st.setRole(ctx, email, role)
	if errors.Is(err, account.ErrUnverified) {
		return fmt.Errorf("%s はメール確認が済んでいないため、管理者にしません", email)
	}
	if err != nil {
		return describeNotFound(err, email)
	}
	fmt.Fprintf(out, "%s の role を %s → %s にし、セッションを %d 件消しました（ログインし直してください）\n",
		email, previous, role, removed)
	return nil
}

func describeNotFound(err error, email string) error {
	// 引いた後に消えていれば、account の操作が ErrNotFound を返す。
	if errors.Is(err, errUserNotFound) || errors.Is(err, account.ErrNotFound) {
		return userNotFoundError(email)
	}
	return err
}

func orDash(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}

// runChaos は実行中の実験を止める。デプロイのログは公開されるので、止めた数（実験があったか）は出さない（JUK-178）。
func runChaos(ctx context.Context, st incidentStore, args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "stop" {
		return usageError(chaosUsage)
	}
	// DATETIME(3) に書くので、ミリ秒で切り捨てる（dates.NowMillis と同じ）。
	if _, err := writechaos.StopAll(ctx, st.db, writechaos.StoppedByDeploy, st.now().UTC().Truncate(time.Millisecond)); err != nil {
		return err
	}
	fmt.Fprintln(out, "実行中の実験があれば止めました")
	return nil
}
