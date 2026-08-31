"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";

export default function LineAccountLinkButton({ linkToken }: { linkToken: string }) {
  const [isLinking, setIsLinking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const link = async () => {
    setIsLinking(true);
    setError(null);
    try {
      const response = await fetch("/api/line/account-link", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ linkToken }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || "連携を開始できませんでした");
      window.location.href = result.redirectUrl;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "連携を開始できませんでした");
      setIsLinking(false);
    }
  };

  return (
    <div className="space-y-4">
      {error ? <InlineFeedback variant="error">{error}</InlineFeedback> : null}
      <Button type="button" size="lg" className="h-11 w-full" disabled={isLinking} onClick={() => void link()}>
        {isLinking ? "LINEへ移動中…" : "このアカウントと連携する"}
      </Button>
    </div>
  );
}
