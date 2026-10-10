package chaos

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/hostfault"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
)

var testRoutes = []string{"GET /api/dashboard", "GET /api/study-logs", "POST /api/study-logs"}

func tokyo(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2026, month, day, hour, minute, 0, 0, dates.Tokyo)
}

func TestInWindow(t *testing.T) {
	// 2026-10-12 は月曜、10-16 は金曜、10-17 は土曜。
	for _, c := range []struct {
		name string
		at   time.Time
		d    time.Duration
		want bool
	}{
		{"平日の10時ちょうどは起こす", tokyo(10, 12, 10, 0), 30 * time.Minute, true},
		{"平日の10時より前は起こさない", tokyo(10, 12, 9, 59), 10 * time.Minute, false},
		{"22時ちょうどに終わるなら起こす", tokyo(10, 16, 21, 30), 30 * time.Minute, true},
		{"22時を越えて終わるなら起こさない", tokyo(10, 16, 21, 31), 30 * time.Minute, false},
		{"土曜は起こさない", tokyo(10, 17, 12, 0), 10 * time.Minute, false},
		{"UTC で渡しても日本時間で見る", tokyo(10, 12, 12, 0).UTC(), 10 * time.Minute, true},
	} {
		if got := inWindow(c.at, c.d); got != c.want {
			t.Errorf("%s: inWindow(%v, %v) = %v", c.name, c.at, c.d, got)
		}
	}
}

func TestDrawFrequencyAndSpecs(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	const weeks = 200
	start := tokyo(10, 12, 0, 0) // 月曜
	hits := 0
	for at := start; at.Before(start.AddDate(0, 0, 7*weeks)); at = at.Add(drawInterval) {
		spec, ok := Draw(r, at, testRoutes, false)
		if !ok {
			continue
		}
		hits++
		if err := spec.Validate(testRoutes); err != nil {
			t.Fatalf("Draw(%v) = %+v: %v", at, spec, err)
		}
		if !inWindow(at, spec.Duration) {
			t.Fatalf("Draw(%v) は時間帯の外: %+v", at, spec)
		}
		if spec.Kind == fault.KindOutboundTimeout && spec.Route != fault.AllRoutes {
			t.Errorf("外部 API のタイムアウトは全ルートにする: %+v", spec)
		}
	}
	// 週2〜3回。実験中もくじを引き続けたとみなしているので、本番は少しだけ減る。
	if perWeek := float64(hits) / weeks; perWeek < 2 || perWeek > 3 {
		t.Errorf("週あたり %.2f 回、want 2〜3", perWeek)
	}
}

func TestDrawHost(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	start := tokyo(10, 12, 0, 0)
	seen := map[fault.Kind]bool{}
	for at := start; at.Before(start.AddDate(0, 0, 7*200)); at = at.Add(drawInterval) {
		spec, ok := Draw(r, at, testRoutes, true)
		if !ok {
			continue
		}
		seen[spec.Kind] = true
		if !inWindow(at, spec.Duration) {
			t.Fatalf("Draw(%v) は時間帯の外: %+v", at, spec)
		}
		if !hostfault.IsHost(spec.Kind) {
			if err := spec.Validate(testRoutes); err != nil {
				t.Fatalf("Draw(%v) = %+v: %v", at, spec, err)
			}
			continue
		}
		e := fault.Experiment{Kind: spec.Kind, Rate: spec.Rate, DelayMs: spec.DelayMs}
		if !slices.Contains(hostfault.Levels[spec.Kind], hostfault.Level(e)) {
			t.Errorf("強さが候補に無い: %+v", spec)
		}
		if spec.Route != "" || spec.Duration < fault.MinDuration || spec.Duration > 30*time.Minute {
			t.Errorf("ホストの実験の値: %+v", spec)
		}
	}
	for _, k := range append(slices.Clone(fault.Kinds), hostfault.Kinds...) {
		if !seen[k] {
			t.Errorf("%s が一度も選ばれない", k)
		}
	}
}

func TestReportEmail(t *testing.T) {
	start := tokyo(10, 12, 14, 0).UTC()
	stoppedAt := start.Add(7 * time.Minute)
	subject, body := reportEmail(fault.Record{
		Experiment: fault.Experiment{ID: 1, Kind: fault.KindHTTPError, Route: "GET /api/study-logs", Rate: 0.25,
			StatusCode: 503, StartsAt: start, EndsAt: start.Add(20 * time.Minute)},
		StoppedAt: &stoppedAt, StoppedBy: writechaos.StoppedByDeploy,
	})
	if subject != "【受験マップ】障害注入の実験が終わりました" {
		t.Errorf("subject = %q", subject)
	}
	for _, want := range []string{"2026年10月12日 14:00:00", "2026年10月12日 14:07:00（デプロイで止めた）",
		"5xx", "GET /api/study-logs", "25%", "503 を返す"} {
		if !strings.Contains(body, want) {
			t.Errorf("本文に %q が無い: %s", want, body)
		}
	}

	_, body = reportEmail(fault.Record{Experiment: fault.Experiment{Kind: fault.KindLatency, Route: fault.AllRoutes,
		Rate: 1, DelayMs: 3000, StartsAt: start, EndsAt: start.Add(10 * time.Minute)}})
	for _, want := range []string{"2026年10月12日 14:10:00（終わる時刻が来て戻った）", "すべて", "3000ms"} {
		if !strings.Contains(body, want) {
			t.Errorf("本文に %q が無い: %s", want, body)
		}
	}
}

func TestReportEmailHost(t *testing.T) {
	start := tokyo(10, 12, 14, 0).UTC()
	failedAt := start.Add(2 * time.Minute)
	_, body := reportEmail(fault.Record{
		Experiment: fault.Experiment{Kind: hostfault.KindDBDelay, Rate: 1, DelayMs: 300, StartsAt: start,
			EndsAt: start.Add(20 * time.Minute)},
		StoppedAt: &failedAt, StoppedBy: writechaos.StoppedByFailed,
	})
	for _, want := range []string{"RDS までの通信の遅延", "—（ホスト）", "300ms 遅らせる",
		"2026年10月12日 14:02:00（ホストで起こせず、すぐ止めた。API のログを見る）"} {
		if !strings.Contains(body, want) {
			t.Errorf("本文に %q が無い: %s", want, body)
		}
	}
}

func TestStoppedByLabel(t *testing.T) {
	for by, want := range map[string]string{
		writechaos.StoppedByDeploy:          "デプロイ",
		writechaos.StoppedByJob:             "機械の入口（/api/chaos/）",
		writechaos.StoppedByAdmin + "user1": "管理画面",
	} {
		if got := stoppedByLabel(by); got != want {
			t.Errorf("stoppedByLabel(%q) = %q, want %q", by, got, want)
		}
	}
}
