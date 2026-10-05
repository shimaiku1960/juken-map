package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus に読ませる数字。名前・ラベル・区切り（buckets）は Node（observability/metrics.ts）と
// 同じにして、Grafana の同じパネルで Node と Go を並べて読めるようにする。
// どちらのサーバーかは、読みに来る側（Alloy）が付けるラベルで分ける。
type metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	// emailSends はアプリが送ろうとした認証のメールの数（06 H1。auth_email.go）。送信量の急増と、上限で止めたことを
	// アラートで知らせる（terraform/grafana/alerting.tf）。Node から移したので名前とラベルは同じ。
	emailSends *prometheus.CounterVec
	// resendQuotaUsed は Resend の送信枠のうち使った数（06 E1）。送らないあいだは古い値が残るので、
	// いつ読んだか（resendQuotaObservedAt）も出し、アラートは新しい値だけを見る。
	resendQuotaUsed       *prometheus.GaugeVec
	resendQuotaObservedAt *prometheus.GaugeVec
}

func newMetrics() *metrics {
	// 既定の置き場（prometheus.DefaultRegisterer）はプロセス全体で1つなので、テストで
	// 何度も作ると「もう登録済み」で失敗する。自分の置き場を持つ。
	registry := prometheus.NewRegistry()
	// CPU（process_cpu_seconds_total）やメモリ（process_resident_memory_bytes）は Node と同じ名前。
	// go_ で始まるものは Go のランタイム（goroutine の数、GC など）。
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())

	labels := []string{"method", "route", "status_code"}
	m := &metrics{
		registry: registry,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "返したリクエストの数",
		}, labels),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "リクエストを受けてから返し終えるまでの時間",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, labels),
	}
	m.emailSends = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "email_sends_total",
		Help: "アプリが送ろうとしたメールの数（result：sent・blocked＝上限で止めた・failed）",
	}, []string{"kind", "result"})
	m.resendQuotaUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "resend_quota_used",
		Help: "Resend の送信枠のうち使った数（最後に送ったときの応答ヘッダー）",
	}, []string{"period"})
	m.resendQuotaObservedAt = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "resend_quota_observed_timestamp_seconds",
		Help: "resend_quota_used を最後に読んだ時刻（UNIX 秒）",
	}, []string{"period"})
	// 全部の組み合わせを 0 で作っておく。値の無い系列にいきなり 1 が現れると、Prometheus の increase() は
	// 1点目を比べる相手が無くて数えられず、最初の「上限で止めた」を見落とす。
	for _, kind := range emailKinds {
		for _, result := range []string{"sent", "blocked", "failed"} {
			m.emailSends.WithLabelValues(string(kind), result)
		}
	}
	registry.MustRegister(m.requests, m.duration, m.emailSends, m.resendQuotaUsed, m.resendQuotaObservedAt)
	return m
}

// observe は1件ぶんを数える。route は実際の URL ではなくルートの型（/api/study-logs/:id）を渡す。
// ID ごとに別の時系列になると、Prometheus が重くなるため。
func (m *metrics) observe(method, route string, status int, elapsed time.Duration) {
	code := strconv.Itoa(status)
	m.requests.WithLabelValues(method, route, code).Inc()
	m.duration.WithLabelValues(method, route, code).Observe(elapsed.Seconds())
}

// handler は /metrics の応答。アプリとは別のポート（METRICS_PORT）で出す。
// 同じポートに置くと、nginx 越しに誰でも内部の数字を読めてしまう（Node と同じ理由）。
func (m *metrics) handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
