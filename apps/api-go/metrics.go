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
	registry.MustRegister(m.requests, m.duration)
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
