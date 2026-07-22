"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { Pause, Play, Square } from "lucide-react";
import { notifyDemoReadOnly } from "@/lib/demo-client";
import { toast } from "sonner";
import QuickManualStudyLogDialog from "@/app/components/QuickManualStudyLogDialog";
import {
  NumberStepper,
  RangeUnitSelect,
  SubjectSelect,
  TextbookSelect,
} from "@/app/components/StudyFields";
import { studyLogsKey } from "@/app/hooks/useStudyLogs";
import {
  studyPlansKey,
  useStudyPlans,
  type StudyPlan,
} from "@/app/hooks/useStudyPlans";
import { useTextbooks } from "@/app/hooks/useTextbooks";
import { todayYmd } from "@/lib/date";
import { studyPlanLabel } from "@/lib/studyPlan";
import {
  type ActiveStudySession,
  elapsedStudyMs,
  formatStudyElapsed,
  parseStoredStudySession,
  pauseStudySession,
  recordedMinutes,
  resumeStudySession,
  reviewStudySession,
  startStudySession,
  studySessionStorageKey,
} from "@/lib/studySession";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const responseError = async (response: Response) => {
  const body = await response.json().catch(() => null);
  if (typeof body?.error === "string") return body.error;
  if (Array.isArray(body?.error) && typeof body.error[0]?.message === "string") {
    return body.error[0].message;
  }
  return "実績を保存できませんでした";
};

