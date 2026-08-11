"use client";

import { useState } from "react";
import { CheckCircle2, Copy, ExternalLink, MessageCircle } from "lucide-react";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import { Button, buttonVariants } from "@/components/ui/button";
import { SUPPORT_LINE_ADD_URL, SUPPORT_LINE_CHAT_URL } from "@/lib/support";
import { cn } from "@/lib/utils";

type Props = {
  linked: boolean;
  showDivider?: boolean;
};

export default function SupportLineOnboarding({ linked, showDivider = true }: Props) {
  const [loading, setLoading] = useState(false);
  const [code, setCode] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  if (linked) {
    return (
      <div className={cn("space-y-3", showDivider && "border-t pt-5")}>
        <div>
          <div className="flex items-center gap-2 text-sm font-medium text-primary">
            <CheckCircle2 aria-hidden="true" className="size-4" />
            LINE連携済み
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            スマホではトークが開きます。PCでは表示されたQRコードをスマホで読み取ってください。
          </p>
        </div>
        <a
          href={SUPPORT_LINE_ADD_URL}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(event) => {
            if (/Android|iPhone|iPad|iPod/i.test(navigator.userAgent)) {
              event.currentTarget.href = SUPPORT_LINE_CHAT_URL;
            }
          }}
          className={cn(
            buttonVariants({ size: "lg" }),
            "min-h-11 w-full sm:w-auto"
          )}
        >
          <MessageCircle aria-hidden="true" />
          LINEを開いて質問する
          <ExternalLink aria-hidden="true" />
        </a>
      </div>
    );
  }

  const issueCode = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch("/api/support/line-code", { method: "POST" });
      const data = (await response.json()) as {
        code?: string;
        expiresAt?: string;
        error?: string;
      };
      if (!response.ok || !data.code || !data.expiresAt) {
        throw new Error(data.error || "照合コードを発行できませんでした");
      }
      setCode(data.code);
      setExpiresAt(data.expiresAt);
    } catch (codeError) {
      setError(
        codeError instanceof Error
          ? codeError.message
          : "照合コードを発行できませんでした"
      );
    } finally {
      setLoading(false);
    }
  };

  const copyCode = async () => {
    if (code) await navigator.clipboard.writeText(code);
  };

  return (
    <div className={cn("space-y-3", showDivider && "border-t pt-5")}>
      <div>
        <p className="font-medium">LINEで質問を始める</p>
        <p className="mt-1 text-sm text-muted-foreground">
          公式アカウントを友だち追加し、発行した照合コードをトークで送ってください。
        </p>
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <a
          href={SUPPORT_LINE_ADD_URL}
          target="_blank"
          rel="noopener noreferrer"
          className={cn(buttonVariants({ size: "lg" }), "min-h-11")}
        >
          <MessageCircle aria-hidden="true" />
          LINEを友だち追加
          <ExternalLink aria-hidden="true" />
        </a>
        <Button
          type="button"
          variant="outline"
          size="lg"
          className="min-h-11"
          disabled={loading}
          onClick={() => void issueCode()}
        >
          {loading ? "発行中…" : code ? "コードを再発行" : "照合コードを発行"}
        </Button>
      </div>
      {code ? (
        <div className="rounded-lg border bg-muted/40 p-4">
          <p className="text-sm text-muted-foreground">LINEで送る照合コード</p>
          <div className="mt-2 flex items-center gap-3">
            <code className="text-xl font-bold tracking-widest">{code}</code>
            <Button type="button" variant="ghost" size="icon-lg" onClick={() => void copyCode()}>
              <Copy aria-hidden="true" />
              <span className="sr-only">照合コードをコピー</span>
            </Button>
          </div>
          {expiresAt ? (
            <p className="mt-2 text-xs text-muted-foreground">
              有効期限：{new Date(expiresAt).toLocaleTimeString("ja-JP", { hour: "2-digit", minute: "2-digit" })}
            </p>
          ) : null}
        </div>
      ) : null}
      {error ? <InlineFeedback variant="error">{error}</InlineFeedback> : null}
    </div>
  );
}
