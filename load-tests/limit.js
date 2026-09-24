// 限界点を測る。1段階＝ある固定RPSで1分間（constant-arrival-rate）。
// 段階を上げながら繰り返し、最初に基準を割った段階が「限界点」になる。
//
// 1反復＝1リクエストにしてある。こうすると到着率がそのままRPSになり、
// 「何RPSで壊れたか」を言い切れる。複数リクエストをまとめた反復にすると、
// 設定値と実際のRPSがずれて、限界点の数字が説明しにくくなる。
//
// 実行は scripts/run-loadtest-limit.sh から。本番は対象にしない。
import http from "k6/http";
import { fail, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";

const baseUrl = __ENV.BASE_URL;
// ログイン済みのクッキー（db/issue-loadtest-sessions.ts が発行する）。
// HTTPでログインさせない理由はそちらのファイルに書いた。
const cookies = (__ENV.LOAD_TEST_COOKIES ?? "").split(",").filter(Boolean);
const rate = Number(__ENV.RATE ?? 100);
const duration = __ENV.DURATION ?? "60s";

if (!baseUrl || cookies.length === 0) {
  fail("BASE_URL / LOAD_TEST_COOKIES が要ります");
}

// local＝手元で起動したサーバー。aws＝本番の AMI から複製した試験環境（terraform/loadtest）。
// 試験環境は外から届かない 10.50.0.0/16 の中にしか無いので、その IP 宛ての HTTPS だけを通す。
// 本番の URL はどちらの場合も通らない。
const loadtestEnv = __ENV.LOADTEST_ENV ?? "local";
const allowedBaseUrls = new Set([
  "http://localhost:3000",
  "http://127.0.0.1:3000",
  "http://host.docker.internal:3000",
]);
const awsTarget = /^https:\/\/10\.50\.\d{1,3}\.\d{1,3}$/;
const allowed = loadtestEnv === "aws" ? awsTarget.test(baseUrl) : allowedBaseUrls.has(baseUrl);
if (!allowed) {
  fail(`本番や外部は対象にしません: ${baseUrl}（LOADTEST_ENV=${loadtestEnv}）`);
}

// spike＝10人から100人へ一気に増える動き。1人が1秒おきに1画面ぶん叩く（閉じたモデル）。
// それ以外は、固定RPSで限界点を探す（開いたモデル）。
const scenarioMode = __ENV.SCENARIO ?? "level";

// 画面がどのAPIを何回呼ぶかの比率。ダッシュボードを開く動きが中心で、
// 書き込み（記録の追加）は1割。実際の使われ方に寄せないと、
// 一番重い一覧の負荷が薄まって限界点が甘く出る。
// key は k6 のメトリクス名に使う（英数字と _ しか使えない）。name は結果の見出し。
// カレンダーが月送りで取りにいく範囲。画面と同じ範囲で叩かないと応答の大きさが変わる。
function monthBounds() {
  const now = new Date();
  const first = `${now.toISOString().slice(0, 7)}-01`;
  const last = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + 1, 0))
    .toISOString()
    .slice(0, 10);
  return [first, last];
}
const [MONTH_FIRST, MONTH_LAST] = monthBounds();

