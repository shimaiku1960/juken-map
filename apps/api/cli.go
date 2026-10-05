package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"
)

// 引数を付けて起動したときのコマンド（JUK-122）。サーバーとしては起動せず、DB に繋いで1つの操作をして終わる。
// 本番は RDS に外から繋げないので、EC2 で動いている Go のコンテナの中で実行する（distroless でシェルが無いので
// 実行ファイルを直接呼ぶ）。コンテナの DATABASE_URL をそのまま使う。
//
//	sudo docker exec juken-map-go /api incident sessions <メールアドレス>
//	sudo docker exec juken-map-go /api grant-admin --list
//
// 手元では pnpm incident・pnpm admin:grant（scripts/go-cli.sh）が .env を読んで同じものを呼ぶ。手順は docs/incident-response.md。
// 変える操作は、実行の内容を OpsAuditLog に残す（incident.go の recordOps、JUK-138）。`incident log` で見る。
// migrate（migrate.go）だけは本番でもデプロイが1回きりのコンテナで流し、手元では pnpm db:migrate が呼ぶ。

const incidentUsage = `使い方: incident <操作> [メールアドレス]
  sessions <メール>   ログイン中のセッションを見る
  revoke <メール>     セッションをすべて消す
  ban <メール>        止めて、セッションをすべて消す
  unban <メール>      止めたのを戻す
  revoke-admins       管理者全員のセッションを消す
  revoke-all          全員のセッションを消す（全員がログインし直し）
  reset-2fa <メール>  2段階認証を設定前に戻し、セッションをすべて消す
  log                 運用コマンドで変えたことの記録を、新しい順に50件見る`

const grantAdminUsage = "使い方: grant-admin <メールアドレス> [--revoke] / grant-admin --list"

// opsLogLimit は incident log が出す件数。運用コマンドはめったに使わないので、これで1年分に足りる見込み。
const opsLogLimit = 50

// cliTimeout は1回の操作にかけてよい時間。DB が詰まっていても、手順の途中で固まらないように。
const cliTimeout = 30 * time.Second

// runCommand は args（os.Args[1:]）の操作をして、終了コードを返す。
func runCommand(args []string, stdout, stderr io.Writer) int {
	// migrate（migrate.go）は繋ぐユーザー・接続の設定・かけてよい時間がほかと違うので、DB を開く前に分ける。
	if args[0] == "migrate" {
		return runMigrate(stdout, stderr)
	}
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
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
	default:
		err = usageError(fmt.Sprintf("知らないコマンドです: %s（incident・grant-admin・migrate）", args[0]))
	}
	if err == nil {
		return 0
	}
	fmt.Fprintln(stderr, err)
	return 1
}

type usageError string

func (e usageError) Error() string { return string(e) }

// userNotFoundError は、いないメールアドレスを渡されたときの文言（Node の CLI と同じ）。
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
	if errors.Is(err, errUnverified) {
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
	if errors.Is(err, errUserNotFound) {
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
