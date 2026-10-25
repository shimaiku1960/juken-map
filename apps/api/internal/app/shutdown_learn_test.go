//go:build learn

// 学習 3-2（JUK-214）の実験。本物の API を別のプロセスとして起動し、サインアップを数件送った直後に
// SIGTERM を送って、応答のあとに送るはずだった確認メールが何通届いたかを数える。`-tags learn` を付けたときだけ
// 動き、CI では動かない。Resend は偽物（1通 mailDelay かかり、受け取った数と送り終えた数を数える）に向け、
// DB はテスト用の juken_map_test を使う（`pnpm db:start` と `pnpm --filter @juken-map/db test-db:prepare` が先に要る）。
//
//	go test -tags learn -run TestLearnShutdownDropsMail -v -count=1 ./internal/app/
//
// Resend が遅いときは LEARN_MAIL_DELAY=10s のように変える。
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/authguard"
)

const learnSignUps = 5 // 止める直前に受け付けるサインアップの数

func learnEmail(i int) string { return fmt.Sprintf("learn-3-2-%d@example.com", i) }
func learnIP(i int) string    { return fmt.Sprintf("198.51.100.%d", i+1) }

func TestLearnShutdownDropsMail(t *testing.T) {
	mailDelay := 3 * time.Second // 偽の Resend が1通にかける時間
	if v := os.Getenv("LEARN_MAIL_DELAY"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatal(err)
		}
		mailDelay = d
	}
	t.Logf("偽の Resend は1通に %s かかる", mailDelay)
	db := dbtest.Open(t)
	cleanup := func() {
		ctx := context.Background()
		for i := range learnSignUps {
			sum := sha256.Sum256([]byte(learnEmail(i)))
			_, _ = db.ExecContext(ctx, "DELETE FROM `user` WHERE email = ?", learnEmail(i))
			_, _ = db.ExecContext(ctx, "DELETE FROM EmailSend WHERE recipientHash = ?", hex.EncodeToString(sum[:]))
			_, _ = db.ExecContext(ctx, "DELETE FROM AuthThrottle WHERE bucket = ?", authguard.Bucket(authguard.AnonymousIP, learnIP(i)))
		}
	}
	cleanup() // 前の回が途中で止まって残したものも消す
	t.Cleanup(cleanup)

	// 偽の Resend。送り始めた数（received）と、送り終えて 200 を返した数（delivered）を数える。
	var received, delivered atomic.Int32
	resend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // 読み終えないと、相手が切れても r.Context() が取り消されない
		received.Add(1)
		select {
		case <-time.After(mailDelay):
			delivered.Add(1)
			_, _ = w.Write([]byte(`{"id":"learn"}`))
		case <-r.Context().Done(): // 呼び出し側（API のプロセス）が途中で消えた
		}
	}))
	defer resend.Close()

	// 本物のサーバーをビルドして起動する。
	bin := t.TempDir() + "/api"
	build := exec.Command("go", "build", "-o", bin, "./cmd/api")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	port := freePort(t)
	var logs bytes.Buffer
	api := exec.Command(bin)
	api.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"BETTER_AUTH_SECRET=learn-3-2-secret-learn-3-2-secret",
		"DATABASE_URL=" + dbtest.DefaultURL,
		"PORT=" + port,
		"RESEND_BASE_URL=" + resend.URL,
		"RESEND_API_KEY=learn",
	}
	api.Stdout, api.Stderr = &logs, &logs
	if err := api.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- api.Wait() }()
	defer func() {
		_ = api.Process.Kill()
		if t.Failed() {
			t.Logf("API のログ:\n%s", logs.String())
		}
	}()
	base := "http://127.0.0.1:" + port
	waitHealthy(t, base)

	// サインアップを同時に送り、全部の応答を受け取る。
	var wg sync.WaitGroup
	for i := range learnSignUps {
		wg.Go(func() {
			body, _ := json.Marshal(map[string]string{
				"email": learnEmail(i), "password": "learn-3-2-Passw0rd!", "callbackURL": "/dashboard",
			})
			req, _ := http.NewRequest(http.MethodPost, base+"/api/auth/sign-up", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", learnIP(i)) // IP ごとの上限に当たらないよう、1件ずつ変える
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("sign-up %d: %v", i, err)
				return
			}
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Errorf("sign-up %d: %d", i, res.StatusCode)
			}
		})
	}
	wg.Wait()
	t.Logf("サインアップ %d 件に 200 が返った（利用者には「確認メールを送りました」と見えている）", learnSignUps)

	// すぐに止める（デプロイの docker stop と同じ SIGTERM）。
	start := time.Now()
	if err := api.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		t.Logf("→ SIGTERM から %s でプロセスが終わった（err=%v）", time.Since(start).Round(100*time.Millisecond), err)
	case <-time.After(30 * time.Second):
		t.Fatal("30秒たってもプロセスが終わらない")
	}

	// 送りかけの分が届くなら届き終わる時間を待ってから数える。
	time.Sleep(mailDelay + time.Second)
	t.Logf("→ 偽の Resend が受け取った: %d 通 / 送り終えた: %d 通（%d 通送るはずだった）",
		received.Load(), delivered.Load(), learnSignUps)
	t.Logf("→ API のログに残った送信の失敗: %d 件 / 待ちきれずに残した仕事: %d 回",
		strings.Count(logs.String(), "[auth-email] Failed to send."),
		strings.Count(logs.String(), "[shutdown] Background tasks abandoned."))
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func waitHealthy(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if res, err := http.Get(base + "/api/health"); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("API が20秒以内に起動しない")
}