const MIX = [
  // ダッシュボードを1回開くとこの1本。実績・予定・連続記録日数がまとまって返る。
  // 期間はサーバーが決めるので指定しない。
  { key: "get_dashboard", name: "GET /api/dashboard", weight: 30, kind: "read", path: "/api/dashboard" },
  // カレンダーの月送り。当月はダッシュボードに同梱されるので、これは他の月を見る動き。
  { key: "get_study_plans_month", name: "GET /api/study-plans 月ぶん", weight: 25, kind: "read", path: `/api/study-plans?from=${MONTH_FIRST}&to=${MONTH_LAST}` },
  { key: "get_goals", name: "GET /api/goals", weight: 20, kind: "read", path: "/api/goals" },
  { key: "get_textbooks", name: "GET /api/textbooks", weight: 10, kind: "read", path: "/api/textbooks" },
  { key: "get_universities", name: "GET /api/universities", weight: 5, kind: "read", path: "/api/universities" },
  { key: "post_study_logs", name: "POST /api/study-logs", weight: 10, kind: "write", path: "/api/study-logs" },
];
// 比率は MIX_WEIGHTS で上書きできる（例: MIX_WEIGHTS=0,0,50,50,0,0 で軽いAPIだけ）。
// 何が詰まりの原因かを切り分けるとき、重い一覧を外した比率と比べる。
if (__ENV.MIX_WEIGHTS) {
  const weights = __ENV.MIX_WEIGHTS.split(",").map(Number);
  if (weights.length !== MIX.length) fail(`MIX_WEIGHTS は ${MIX.length} 個です`);
  MIX.forEach((entry, index) => {
    entry.weight = weights[index];
  });
}
const totalWeight = MIX.reduce((sum, entry) => sum + entry.weight, 0);

// 一覧ごとに別々の Trend を作る。タグだけだと要約から読み出しにくく、
// 「どのAPIが先に遅くなったか」を段階ごとに比べられない。
const latency = {};
const bytes = {};
for (const entry of MIX) {
  latency[entry.key] = new Trend(`lat_${entry.key}`, true);
  bytes[entry.key] = new Trend(`bytes_${entry.key}`);
}
const status2xx = new Counter("status_2xx");
const status4xx = new Counter("status_4xx");
const status5xx = new Counter("status_5xx");
const statusFailed = new Counter("status_failed"); // 接続できなかった（status 0）
// 混雑時にサーバーが意図して断った分（apps/api/src/overload.ts）。想定外の 5xx とは分けて数える。
const statusShed = new Counter("status_shed");
// 成功した応答だけの所要時間。断った 503 は一瞬で返るので、全体の p95 に混ぜると
// 「受け付けた分がどれだけ速く返ったか」が実際より良く見える。
const latencyOk = new Trend("lat_ok", true);

const scenarios = {
  level: {
    executor: "constant-arrival-rate",
    rate,
    timeUnit: "1s",
    duration,
    // 応答が遅くなるとVUが足りなくなり、「サーバーではなくk6側が詰まった」数字になる。
    // 余裕を持って確保しておく（足りなければ dropped_iterations に出る）。
    preAllocatedVUs: Math.max(50, rate * 2),
    maxVUs: Math.max(100, rate * 6),
    gracefulStop: "30s",
  },
  spike: {
    executor: "ramping-vus",
    startVUs: 10,
    stages: [
      { duration: "1m", target: 10 },
      { duration: "10s", target: 100 },
      { duration: "3m", target: 100 },
      { duration: "10s", target: 10 },
      { duration: "1m", target: 10 },
    ],
    gracefulRampDown: "30s",
  },
};

const thresholds = {
  // 合格条件（dev-standards の暫定値）。割ったらその段階が限界点。
  "http_req_failed": ["rate<0.01"],
  "status_5xx": ["count==0"],
  "http_req_duration{kind:read}": ["p(95)<1000"],
  "http_req_duration{kind:write}": ["p(95)<1500"],
};
// 取りこぼしは到着率を決める level だけの指標。
if (scenarioMode === "level") thresholds.dropped_iterations = ["count==0"];

export const options = {
  discardResponseBodies: false,
  // 試験環境へは IP で繋ぐので、本番と同じ証明書（juken-map.com）とは名前が合わない。
  // 暗号化とハンドシェイクの重さは本番と変わらない。
  insecureSkipTLSVerify: loadtestEnv === "aws",
  scenarios: { [scenarioMode]: scenarios[scenarioMode] },
  thresholds,
  summaryTrendStats: ["avg", "p(50)", "p(95)", "p(99)", "max"],
};
if (!options.scenarios[scenarioMode]) fail(`SCENARIO は level か spike です: ${scenarioMode}`);

// VUごとに1人の利用者を担当する。実際の利用者と同じく、最初からセッションを持っている。
const myCookie = cookies[(__VU - 1) % cookies.length];

