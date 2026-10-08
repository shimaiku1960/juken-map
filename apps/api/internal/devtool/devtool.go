// Package devtool は開発でしか使わない道具（cmd/devtool）。seed・E2E・負荷試験の下ごしらえで、
// ログインと同じ作り方のパスワードのハッシュ・セッション・メールのトークンを作る（JUK-143）。
// 作り方は internal/feature/auth の本物の関数に任せ、ここでは引数と出力の形だけを決める。
//
//	hash-password                    標準入力のパスワードのハッシュを出す（DB に繋がない）
//	email-token <メール> <用途>      verify-email か password-reset のトークンを発行して出す
//	sessions <User-Agent>           標準入力の利用者 ID（1行に1つ）ごとにセッションを作り、Cookie を1行ずつ出す
//
// DB は環境変数の DATABASE_URL に繋ぐ（scripts/go-devtool.sh が .env を読む）。
package devtool

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/auth"
)

const usage = `使い方: devtool <操作>
  hash-password                 標準入力のパスワードのハッシュを出す
  email-token <メール> <用途>   verify-email か password-reset のトークンを出す
  sessions <User-Agent>         標準入力の利用者 ID ごとにセッションを作り、Cookie を出す`

// timeout は1回の操作にかけてよい時間。負荷試験のセッション（既定 500 人）を作り切れる長さにする。
const timeout = 2 * time.Minute

// sessionIP はセッションに残す接続元。HTTP を通さずに作ったものだと分かるように、手元の値にする。
const sessionIP = "127.0.0.1"

// Main は args（os.Args[1:]）の操作をして、終了コードを返す。
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := run(ctx, args, stdin, stdout); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	// ハッシュは DB に繋がずに作る（seed が DB へ書く前に呼ぶ）。
	if args[0] == "hash-password" {
		return hashPassword(ctx, stdin, stdout)
	}
	db, err := database.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()
	switch {
	case args[0] == "email-token" && len(args) == 3:
		return emailToken(ctx, db, args[1], args[2], stdout)
	case args[0] == "sessions" && len(args) == 2:
		return sessions(ctx, db, args[1], stdin, stdout)
	default:
		return errors.New(usage)
	}
}

// hashPassword は標準入力をそのままパスワードにする（コマンドラインに出すと ps で見える）。
// echo で渡したときの末尾の改行は1つだけ取る。
func hashPassword(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	b, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if password == "" {
		return errors.New("パスワードを標準入力で渡してください")
	}
	hash, err := auth.HashPassword(ctx, password)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, hash)
	return err
}

func emailToken(ctx context.Context, db *sql.DB, email, purpose string, stdout io.Writer) error {
	var userID string
	err := db.QueryRowContext(ctx, "SELECT id FROM `user` WHERE email = ?", email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s のユーザーが見つかりません", email)
	}
	if err != nil {
		return err
	}
	token, err := auth.IssueEmailToken(ctx, db, userID, purpose)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, token)
	return err
}

// sessions は、利用者の role に合わせた期限のセッションを作る（ログインしたときと同じ）。
func sessions(ctx context.Context, db *sql.DB, userAgent string, stdin io.Reader, stdout io.Writer) error {
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		userID := strings.TrimSpace(sc.Text())
		if userID == "" {
			continue
		}
		var role string
		err := db.QueryRowContext(ctx, "SELECT role FROM `user` WHERE id = ?", userID).Scan(&role)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("利用者 %s が見つかりません", userID)
		}
		if err != nil {
			return err
		}
		cookie, err := auth.IssueSession(ctx, db, userID, role, sessionIP, userAgent)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, cookie); err != nil {
			return err
		}
	}
	return sc.Err()
}
