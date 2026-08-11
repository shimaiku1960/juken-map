"use client";

import { useEffect, useState } from "react";
import { CreditCard } from "lucide-react";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import SupportLineOnboarding from "@/app/components/SupportLineOnboarding";

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

function statusBadgeVariant(status: string) {
  if (status === "trialing" || status === "active") return "success" as const;
  if (status === "past_due" || status === "unpaid") return "warning" as const;
  return "secondary" as const;
}

type Props = {
  status: string;
  trialEndsAt: string | null;
  currentPeriodEndsAt: string | null;
  cancelAtPeriodEnd: boolean;
  cancelAt: string | null;
  lineAccessEnabled: boolean;
  lineLinked: boolean;
};

export default function SupportSubscriptionCard({
  status,
  trialEndsAt,
  currentPeriodEndsAt,
  cancelAtPeriodEnd,
  cancelAt,
  lineAccessEnabled,
  lineLinked,
}: Props) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const trialEnd = formatDate(trialEndsAt);
  const periodEnd = formatDate(currentPeriodEndsAt);
  const scheduledCancellation = formatDate(cancelAt) ??
    (cancelAtPeriodEnd ? periodEnd : null);

  useEffect(() => {
    const resetLoading = () => setLoading(false);
    window.addEventListener("pageshow", resetLoading);
    return () => window.removeEventListener("pageshow", resetLoading);
  }, []);

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
      <CardContent className="space-y-5 py-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <p className="font-medium">受験英語LINE質問サポート</p>
              <Badge variant={statusBadgeVariant(status)}>
                {statusLabels[status] ?? status}
              </Badge>
            </div>
            {scheduledCancellation ? (
              <p className="mt-2 text-sm text-muted-foreground">
                {scheduledCancellation}に解約予定です。
              </p>
            ) : status === "trialing" && trialEnd ? (
              <p className="mt-2 text-sm text-muted-foreground">
                無料体験は{trialEnd}までです。
              </p>
            ) : periodEnd ? (
              <p className="mt-2 text-sm text-muted-foreground">
                次回更新日は{periodEnd}です。
              </p>
            ) : null}
          </div>

          <Button
            type="button"
            variant="outline"
            className="min-h-11 w-full shrink-0 sm:w-auto"
            disabled={loading}
            onClick={() => void openPortal()}
          >
            <CreditCard aria-hidden="true" />
            {loading ? "契約管理画面を開いています…" : "契約・支払いを管理"}
          </Button>
        </div>

        {error ? (
          <div>
            <InlineFeedback variant="error">{error}</InlineFeedback>
          </div>
        ) : null}

        {lineAccessEnabled ? <SupportLineOnboarding linked={lineLinked} /> : null}
      </CardContent>
    </Card>
  );
}
