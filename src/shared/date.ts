// 受験日まであと何日かを返す（今日=0、過去はマイナス）
export function daysUntil(date: Date | string): number {
  const target = new Date(date);
  const today = new Date();
  // 時刻を切り捨てて「日付」単位で差を取る
  target.setHours(0, 0, 0, 0);
  today.setHours(0, 0, 0, 0);
  const diffMs = target.getTime() - today.getTime();
  return Math.round(diffMs / (1000 * 60 * 60 * 24));
}

// Date → ローカル基準の "YYYY-MM-DD"
export function ymdLocal(date: Date | string): string {
  const d = new Date(date);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// 今日の "YYYY-MM-DD"
export function todayYmd(): string {
  return ymdLocal(new Date());
}

// Intl.DateTimeFormat は作るのが重く（1回あたり十数µs）、ダッシュボードは毎リクエスト
// 今日の日付を求めるので、作るのは1回だけにして使い回す（2026-09-25 のプロファイル、JUK-49）。
const tokyoYmdFormat = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Tokyo",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

// 日本の学習日としての今日。サーバーがUTCでもクライアントと同じ日付になる。
export function todayYmdTokyo(): string {
  const parts = tokyoYmdFormat.formatToParts(new Date());
  const values = Object.fromEntries(
    parts
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value])
  );
  return `${values.year}-${values.month}-${values.day}`;
}

// "YYYY-MM-DD" の形。入力チェックが日付の文字列に使う。
export const YMD_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

// "YYYY-MM-DD"（形は YMD_PATTERN で確かめ済み）が暦にある日付か。2026-02-30・2026-13-01 は false。
// Date は使わない（Date.UTC は 0〜99 年を 1900 年代に読み替える）。Go 側（apps/api-go）も同じ算数で判定する。
export function isCalendarYmd(ymd: string): boolean {
  const [y, m, d] = ymd.split("-").map(Number);
  if (m < 1 || m > 12 || d < 1) return false;
  const leap = (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0;
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return d <= days[m - 1];
}

// "YYYY-MM-DD" を n 日ずらした "YYYY-MM-DD"（負の数で過去へ）。
// 月またぎ・うるう年は Date に任せる。時刻を持たないので時間帯の影響を受けない。
export function shiftYmd(ymd: string, days: number): string {
  const [y, m, d] = ymd.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  date.setDate(date.getDate() + days);
  return ymdLocal(date);
}

// 今日から n 日後の "YYYY-MM-DD"
export function ymdAfterDays(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() + n);
  return ymdLocal(d);
}

// 受験日を「2027年2月20日」形式で返す
export function formatExamDate(date: Date | string): string {
  return new Date(date).toLocaleDateString("ja-JP", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}

const tokyoDateFormat = new Intl.DateTimeFormat("ja-JP", { timeZone: "Asia/Tokyo" });

// 記事の公開日などを「2026/8/3」形式で、日本時間の日付として返す。
// 記事の HTML はサーバー（UTC）でも作る（SSR・SSG）ので、実行環境の時間帯に任せると
// 日本時間 0〜9 時の記事はサーバーとブラウザで日付が1日ずれ、ハイドレーションで食い違う。
export function formatDateTokyo(date: Date | string): string {
  return tokyoDateFormat.format(new Date(date));
}
