package httpx

import (
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// 利用者単位の回数制限（セキュリティ基準 06 の E2）。ログインしている人が重い API を叩き続けて、
// ほかの利用者まで遅くしたり、DB を詰まらせたりするのを止める。
//
// 認証の回数制限（auth_throttle.go）とは目的が違うので、数え方も変えた。
//   - 認証は総当たりを止めるので、数え漏れが許されない。数は DB に置き、デプロイで Go のコンテナが
//     2つ並ぶあいだも同じ数を見る。
//   - こちらは使いすぎを抑えるだけで、数え漏れても被害は「少し多めに通る」で済む。すべてのリクエストで
//     DB を1往復させる方が高くつくので、数はこのプロセスのメモリに置く。コンテナが並ぶのは切り替えの
//     数秒だけで、その間は最大で上限の2倍まで通りうる。再起動すると数は消える。
//
// 数え方はトークンバケット。利用者ごとに最大 burst 個の札を持ち、1リクエストで1つ使い、毎秒 rate 個ずつ
// 戻る。画面を開いた直後にまとめて飛ぶリクエストは burst で受け、叩き続けると rate で頭打ちになる。
// 「1分に何回」の窓で数える方式と違い、窓の切れ目の前後で2倍通ることが無い。

// rateLimitRule は1つの種類（読み取り・書き込み）の決まり。
type rateLimitRule struct {
	name  string
	burst float64 // 一度に使える数（札の最大）
	rate  float64 // 1秒に戻る数
}

var (
	// 読み取り（GET・HEAD）。1画面で多くて5〜6本なので、120 は画面を20回ほど続けて開ける量。
	// 叩き続けても毎秒2本（1分に120本）まで。手元の負荷試験（500人で毎秒600本＝1人毎秒1.2本）も収まる。
	rateLimitRead = rateLimitRule{name: "read", burst: 120, rate: 2}
	// 書き込み。学習記録を続けて付けても1分に数件なので、30 件を続けて受け、その後は2秒に1件まで。
	rateLimitWrite = rateLimitRule{name: "write", burst: 30, rate: 0.5}
)

// rateLimitSweepEvery は、使われなくなった利用者の札を捨てる間隔。札が満タンに戻った利用者は、
// 消しても次に来たとき満タンから始まるだけで結果が変わらないので捨ててよい。
const rateLimitSweepEvery = time.Minute

const tooManyRequestsMessage = "リクエストが多すぎます。少し待ってからもう一度お試しください"

type tokenBucket struct {
	rule   rateLimitRule
	tokens float64
	at     time.Time // tokens を最後に計算した時刻
}

type userRateLimiter struct {
	now       func() time.Time
	mu        sync.Mutex
	buckets   map[string]*tokenBucket
	lastSweep time.Time
}

func newUserRateLimiter(now func() time.Time) *userRateLimiter {
	return &userRateLimiter{now: now, buckets: map[string]*tokenBucket{}, lastSweep: now()}
}

// allow は札を1つ使えたら true を返す。使えないときは、1つ戻るまでの時間を返す（札は減らさない）。
//
// Go のハンドラは本当に同時に走るので、札の読み書きは mutex の中で行う（limitInFlight のチャネルと同じ理由）。
// 計算は数十ナノ秒なので、全員で1つの mutex を共有しても待ちは問題にならない。
func (l *userRateLimiter) allow(rule rateLimitRule, key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= rateLimitSweepEvery {
		l.sweep(now)
	}

	id := rule.name + ":" + key
	b := l.buckets[id]
	if b == nil {
		b = &tokenBucket{rule: rule, tokens: rule.burst, at: now}
		l.buckets[id] = b
	}
	b.refill(now)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / rule.rate * float64(time.Second))
	return false, wait
}

// refill は、前回からの経過時間ぶん札を戻す（最大 burst まで）。
// 時計が戻ったとき（経過が負）は戻さない。
func (b *tokenBucket) refill(now time.Time) {
	if elapsed := now.Sub(b.at).Seconds(); elapsed > 0 {
		b.tokens = math.Min(b.rule.burst, b.tokens+elapsed*b.rule.rate)
	}
	b.at = now
}

// sweep は満タンに戻った札を捨てる。札の数は「最近使った利用者の数」までしか増えない。
func (l *userRateLimiter) sweep(now time.Time) {
	for id, b := range l.buckets {
		b.refill(now)
		if b.tokens >= b.rule.burst {
			delete(l.buckets, id)
		}
	}
	l.lastSweep = now
}

// rateLimitKey は数える単位。ふつうは利用者。デモアカウントは面接官が共有して使うので、
// 利用者と IP の組で数える（1人が使い切っても、ほかの人のデモが止まらないように）。
func rateLimitKey(r *http.Request, s *Session) string {
	if s.Email == DemoEmail {
		return s.UserID + "@" + ClientIP(r)
	}
	return s.UserID
}

// limitUser は、利用者の札が尽きていたら 429 を送って false を返す。
func (rt *Router) limitUser(w http.ResponseWriter, r *http.Request, s *Session) bool {
	rule := rateLimitRead
	if isWrite(r) {
		rule = rateLimitWrite
	}
	ok, wait := rt.rateLimiter.allow(rule, rateLimitKey(r, s))
	if ok {
		return true
	}
	// 利用者 ID は個人を直接は表さないので、調べられるようにログに残す（メールアドレスは残さない）。
	slog.WarnContext(r.Context(), "request limited: per user", "userId", s.UserID, "kind", rule.name)
	// Retry-After は秒の整数。切り上げて、待った後には必ず1つ戻っているようにする。
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
	WriteErrorBody(w, r, http.StatusTooManyRequests, codeTooManyRequests)
	return false
}

// UseUp は、時計を止めたうえで利用者たちの札（読み取り・書き込みとも）を使い切る。
// main のテストで、登録したどのルートも回数制限を通る（ハンドラまで来ずに 429 になる）かを確かめるためのもの。
func (rt *Router) UseUp(userIDs ...string) {
	frozen := time.Now()
	rt.rateLimiter = newUserRateLimiter(func() time.Time { return frozen })
	for _, id := range userIDs {
		for _, rule := range []rateLimitRule{rateLimitRead, rateLimitWrite} {
			for {
				if ok, _ := rt.rateLimiter.allow(rule, id); !ok {
					break
				}
			}
		}
	}
}
