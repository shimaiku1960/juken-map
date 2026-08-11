type SupportSubscriptionDateDisplayInput = {
  status: string;
  trialEndsAt: string | null;
  currentPeriodEndsAt: string | null;
  cancelAtPeriodEnd: boolean;
  cancelAt: string | null;
};

function formatDate(value: string | null) {
  if (!value) return null;
  return new Intl.DateTimeFormat("ja-JP", {
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(new Date(value));
}

export function getSupportSubscriptionDateLabel({
  status,
  trialEndsAt,
  currentPeriodEndsAt,
  cancelAtPeriodEnd,
  cancelAt,
}: SupportSubscriptionDateDisplayInput) {
  if (status !== "trialing" && status !== "active") return null;

  const periodEnd = formatDate(currentPeriodEndsAt);
  const scheduledCancellation = formatDate(cancelAt) ??
    (cancelAtPeriodEnd ? periodEnd : null);

  if (scheduledCancellation) {
    return `${scheduledCancellation}に解約予定です。`;
  }

  if (status === "trialing") {
    const trialEnd = formatDate(trialEndsAt);
    return trialEnd ? `無料体験は${trialEnd}までです。` : null;
  }

  return periodEnd ? `次回更新日は${periodEnd}です。` : null;
}

export function getSupportSubscriptionAvailabilityNotice(status: string) {
  if (status !== "canceled") return null;

  return "契約期間が終了したため、LINE質問サポートは現在利用できません。再度利用する場合は、LINEから再申込みをご相談ください。";
}
