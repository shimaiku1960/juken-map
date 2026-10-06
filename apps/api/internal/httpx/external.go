package httpx

import "time"

// ExternalTimeout は外部サービス（Resend・LINE・microCMS・OAuth）1回の呼び出しの上限
// （Node の EXTERNAL_TIMEOUT_MS と同じ）。
const ExternalTimeout = 5 * time.Second
