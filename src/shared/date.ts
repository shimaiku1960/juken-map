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

// 日本の学習日としての今日。サーバーがUTCでもクライアントと同じ日付になる。
export function todayYmdTokyo(): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Tokyo",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(new Date());
  const values = Object.fromEntries(
    parts
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value])
  );
  return `${values.year}-${values.month}-${values.day}`;
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
