import { SITE_URL } from "@/lib/site";

export type NotificationSlot = "morning" | "evening";

type StudyPlanSummary = {
  done: boolean;
  content: string | null;
  subject: string | null;
  textbook: { name: string } | null;
};

type StudyLogSummary = { minutes: number };

export function tokyoDateRange(now = new Date()) {
  const date = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Tokyo",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
  const start = new Date(`${date}T00:00:00+09:00`);
  const end = new Date(start.getTime() + 24 * 60 * 60 * 1000);
  return { date, start, end };
}

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function planLabel(plan: StudyPlanSummary) {
  return plan.textbook?.name ?? plan.content ?? plan.subject ?? "学習予定";
}

export function buildDailyNotification(args: {
  slot: NotificationSlot;
  nickname: string;
  plans: StudyPlanSummary[];
  logs: StudyLogSummary[];
}) {
  const { slot, nickname, plans, logs } = args;
  const safeName = escapeHtml(nickname);

  if (slot === "morning") {
    const planText =
      plans.length === 0
        ? "今日はまだ予定がありません。まず1つだけ決めてみましょう。"
        : `今日の予定は${plans.length}件です。`;
    const planList = plans.length
      ? `<ul>${plans
          .slice(0, 5)
          .map((plan) => `<li>${escapeHtml(planLabel(plan))}</li>`)
          .join("")}</ul>`
      : "";

    return {
      subject: "【受験マップ】今日の学習予定",
      html: `<p>${safeName}さん、おはようございます。</p><p>${planText}</p>${planList}<p><a href="${SITE_URL}/dashboard">今日の学習を始める</a></p>`,
    };
  }

  const totalMinutes = logs.reduce((sum, log) => sum + log.minutes, 0);
  const completedPlans = plans.filter((plan) => plan.done).length;
  const achievement =
    plans.length > 0
      ? `予定${plans.length}件中${completedPlans}件を完了しました。`
      : "今日は予定を入れない一日でした。";
  const effort =
    totalMinutes > 0
      ? `今日は${totalMinutes}分の学習を記録しました。`
      : "今日はまだ学習記録がありません。短い時間でも、記録から再開できます。";

  return {
    subject: "【受験マップ】今日の学習振り返り",
    html: `<p>${safeName}さん、今日もおつかれさまでした。</p><p>${effort}</p><p>${achievement}</p><p><a href="${SITE_URL}/dashboard">今日を振り返る</a></p>`,
  };
}
