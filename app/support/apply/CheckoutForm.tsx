"use client";

import { useState } from "react";
import { LockKeyhole } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";

export default function CheckoutForm({ invitationToken }: { invitationToken: string }) {
  const [termsAccepted, setTermsAccepted] = useState(false);
  const [privacyAccepted, setPrivacyAccepted] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const startCheckout = async () => {
    setLoading(true);
    setError(null);

    try {
      const response = await fetch("/api/support/checkout", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ invitationToken }),
      });
      const data = (await response.json()) as { url?: string; error?: string };
      if (!response.ok || !data.url) {
        throw new Error(data.error || "決済画面を開始できませんでした");
      }
      window.location.assign(data.url);
    } catch (checkoutError) {
      setError(
        checkoutError instanceof Error
          ? checkoutError.message
          : "決済画面を開始できませんでした"
      );
      setLoading(false);
    }
  };

  return (
    <div className="space-y-4">
      <label className="flex min-h-11 items-start gap-3 text-sm leading-6 text-muted-foreground">
        <Checkbox
          checked={termsAccepted}
          onCheckedChange={(checked) => setTermsAccepted(checked === true)}
          aria-label="有料サポート特約と重要条件に同意する"
          className="mt-1"
        />
        <span>有料サポート利用特約と上記の重要条件を確認し、同意します。</span>
      </label>
      <label className="flex min-h-11 items-start gap-3 text-sm leading-6 text-muted-foreground">
        <Checkbox
          checked={privacyAccepted}
          onCheckedChange={(checked) => setPrivacyAccepted(checked === true)}
          aria-label="プライバシーポリシーを確認する"
          className="mt-1"
        />
        <span>プライバシーポリシーを確認しました。</span>
      </label>

      {error ? (
        <InlineFeedback variant="error">
          <p>{error}</p>
        </InlineFeedback>
      ) : null}

      <Button
        type="button"
        size="lg"
        className="h-12 w-full text-base"
        disabled={!termsAccepted || !privacyAccepted || loading}
        onClick={() => void startCheckout()}
      >
        <LockKeyhole aria-hidden="true" />
        {loading ? "決済画面を準備中…" : "支払い方法を登録して7日間無料で始める"}
      </Button>
      <p className="text-center text-sm text-muted-foreground">
        本日は請求されません。7日間の無料体験終了後に月額料金が発生します。
      </p>
    </div>
  );
}
