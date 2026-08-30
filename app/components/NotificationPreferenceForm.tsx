"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { notifyDemoReadOnly } from "@/lib/demo-client";
import type { NotificationPreferenceInput } from "@/lib/validations/notification";

type Props = NotificationPreferenceInput & { readOnly?: boolean };

export default function NotificationPreferenceForm({
  morningEnabled: initialMorningEnabled,
  eveningEnabled: initialEveningEnabled,
  readOnly = false,
}: Props) {
  const [morningEnabled, setMorningEnabled] = useState(initialMorningEnabled);
  const [eveningEnabled, setEveningEnabled] = useState(initialEveningEnabled);
  const [savedPreference, setSavedPreference] = useState({
    morningEnabled: initialMorningEnabled,
    eveningEnabled: initialEveningEnabled,
  });
  const [isSaving, setIsSaving] = useState(false);
  const isDirty =
    morningEnabled !== savedPreference.morningEnabled ||
    eveningEnabled !== savedPreference.eveningEnabled;

  const save = async () => {
    if (readOnly) {
      notifyDemoReadOnly();
      return;
    }

    setIsSaving(true);
    try {
      const response = await fetch("/api/notification-preferences", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ morningEnabled, eveningEnabled }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || "保存に失敗しました");
      setSavedPreference(result);
      toast.success("メール通知の設定を保存しました");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存に失敗しました");
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">
        必要な通知だけを選べます。通知はいつでも停止できます。
      </p>
      <div className="flex items-start gap-3">
        <Checkbox
          id="morning-notification"
          className="mt-1"
          checked={morningEnabled}
          disabled={readOnly || isSaving}
          onCheckedChange={(checked) => setMorningEnabled(checked === true)}
        />
        <Label htmlFor="morning-notification" className="block cursor-pointer">
          <span className="block font-medium">朝の予定メール</span>
          <span className="mt-1 block text-sm font-normal text-muted-foreground">
            毎朝7時ごろ、今日の予定と学習を始める導線を送ります。
          </span>
        </Label>
      </div>
      <div className="flex items-start gap-3">
        <Checkbox
          id="evening-notification"
          className="mt-1"
          checked={eveningEnabled}
          disabled={readOnly || isSaving}
          onCheckedChange={(checked) => setEveningEnabled(checked === true)}
        />
        <Label htmlFor="evening-notification" className="block cursor-pointer">
          <span className="block font-medium">夜の振り返りメール</span>
          <span className="mt-1 block text-sm font-normal text-muted-foreground">
            毎晩21時ごろ、今日の学習時間と予定の達成状況を送ります。
          </span>
        </Label>
      </div>
      <Button
        type="button"
        size="lg"
        className="h-11"
        disabled={!isDirty || isSaving || readOnly}
        onClick={() => void save()}
      >
        {isSaving ? "保存中…" : "通知設定を保存"}
      </Button>
    </div>
  );
}
