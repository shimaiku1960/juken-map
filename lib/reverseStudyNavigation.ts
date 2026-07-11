import { ymdLocal } from "@/lib/date";

export type NavigationLog = {
  textbookId: number | null;
  date: Date | string;
  rangeEnd: number | null;
  rangeUnit: string | null;
};

export type StudyNavigation = {
  currentAmount: number;
  remainingAmount: number;
  remainingDays: number;
  todayStart: number;
  todayEnd: number;
  dailyAmount: number;
  status: "active" | "completed" | "expired";
};

function calendarDayDiff(from: string, to: string): number {
  const [fromYear, fromMonth, fromDay] = from.split("-").map(Number);
  const [toYear, toMonth, toDay] = to.split("-").map(Number);
  const fromUtc = Date.UTC(fromYear, fromMonth - 1, fromDay);
  const toUtc = Date.UTC(toYear, toMonth - 1, toDay);
  return Math.floor((toUtc - fromUtc) / 86_400_000);
}

export function computeCurrentAmount(
  textbookId: number,
  rangeUnit: string,
  logs: NavigationLog[],
  throughDate: string
): number {
  return logs.reduce((max, log) => {
    if (
      log.textbookId !== textbookId ||
      log.rangeUnit !== rangeUnit ||
      log.rangeEnd == null ||
      ymdLocal(log.date) > throughDate
    ) {
      return max;
    }
    return Math.max(max, log.rangeEnd);
  }, 0);
}

export function computeStudyNavigation({
  totalAmount,
  currentAmount,
  examDate,
  today,
}: {
  totalAmount: number;
  currentAmount: number;
  examDate: Date | string;
  today: string;
}): StudyNavigation {
  const safeCurrent = Math.min(Math.max(currentAmount, 0), totalAmount);
  const remainingAmount = Math.max(totalAmount - safeCurrent, 0);
  const remainingDays = calendarDayDiff(today, ymdLocal(examDate));

  if (remainingAmount === 0) {
    return {
      currentAmount: safeCurrent,
      remainingAmount,
      remainingDays: Math.max(remainingDays, 0),
      todayStart: totalAmount,
      todayEnd: totalAmount,
      dailyAmount: 0,
      status: "completed",
    };
  }

  if (remainingDays <= 0) {
    return {
      currentAmount: safeCurrent,
      remainingAmount,
      remainingDays: 0,
      todayStart: safeCurrent + 1,
      todayEnd: safeCurrent,
      dailyAmount: 0,
      status: "expired",
    };
  }

  const dailyAmount = Math.ceil(remainingAmount / remainingDays);
  return {
    currentAmount: safeCurrent,
    remainingAmount,
    remainingDays,
    todayStart: safeCurrent + 1,
    todayEnd: Math.min(safeCurrent + dailyAmount, totalAmount),
    dailyAmount,
    status: "active",
  };
}