function pickEndpoint() {
  let r = Math.random() * totalWeight;
  for (const entry of MIX) {
    if (r < entry.weight) return entry;
    r -= entry.weight;
  }
  return MIX[0];
}

// 未来日は実績にできないので、UTCの昨日を使う（日本時間でも必ず今日以前になる）。
function yesterdayYmd() {
  return new Date(Date.now() - 86400000).toISOString().slice(0, 10);
}

const SUBJECTS = ["english", "math", "japanese", "science", "social", "other"];

export default function () {
  const entry = pickEndpoint();
  const params = {
    tags: { name: entry.name, kind: entry.kind },
    headers: { Cookie: myCookie },
  };

  let response;
  if (entry.kind === "write") {
    response = http.post(
      `${baseUrl}${entry.path}`,
      JSON.stringify({
        date: yesterdayYmd(),
        minutes: 20 + Math.floor(Math.random() * 100),
        subject: SUBJECTS[Math.floor(Math.random() * SUBJECTS.length)],
      }),
      {
        ...params,
        headers: { ...params.headers, "Content-Type": "application/json" },
      }
    );
  } else {
    response = http.get(`${baseUrl}${entry.path}`, params);
  }

  latency[entry.key].add(response.timings.duration);
  bytes[entry.key].add(response.body ? response.body.length : 0);

  if (response.status === 0) statusFailed.add(1);
  else if (response.status === 503 && response.json("code") === "OVERLOADED") statusShed.add(1);
  else if (response.status >= 500) status5xx.add(1);
  else if (response.status >= 400) status4xx.add(1);
  else {
    status2xx.add(1);
    latencyOk.add(response.timings.duration);
  }

  // spike は人数で負荷を決めるので、1人が息をつく間を入れる（無いと人数ではなく最大速度の試験になる）。
  if (scenarioMode === "spike") sleep(1);
}

// 段階ごとの数字を1行のJSONで出す。段階を跨いで比べたいのはここに入れた項目だけで、
// k6 の既定の要約は段階間の比較には情報が多すぎる。
export function handleSummary(data) {
  const trend = (name) => {
    const metric = data.metrics[name];
    if (!metric) return null;
    const round = (value) => (value == null ? null : Math.round(value * 10) / 10);
    return {
      count: metric.values.count,
      p50: round(metric.values["p(50)"]),
      p95: round(metric.values["p(95)"]),
      p99: round(metric.values["p(99)"]),
      max: round(metric.values.max),
      avg: round(metric.values.avg),
    };
  };
  const count = (name) => data.metrics[name]?.values?.count ?? 0;

  const endpoints = {};
  for (const entry of MIX) {
    endpoints[entry.name] = {
      latency_ms: trend(`lat_${entry.key}`),
      body_bytes: trend(`bytes_${entry.key}`),
    };
  }

  const summary = {
    scenario: scenarioMode,
    rate_target: scenarioMode === "level" ? rate : null,
    duration,
    requests: count("http_reqs"),
    rps_actual: Math.round((data.metrics.http_reqs?.values?.rate ?? 0) * 10) / 10,
    failed_rate: data.metrics.http_req_failed?.values?.rate ?? 0,
    dropped_iterations: count("dropped_iterations"),
    vus_max: data.metrics.vus_max?.values?.max ?? 0,
    status: {
      "2xx": count("status_2xx"),
      "4xx": count("status_4xx"),
      "5xx": count("status_5xx"),
      failed: count("status_failed"),
      shed: count("status_shed"),
    },
    ok_rps: Math.round((count("status_2xx") / (data.state.testRunDurationMs / 1000)) * 10) / 10,
    overall_ms: trend("http_req_duration"),
    ok_ms: trend("lat_ok"),
    endpoints,
    thresholds_failed: Object.entries(data.metrics)
      .filter(([, metric]) =>
        Object.values(metric.thresholds ?? {}).some((threshold) => threshold.ok === false)
      )
      .map(([name]) => name),
  };

  return { stdout: `\nLIMIT_RESULT ${JSON.stringify(summary)}\n` };
}
