"use client";

import { useState } from "react";
import { CreditCard } from "lucide-react";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

const statusLabels: Record<string, string> = {
  trialing: "無料体験中",
  active: "契約中",
  past_due: "支払いの確認が必要",
  canceled: "解約済み",
  unpaid: "未払い",
  paused: "一時停止中",
  incomplete: "手続き未完了",
  incomplete_expired: "手続き期限切れ",
};

function formatDate(value: string | null) {
  if (!value) return null;
  return new Intl.DateTimeFormat("ja-JP", {
    year: "numeric",
    month: "long",
    day: "numeric",
  }).format(new Date(value));
}

type Props = {
  status: string;
  trialEndsAt: string | null;
  currentPeriodEndsAt: string | null;
  cancelAtPeriodEnd: boolean;
  cancelAt: string | null;
};

export default function SupportSubscriptionCard({
  status,
  trialEndsAt,
  currentPeriodEndsAt,
  cancelAtPeriodEnd,
  cancelAt,
}: Props) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const trialEnd = formatDate(trialEndsAt);
  const periodEnd = formatDate(currentPeriodEndsAt);
  const scheduledCancellation = formatDate(cancelAt) ??
    (cancelAtPeriodEnd ? periodEnd : null);

  const openPortal = async () => {
    setLoading(true);
    setError(null);

    try {
      const response = await fetch("/api/support/portal", { method: "POST" });
      const data = (await response.json()) as { url?: string; error?: string };
      if (!response.ok || !data.url) {
        throw new Error(data.error || "契約管理画面を開けませんでした");
      }
      window.location.assign(data.url);
    } catch (portalError) {
      setError(
        portalError instanceof Error
          ? portalError.message
          : "契約管理画面を開けませんでした"
      );
      setLoading(false);
    }
  };

  return (
    <Card>
      <CardContent className="flex flex-col gap-5 py-5 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
        <div>
          <p className="font-medium">受験英語LINE質問サポート</p>
          <p className="mt-1 text-sm text-muted-foreground">
            状態：{statusLabels[status] ?? status}
          </p>
          {scheduledCancellation ? (
            <p className="mt-1 text-sm text-muted-foreground">
              {scheduledCancellation}に解約予定です。
            </p>
          ) : status === "trialing" && trialEnd ? (
            <p className="mt-1 text-sm text-muted-foreground">
              無料体験は{trialEnd}までです。
            </p>
          ) : periodEnd ? (
            <p className="mt-1 text-sm text-muted-foreground">
              次回更新日は{periodEnd}です。
            </p>
          ) : null}
        </div>

        <Button
          type="button"
          variant="outline"
          size="lg"
          className="min-h-11 shrink-0"
          disabled={loading}
          onClick={() => void openPortal()}
        >
          <CreditCard aria-hidden="true" />
          {loading ? "準備中…" : "契約・支払いを管理"}
        </Button>

        {error ? (
          <div className="sm:basis-full">
            <InlineFeedback variant="error">{error}</InlineFeedback>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