export default function StudySessionManager({
  initialPlans,
  userId,
  readOnly = false,
  variant = "compact",
}: {
  initialPlans: StudyPlan[];
  userId: string;
  readOnly?: boolean;
  /** hero: ログイン直後の集中スタート画面向けに、ボタンを中央・大きく表示する */
  variant?: "compact" | "hero";
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { data: textbooks = [] } = useTextbooks();
  const { data: studyPlans = [] } = useStudyPlans(initialPlans);
  const plans = useMemo(
    () =>
      studyPlans
        .filter((plan) => plan.date.slice(0, 10) === todayYmd())
        .map((plan) => ({
          id: plan.id,
          content: studyPlanLabel(plan),
          done: plan.done,
          subject: plan.subject,
          textbookId: plan.textbookId,
          textbookName: plan.textbook?.name ?? null,
          rangeStart: plan.rangeStart,
          rangeEnd: plan.rangeEnd,
          rangeUnit: plan.rangeUnit,
          recordedMinutes: plan.studyLogId == null ? null : 0,
        })),
    [studyPlans]
  );
  const storageKey = studySessionStorageKey(userId);
  const selectablePlans = useMemo(
    () => plans.filter((plan) => plan.recordedMinutes == null),
    [plans]
  );

  const [hydrated, setHydrated] = useState(false);
  const [session, setSession] = useState<ActiveStudySession | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [pickerOpen, setPickerOpen] = useState(false);
  const [manualLogOpen, setManualLogOpen] = useState(false);
  const [selectedTarget, setSelectedTarget] = useState("manual");
  const [manualLabel, setManualLabel] = useState("");
  const [manualSubject, setManualSubject] = useState<string | null>(null);
  const [manualTextbookId, setManualTextbookId] = useState<number | null>(null);
  const [minutes, setMinutes] = useState(1);
  const [rangeStart, setRangeStart] = useState<number | null>(null);
  const [rangeEnd, setRangeEnd] = useState<number | null>(null);
  const [rangeUnit, setRangeUnit] = useState<string | null>(null);
  const [memo, setMemo] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [confirmDiscard, setConfirmDiscard] = useState(false);

  useEffect(() => {
    const restoreTimer = window.setTimeout(() => {
      const storedValue = window.localStorage.getItem(storageKey);
      const restored = parseStoredStudySession(storedValue);
      if (!restored && storedValue) {
        window.localStorage.removeItem(storageKey);
        toast.error("以前のタイマー情報を復元できなかったため、リセットしました");
      }
      setSession(restored);
      if (restored?.status === "reviewing") {
        setMinutes(recordedMinutes(restored));
        setRangeStart(restored.rangeStart);
        setRangeEnd(restored.rangeEnd);
        setRangeUnit(restored.rangeUnit);
      }
      setHydrated(true);
    }, 0);

    const sync = (event: StorageEvent) => {
      if (event.key === storageKey) {
        const synced = parseStoredStudySession(event.newValue);
        setSession(synced);
        if (synced?.status === "reviewing") {
          setMinutes(recordedMinutes(synced));
          setRangeStart(synced.rangeStart);
          setRangeEnd(synced.rangeEnd);
          setRangeUnit(synced.rangeUnit);
        }
      }
    };
    window.addEventListener("storage", sync);
    return () => {
      window.clearTimeout(restoreTimer);
      window.removeEventListener("storage", sync);
    };
  }, [storageKey]);

  useEffect(() => {
    if (!hydrated) return;
    if (session) {
      window.localStorage.setItem(storageKey, JSON.stringify(session));
    } else {
      window.localStorage.removeItem(storageKey);
    }
  }, [hydrated, session, storageKey]);

  useEffect(() => {
    if (session?.status !== "running") return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [session?.status]);

  const openPicker = () => {
    setSelectedTarget(
      selectablePlans[0] ? `plan:${selectablePlans[0].id}` : "manual"
    );
    setManualLabel("");
    setManualSubject(null);
    setManualTextbookId(null);
    setPickerOpen(true);
  };

  const beginSession = () => {
    const timestamp = Date.now();
    if (selectedTarget.startsWith("plan:")) {
      const planId = Number(selectedTarget.slice("plan:".length));
      const plan = selectablePlans.find((candidate) => candidate.id === planId);
      if (!plan) return;
      setSession(
        startStudySession(
          {
            planId: plan.id,
            label: plan.content,
            subject: plan.subject,
            textbookId: plan.textbookId,
            rangeStart: plan.rangeStart,
            rangeEnd: plan.rangeEnd,
            rangeUnit: plan.rangeUnit,
          },
          timestamp
        )
      );
    } else {
      const textbook = textbooks.find((item) => item.id === manualTextbookId);
      // 自由入力した「内容」を最優先。なければ参考書名 → その他の学習。
      const trimmedLabel = manualLabel.trim();
      const label = trimmedLabel || textbook?.name || "その他の学習";
      setSession(
        startStudySession(
          {
            planId: null,
            label,
            subject: manualSubject,
            textbookId: manualTextbookId,
            rangeStart: null,
            rangeEnd: null,
            rangeUnit: textbook?.rangeUnit ?? null,
          },
          timestamp
        )
      );
    }
    setNow(timestamp);
    setPickerOpen(false);
    toast.success("学習時間の計測を開始しました");
  };

  const moveToReview = () => {
    if (!session) return;
    const reviewed = reviewStudySession(session);
    setSession(reviewed);
    setMinutes(recordedMinutes(reviewed));
    setRangeStart(reviewed.rangeStart);
    setRangeEnd(reviewed.rangeEnd);
    setRangeUnit(reviewed.rangeUnit);
    // 「その他の学習」で自由入力した内容（＝ラベル）をメモの初期値に引き継ぎ、
    // 記録に「何をやったか」を残せるようにする。予定・参考書由来のラベルは対象外。
    const freeTextLabel =
      reviewed.planId == null &&
      reviewed.textbookId == null &&
      reviewed.label !== "その他の学習"
        ? reviewed.label
        : "";
    setMemo(freeTextLabel);
    setSaveError(null);
    setConfirmDiscard(false);
  };

  const save = async (asManual = false) => {
    if (!session) return;
    setSaving(true);
    setSaveError(null);
    const planId = asManual ? null : session.planId;
    try {
      const response = await fetch(
        planId == null
          ? "/api/study-logs"
          : `/api/study-plans/${planId}/complete`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(
            planId == null
              ? {
                  date: todayYmd(),
                  minutes,
                  subject: session.subject,
                  textbookId: session.textbookId,
                  rangeStart,
                  rangeEnd,
                  rangeUnit,
                  memo,
                }
              : { minutes, rangeStart, rangeEnd, rangeUnit, memo }
          ),
        }
      );
      if (!response.ok) {
        setSaveError(await responseError(response));
        return;
      }

      clearSession();
      setConfirmDiscard(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: studyLogsKey }),
        queryClient.invalidateQueries({ queryKey: studyPlansKey }),
      ]);
      router.refresh();
      toast.success(
        planId == null
          ? "学習実績を保存しました"
          : "実績を保存し、予定を完了しました"
      );
    } catch {
      setSaveError("通信に失敗しました。接続を確認して、もう一度お試しください");
    } finally {
      setSaving(false);
    }
  };

  const elapsed = session ? elapsedStudyMs(session, now) : 0;

  const clearSession = () => {
    window.localStorage.removeItem(storageKey);
    setSession(null);
  };

  const isHero = variant === "hero";

  return (
    <div
      className={
        isHero
          ? "flex w-full flex-col items-center gap-3"
          : "flex w-full flex-col items-end gap-2"
      }
    >
      {!hydrated ? (
        <Button
          type="button"
          className={
            isHero ? "h-14 px-8 text-lg" : "h-11 w-full sm:w-auto"
          }
          disabled
        >
          タイマーを確認中…
        </Button>
      ) : session ? null : (
        <Button
          type="button"
          className={
            isHero ? "h-14 px-8 text-lg" : "h-11 w-full sm:w-auto"
          }
          title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
          onClick={readOnly ? notifyDemoReadOnly : openPicker}
        >
          <Play aria-hidden="true" />
          学習を始める
        </Button>
      )}

      {readOnly && !session && (
        <p
          className={
            isHero
              ? "text-xs text-muted-foreground"
              : "text-xs text-muted-foreground sm:text-right"
          }
        >
          デモアカウントでは計測できません
        </p>
      )}

      {session && session.status !== "reviewing" && (
        <div className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-8 bg-background px-6 text-center">
          <div className="flex flex-col items-center gap-3">
            <p className="text-sm font-medium text-blue-600">
              {session.status === "running" ? "● 計測中" : "一時停止中"}
            </p>
            <p className="max-w-md truncate text-lg font-medium">
              {session.label}
            </p>
            <p className="font-mono text-6xl font-bold tabular-nums sm:text-7xl">
              {formatStudyElapsed(elapsed)}
            </p>
          </div>
          <div className="flex w-full max-w-xs flex-col gap-3 sm:max-w-none sm:flex-row sm:justify-center">
            {session.status === "running" ? (
              <Button
                type="button"
                variant="outline"
                className="h-12 sm:w-40"
                onClick={() => setSession(pauseStudySession(session))}
              >
                <Pause aria-hidden="true" />
                一時停止
              </Button>
            ) : (
              <Button
                type="button"
                className="h-12 sm:w-40"
                onClick={() => setSession(resumeStudySession(session))}
              >
                <Play aria-hidden="true" />
                再開
              </Button>
            )}
            <Button
              type="button"
              variant="destructive"
              className="h-12 sm:w-40"
              onClick={moveToReview}
            >
              <Square aria-hidden="true" />
              学習を終了
            </Button>
          </div>
        </div>
      )}

      <Dialog open={pickerOpen} onOpenChange={setPickerOpen}>
        <DialogContent className="top-auto bottom-0 max-h-[90dvh] translate-y-0 overflow-y-auto rounded-b-none sm:top-1/2 sm:bottom-auto sm:max-w-lg sm:-translate-y-1/2 sm:rounded-xl">
          <DialogHeader>
            <DialogTitle>何を勉強しますか？</DialogTitle>
            <DialogDescription>
              今日の予定またはその他の学習を選んで、時間計測を始めます。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            {selectablePlans.length > 0 && (
              <fieldset className="space-y-2">
                <legend className="text-sm font-medium">今日の予定</legend>
                {selectablePlans.map((plan) => (
                  <label
                    key={plan.id}
                    className="flex cursor-pointer gap-3 rounded-lg border p-3 has-[:checked]:border-blue-500 has-[:checked]:bg-blue-50"
                  >
                    <input
                      type="radio"
                      name="study-target"
                      value={`plan:${plan.id}`}
                      checked={selectedTarget === `plan:${plan.id}`}
                      onChange={(event) => setSelectedTarget(event.target.value)}
                    />
                    <span className="min-w-0 font-medium">{plan.content}</span>
                  </label>
                ))}
              </fieldset>
            )}

            <label className="flex cursor-pointer gap-3 rounded-lg border p-3 has-[:checked]:border-blue-500 has-[:checked]:bg-blue-50">
              <input
                type="radio"
                name="study-target"
                value="manual"
                checked={selectedTarget === "manual"}
                onChange={(event) => setSelectedTarget(event.target.value)}
              />
              <span className="font-medium">その他の学習</span>
            </label>

            {selectedTarget === "manual" && (
              <div className="grid gap-3 rounded-lg bg-muted p-3 sm:grid-cols-2">
                <div className="space-y-1 sm:col-span-2">
                  <Label htmlFor="session-activity">内容（任意）</Label>
                  <Input
                    id="session-activity"
                    className="h-11"
                    placeholder="例：英作文の添削、過去問演習"
                    maxLength={100}
                    value={manualLabel}
                    onChange={(event) => setManualLabel(event.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    科目や参考書に当てはまらない学習も、ここに自由に書けます。
                  </p>
                </div>
                <div className="space-y-1">
                  <Label htmlFor="session-subject">科目</Label>
                  <SubjectSelect
                    id="session-subject"
                    ariaLabel="科目"
                    className="h-11 w-full"
                    value={manualSubject}
                    onChange={setManualSubject}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="session-textbook">参考書（任意）</Label>
                  <TextbookSelect
                    id="session-textbook"
                    ariaLabel="参考書"
                    className="h-11 w-full"
                    value={manualTextbookId}
                    onChange={setManualTextbookId}
                  />
                </div>
              </div>
            )}
          </div>

          <DialogFooter className="gap-2 sm:justify-between">
            <Button
              type="button"
              variant="ghost"
              onClick={() => {
                setPickerOpen(false);
                setManualLogOpen(true);
              }}
            >
              過去の実績を手入力
            </Button>
            <Button type="button" className="h-11" onClick={beginSession}>
              <Play aria-hidden="true" />
              計測を開始
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={session?.status === "reviewing"}
        onOpenChange={(open) => {
          if (!open && session && !saving) {
            setSession({ ...session, status: "paused" });
          }
        }}
      >
        <DialogContent
          showCloseButton={!saving}
          className="top-auto bottom-0 max-h-[90dvh] translate-y-0 overflow-y-auto rounded-b-none sm:top-1/2 sm:bottom-auto sm:max-w-lg sm:-translate-y-1/2 sm:rounded-xl"
        >
          <DialogHeader>
            <DialogTitle>おつかれさまでした</DialogTitle>
            <DialogDescription>
              計測した時間と実施内容を確認して、実績を保存します。
            </DialogDescription>
          </DialogHeader>

          {session && (
            <div className="space-y-4">
              <div className="rounded-lg bg-muted p-3">
                <p className="font-medium">{session.label}</p>
                <p className="mt-1 font-mono text-2xl font-bold tabular-nums">
                  {formatStudyElapsed(elapsedStudyMs(session))}
                </p>
              </div>

              {saveError && (
                <div role="alert" className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
                  <p>{saveError}</p>
                  {session.planId != null && (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="mt-2"
                      disabled={saving}
                      onClick={() => save(true)}
                    >
                      その他の実績として保存
                    </Button>
                  )}
                </div>
              )}

              <div className="space-y-1">
                <Label htmlFor="session-minutes">学習時間（分）</Label>
                <Input
                  id="session-minutes"
                  type="number"
                  min={1}
                  max={1440}
                  inputMode="numeric"
                  className="h-11 w-28 text-base"
                  value={minutes}
                  onChange={(event) => setMinutes(Number(event.target.value))}
                />
                {elapsedStudyMs(session) < 60_000 && (
                  <p className="text-xs text-muted-foreground">
                    1分未満のため、1分として記録します。
                  </p>
                )}
              </div>

              <div className="space-y-2">
                <Label>実施範囲（任意）</Label>
                <div className="flex flex-wrap items-start gap-2">
                  <NumberStepper
                    ariaLabel="実施範囲の開始"
                    placeholder="開始"
                    value={rangeStart}
                    onChange={setRangeStart}
                  />
                  <span className="pt-2 text-muted-foreground">〜</span>
                  <NumberStepper
                    ariaLabel="実施範囲の終了"
                    placeholder="終了"
                    value={rangeEnd}
                    onChange={setRangeEnd}
                  />
                  <RangeUnitSelect
                    ariaLabel="範囲の単位"
                    value={rangeUnit}
                    onChange={setRangeUnit}
                  />
                </div>
              </div>

              <div className="space-y-1">
                <Label htmlFor="session-memo">メモ（任意）</Label>
                <Input
                  id="session-memo"
                  value={memo}
                  maxLength={500}
                  onChange={(event) => setMemo(event.target.value)}
                />
              </div>

              {confirmDiscard && (
                <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3">
                  <p className="text-sm">計測結果を保存せずに破棄しますか？</p>
                  <div className="mt-2 flex gap-2">
                    <Button type="button" variant="outline" size="sm" onClick={() => setConfirmDiscard(false)}>
                      戻る
                    </Button>
                    <Button type="button" variant="destructive" size="sm" onClick={clearSession}>
                      破棄する
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}

          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={saving}
              onClick={() => setConfirmDiscard(true)}
            >
              保存せず終了
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => session && setSession({ ...session, status: "paused" })}
            >
              タイマーへ戻る
            </Button>
            <Button
              type="button"
              disabled={saving || minutes < 1 || minutes > 1440}
              onClick={() => save(false)}
            >
              {saving ? "保存中…" : "実績を保存"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <QuickManualStudyLogDialog
        open={manualLogOpen}
        onOpenChange={setManualLogOpen}
      />
    </div>
  );
}
