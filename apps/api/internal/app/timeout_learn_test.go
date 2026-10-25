//go:build learn

// 学習 3-1（JUK-212）の実験。1リクエストの時間の上限（requestTimeout）と、サーバーの WriteTimeout の
// 大小を入れ替えると、呼び出し側に何が届くかを見る。`-tags learn` を付けたときだけ動き、CI では動かない。
// DB には繋がない。
//
//	go test -tags learn -run TestLearnTimeoutOrder -v ./internal/app/
package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// slowHandler は、DB の照会のように ctx の取り消しで止まる仕事を work だけ続ける。
// 間に合えば 200、先に ctx が取り消されたら 500 を返す（本物のハンドラも、照会が ctx で止まると 500 にする）。
func slowHandler(work time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(work):
			_, _ = w.Write([]byte("ok"))
		case <-r.Context().Done():
			http.Error(w, "stopped: "+r.Context().Err().Error(), http.StatusInternalServerError)
		}
	})
}

// callOnce は、上限 deadline と WriteTimeout writeTimeout のサーバーを立て、work かかる仕事を1回呼ぶ。
// 呼び出し側から見えたもの（ステータスか、接続のエラー）と、かかった時間を記録する。
func callOnce(t *testing.T, deadline, writeTimeout, work time.Duration) {
	t.Helper()
	srv := httptest.NewUnstartedServer(withDeadline(deadline, slowHandler(work)))
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	defer srv.Close()

	// 呼び出し側には上限を付けない（nginx や CloudFront がもっと長く待っている状態）。
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	t.Logf("上限 %s / WriteTimeout %s / 仕事 %s", deadline, writeTimeout, work)
	start := time.Now()
	res, err := client.Get(srv.URL)
	elapsed := time.Since(start).Round(100 * time.Millisecond)
	if err != nil {
		t.Logf("→ %s 後に、応答なしで接続が切れた: %v", elapsed, err)
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	t.Logf("→ %s 後に %d が返った: %q", elapsed, res.StatusCode, body)
}

func TestLearnTimeoutOrder(t *testing.T) {
	// 今の値（server.go の requestTimeout 10秒 < main.go の WriteTimeout 15秒）。仕事は12秒かかる。
	t.Run("current", func(t *testing.T) {
		callOnce(t, requestTimeout, 15*time.Second, 12*time.Second)
	})
	// 大小を逆にした値（上限20秒 > WriteTimeout 15秒）。仕事は18秒かかる。
	t.Run("reversed", func(t *testing.T) {
		callOnce(t, 20*time.Second, 15*time.Second, 18*time.Second)
	})
}

// TestLearnNestedTimeout は、外側の上限（requestTimeout 10秒）の内側で、もっと長い上限を作ったときに
// どちらが効くかを見る。AI クイック記録の解析（internal/feature/study の study_log_parse.go）と同じ書き方で、
// r.Context() から parseTimeout 20秒の ctx を作り、12秒かかる Claude の応答を待つ。
//
//	go test -tags learn -run TestLearnNestedTimeout -v ./internal/app/
func TestLearnNestedTimeout(t *testing.T) {
	const parseTimeout = 20 * time.Second // study_log_parse.go と同じ値
	claude := 12 * time.Second

	h := withDeadline(requestTimeout, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), parseTimeout)
		defer cancel()
		dl, _ := ctx.Deadline()
		t.Logf("parseTimeout %s で作った ctx の締め切り: %s 先", parseTimeout, time.Until(dl).Round(time.Second))

		start := time.Now()
		select {
		case <-time.After(claude):
			t.Logf("→ %s 後に Claude の応答を受け取れた", time.Since(start).Round(100*time.Millisecond))
		case <-ctx.Done():
			t.Logf("→ %s 後に待つのをやめた: %v", time.Since(start).Round(100*time.Millisecond), ctx.Err())
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/study-logs/parse", nil))
}

// TestLearnShutdownAfterResponse は、止めるとき（main.go の Shutdown）に「応答のあとの仕事」を待つかを見る。
// ログインのメール（internal/feature/auth の email.go）と同じく、応答を返したあとに go f() で3秒かかる送信を始め、
// すぐに Shutdown を呼ぶ。Shutdown が戻ると run が戻り、プロセスが終わる（そのとき残っている goroutine も消える）。
//
//	go test -tags learn -run TestLearnShutdownAfterResponse -v ./internal/app/
func TestLearnShutdownAfterResponse(t *testing.T) {
	var sent atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("受け付けました"))
		go func() {
			time.Sleep(3 * time.Second) // Resend へ送っている間
			sent.Store(true)
		}()
	}))
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	t.Logf("応答: %d", res.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // main.go と同じ値
	defer cancel()
	start := time.Now()
	err = srv.Config.Shutdown(ctx)
	t.Logf("→ Shutdown は %s 待って戻った（err=%v）", time.Since(start).Round(100*time.Millisecond), err)
	t.Logf("→ その時点でメールは送り終えていたか: %v", sent.Load())
}
