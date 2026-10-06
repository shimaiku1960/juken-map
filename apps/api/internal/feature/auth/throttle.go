package auth

import (
	"context"
	"database/sql"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/authguard"
)

// 回数制限（認証基準 10 の H1、06 の B4）。入口ごとの決まり（authguard.SignInAccount など）と、数え方
// （試行を先に1つ足してから判定する）は持ち主の internal/write/authguard にある（JUK-154）。

// mfaChallengeMaxAttempts は、2段階認証の途中の状態1つで試せるコードの数。
const mfaChallengeMaxAttempts = 5

type throttle struct {
	db  *sql.DB
	now func() time.Time
}

// hit は試行を1つ数え、上限の内なら allowed=true を返す。止めるときは何秒後に窓が終わるかも返す。
func (t *throttle) hit(ctx context.Context, rule authguard.Rule, subject string) (allowed bool, retryAfter time.Duration, err error) {
	return authguard.Hit(ctx, t.db, rule, subject, t.now())
}

// clear は成功したときに、その単位の数を消す。
func (t *throttle) clear(ctx context.Context, rule authguard.Rule, subject string) error {
	return authguard.Clear(ctx, t.db, rule, subject)
}
