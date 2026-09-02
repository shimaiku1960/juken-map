"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import { notifyDemoReadOnly } from "@/lib/demo-client";
import type { NotificationPreferenceInput } from "@/lib/validations/notification";

type Props = NotificationPreferenceInput & {
  initialLineConnected: boolean;
  lineOfficialAccountUrl: string;
  lineResult?: string;
  readOnly?: boolean;
};

const lineResultFeedback: Record<string, { variant: "success" | "info" | "error"; message: string }> = {
  connected: { variant: "success", message: "LINEとの連携が完了しました。朝・夜のLINE通知を選べます。" },
  cancelled: { variant: "info", message: "LINE連携をキャンセルしました。連携する場合は、もう一度お試しください。" },
  "friend-required": { variant: "error", message: "通知を届けるには、受験マップ公式アカウントの友だち追加が必要です。" },
  "already-used": { variant: "error", message: "このLINEアカウントは別の受験マップアカウントに連携済みです。" },
  expired: { variant: "error", message: "LINE連携の有効期限が切れました。もう一度お試しください。" },
  invalid: { variant: "error", message: "LINE連携情報を確認できませんでした。もう一度お試しください。" },
  failed: { variant: "error", message: "LINE連携を完了できませんでした。時間をおいてもう一度お試しください。" },
  unavailable: { variant: "error", message: "LINE連携は現在準備中です。設定完了後にもう一度お試しください。" },
};

const slots = [
  { key: "morning" as const, title: "朝の予定", description: "毎朝7時ごろ（日本時間）、今日の予定と学習を始める導線を送ります。" },
  { key: "evening" as const, title: "夜の振り返り", description: "毎晩21時ごろ（日本時間）、学習時間と予定の達成状況を送ります。" },
];

export default function NotificationPreferenceForm(props: Props) {
  const { lineOfficialAccountUrl, readOnly = false } = props;
  const lineFeedback = props.lineResult ? lineResultFeedback[props.lineResult] : undefined;
  const lineConnectionFeedback = props.lineResult === "connected" ? undefined : lineFeedback;
  const initialPreference: NotificationPreferenceInput = {
    emailMorningEnabled: props.emailMorningEnabled,
    emailEveningEnabled: props.emailEveningEnabled,
    lineMorningEnabled: props.lineMorningEnabled,
    lineEveningEnabled: props.lineEveningEnabled,
  };
  const [preference, setPreference] = useState(initialPreference);
  const [savedPreference, setSavedPreference] = useState(initialPreference);
  const [lineConnected, setLineConnected] = useState(props.initialLineConnected);
  const [isSaving, setIsSaving] = useState(false);
  const [isDisconnecting, setIsDisconnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isDirty = JSON.stringify(preference) !== JSON.stringify(savedPreference);

  const setValue = (key: keyof NotificationPreferenceInput, checked: boolean) =>
    setPreference((current) => ({ ...current, [key]: checked }));

  const save = async () => {
    if (readOnly) return notifyDemoReadOnly();
    setIsSaving(true);
    setError(null);
    try {
      const response = await fetch("/api/notification-preferences", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(preference),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || "保存に失敗しました");
      setPreference(result);
      setSavedPreference(result);
      toast.success("通知設定を保存しました");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "保存に失敗しました");
    } finally {
      setIsSaving(false);
    }
  };

  const disconnect = async () => {
    if (readOnly) return notifyDemoReadOnly();
    if (!window.confirm("LINE連携を解除しますか？LINE通知は停止しますが、メール通知は変わりません。")) return;
    setIsDisconnecting(true);
    setError(null);
    try {
      const response = await fetch("/api/line/connection", { method: "DELETE" });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || "LINE連携を解除できませんでした");
      setLineConnected(false);
      setPreference((current) => ({ ...current, lineMorningEnabled: false, lineEveningEnabled: false }));
      setSavedPreference((current) => ({ ...current, lineMorningEnabled: false, lineEveningEnabled: false }));
      toast.success("LINE連携を解除しました");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "LINE連携を解除できませんでした");
    } finally {
      setIsDisconnecting(false);
    }
  };

  const busy = isSaving || isDisconnecting;

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">朝・夜それぞれで、メールとLINEの両方または片方を選べます。いつでも停止できます。</p>
      {error ? <InlineFeedback variant="error">{error}</InlineFeedback> : null}
      <div className="divide-y rounded-lg border">
        {slots.map((slot) => {
          const emailKey: keyof NotificationPreferenceInput = slot.key === "morning" ? "emailMorningEnabled" : "emailEveningEnabled";
          const lineKey: keyof NotificationPreferenceInput = slot.key === "morning" ? "lineMorningEnabled" : "lineEveningEnabled";
          return (
            <fieldset key={slot.key} className="space-y-4 p-4">
              <legend className="font-medium">{slot.title}</legend>
              <p className="text-sm text-muted-foreground">{slot.description}</p>
              <div className="flex flex-col gap-3 sm:flex-row sm:gap-6">
                <div className="flex min-h-11 items-center gap-3">
                  <Checkbox id={`${slot.key}-email`} checked={preference[emailKey]} disabled={readOnly || busy} onCheckedChange={(checked) => setValue(emailKey, checked === true)} />
                  <Label htmlFor={`${slot.key}-email`} className="cursor-pointer">メール</Label>
                </div>
                <div className="flex min-h-11 items-center gap-3">
                  <Checkbox id={`${slot.key}-line`} checked={preference[lineKey]} disabled={!lineConnected || readOnly || busy} onCheckedChange={(checked) => setValue(lineKey, checked === true)} />
                  <Label htmlFor={`${slot.key}-line`} className={lineConnected ? "cursor-pointer" : "text-muted-foreground"}>
                    LINE{lineConnected ? null : <span className="ml-1 text-xs">（連携が必要）</span>}
                  </Label>
                </div>
              </div>
            </fieldset>
          );
        })}
      </div>
      <div id="line-connection" className="scroll-mt-24 rounded-lg border p-4">
        <p className="font-medium">LINE連携</p>
        {lineConnectionFeedback ? <InlineFeedback variant={lineConnectionFeedback.variant} className="mt-3">{lineConnectionFeedback.message}</InlineFeedback> : null}
        {lineConnected ? (
          <div className="mt-3 space-y-3">
            <InlineFeedback variant="success">
              {props.lineResult === "connected"
                ? "LINEとの連携が完了しました。朝・夜のLINE通知を選べます。"
                : "連携済みです。このLINEアカウントで通知を受け取れます。"}
            </InlineFeedback>
            <Button type="button" variant="outline" size="lg" className="h-11" disabled={readOnly || busy} onClick={() => void disconnect()}>
              {isDisconnecting ? "解除中…" : "LINE連携を解除"}
            </Button>
          </div>
        ) : (
          <div className="mt-3 space-y-3">
            <p className="text-sm text-muted-foreground">LINEで本人確認と公式アカウントの友だち追加を行います。トークでメッセージを送る必要はありません。</p>
            <Button asChild size="lg" className="h-11">
              <a href="/api/line/oauth/start">LINEと連携する</a>
            </Button>
            <p className="text-xs text-muted-foreground">
              直接連携できない場合は、<a href={lineOfficialAccountUrl} target="_blank" rel="noreferrer" className="font-medium text-primary hover:underline">公式アカウントのトーク</a>で「連携」と送る方法も利用できます。
            </p>
          </div>
        )}
      </div>
      <Button type="button" size="lg" className="h-11" disabled={!isDirty || busy || readOnly} onClick={() => void save()}>
        {isSaving ? "保存中…" : "通知設定を保存"}
      </Button>
    </div>
  );
}
