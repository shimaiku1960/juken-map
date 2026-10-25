// Package background は、応答を返したあとに動かす仕事（ログインのメールなど）を数え、止めるときに
// 終わるまで待つ。http.Server.Shutdown は処理中のリクエストしか待たないので、素の go f() で始めた仕事は、
// プロセスが終わると途中で消え、ログも残らない（JUK-214）。
package background

import (
	"context"
	"log/slog"
	"sync"
)

// Tasks は動いている仕事の数を持つ。ゼロ値のまま使える。
type Tasks struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	running int
	closed  bool
}

// Go は f を goroutine で動かし、Wait が待つ数に入れる。
// Wait を呼んだあとは、まだ動いている仕事が始めた続きの仕事（パスワード再設定の準備→メールなど）だけを受ける。
// 動いている仕事が無くなってから来たものは、待つ人がもういないので動かさず、ログに残す。
func (t *Tasks) Go(f func()) {
	t.mu.Lock()
	if t.closed && t.running == 0 {
		t.mu.Unlock()
		slog.Error("[background] Task rejected: shutting down.")
		return
	}
	t.running++
	t.wg.Add(1)
	t.mu.Unlock()
	go func() {
		defer t.done()
		f()
	}()
}

func (t *Tasks) done() {
	t.mu.Lock()
	t.running--
	t.mu.Unlock()
	t.wg.Done()
}

// Wait は新しい仕事を締め切り、動いている仕事が全部終わるか、ctx が終わるまで待つ。
// 戻り値は待ちきれずに残した数（0 なら全部終わった）。残した仕事は、プロセスが終わると途中で消える。
func (t *Tasks) Wait(ctx context.Context) int {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()

	idle := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(idle)
	}()
	select {
	case <-idle:
		return 0
	case <-ctx.Done():
		t.mu.Lock()
		defer t.mu.Unlock()
		return t.running
	}
}
