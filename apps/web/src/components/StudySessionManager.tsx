import { useEffect, useMemo, useState } from "react";
import { Play } from "lucide-react";
import { notifyDemoReadOnly } from "@/web/lib/demo-client";
import { ApiError } from "@/web/lib/api-client";
import { toast } from "sonner";
import QuickManualStudyLogDialog from "@/web/components/QuickManualStudyLogDialog";
import StudySessionPickerDialog, {
  type SelectablePlan,
} from "@/web/components/studySession/StudySessionPickerDialog";
import StudySessionTimerOverlay from "@/web/components/studySession/StudySessionTimerOverlay";
import StudySessionReviewDialog, {
  type ReviewValues,
} from "@/web/components/studySession/StudySessionReviewDialog";
import { useSaveStudySession } from "@/web/hooks/useStudyLogs";
import { useStudyPlans, type StudyPlan } from "@/web/hooks/useStudyPlans";
import { todayYmd } from "@/shared/date";
import { studyPlanLabel } from "@/web/lib/studyPlan";
import {
  type ActiveStudySession,
  type StudySessionTarget,
  elapsedStudyMs,
  parseStoredStudySession,
  pauseStudySession,
  recordedMinutes,
  resumeStudySession,
  reviewStudySession,
  startStudySession,
  studySessionStorageKey,
} from "@/web/lib/studySession";
import { Button } from "@/web/components/ui/button";

const EMPTY_REVIEW: ReviewValues = {
  minutes: 1,
  rangeStart: null,
  rangeEnd: null,
  rangeUnit: null,
  memo: "",
};

// 中断していたセッションを復元したときは、保存前の入力欄もその内容で埋め直す。
function reviewValuesOf(session: ActiveStudySession): ReviewValues {
  return {
    minutes: recordedMinutes(session),
    rangeStart: session.rangeStart,
    rangeEnd: session.rangeEnd,
    rangeUnit: session.rangeUnit,
    memo: "",
  };
}

/**
 * 学習セッション（計測）の進行役。
 *
 * ここが持つのはセッションそのものの状態だけ＝復元・保存・1秒ごとの更新と、
 * 開始／終了／保存の流れ。画面は3つに分けてある：
 *   - 何を勉強するか選ぶ → studySession/StudySessionPickerDialog
 *   - 計測中の全画面表示 → studySession/StudySessionTimerOverlay
 *   - 終了後の確認と保存 → studySession/StudySessionReviewDialog
 */
export default function StudySessionManager({
  userId,
  readOnly = false,
  variant = "compact",
}: {
  userId: string;
  readOnly?: boolean;
  /** hero: ログイン直後の集中スタート画面向けに、ボタンを中央・大きく表示する */
  variant?: "compact" | "hero";
}) {
  const saveSession = useSaveStudySession();
  // この画面が使うのは「今日の予定」だけなので、その1日ぶんしか取らない。
  const today = todayYmd();
  const { data: studyPlans = [] } = useStudyPlans({ from: today, to: today });
  // 今日の予定のうち、まだ実績を記録していないものだけを選ばせる。
  const selectablePlans = useMemo<SelectablePlan[]>(
    () =>
      studyPlans
        .filter(
          (plan) =>
            plan.date.slice(0, 10) === today && plan.studyLogId == null
        )
        .map((plan) => ({
          id: plan.id,
          content: studyPlanLabel(plan),
          subject: plan.subject,
          textbookId: plan.textbookId,
          rangeStart: plan.rangeStart,
          rangeEnd: plan.rangeEnd,
          rangeUnit: plan.rangeUnit,
        })),
    [studyPlans]
  );
  const storageKey = studySessionStorageKey(userId);

  const [hydrated, setHydrated] = useState(false);
  const [session, setSession] = useState<ActiveStudySession | null>(null);
  const [now, setNow] = useState(() => Date.now());
  const [pickerOpen, setPickerOpen] = useState(false);
  // 開くたびに選びかけの状態を作り直すための鍵。閉じるアニメーションを残したい
  // ので、条件付きで描くのではなく key を変えて作り直す。
  const [pickerKey, setPickerKey] = useState(0);
  const [manualLogOpen, setManualLogOpen] = useState(false);
  const [review, setReview] = useState<ReviewValues>(EMPTY_REVIEW);
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
        setReview(reviewValuesOf(restored));
      }
      setHydrated(true);
    }, 0);

    const sync = (event: StorageEvent) => {
      if (event.key === storageKey) {
        const synced = parseStoredStudySession(event.newValue);
        setSession(synced);
        if (synced?.status === "reviewing") {
          setReview(reviewValuesOf(synced));
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

  const clearSession = () => {
    window.localStorage.removeItem(storageKey);
    setSession(null);
  };

  const openPicker = () => {
    setPickerKey((current) => current + 1);
    setPickerOpen(true);
  };

  const beginSession = (target: StudySessionTarget) => {
    const timestamp = Date.now();
    setSession(startStudySession(target, timestamp));
    setNow(timestamp);
    setPickerOpen(false);
    toast.success("学習時間の計測を開始しました");
  };

  const moveToReview = () => {
    if (!session) return;
    const reviewed = reviewStudySession(session);
    setSession(reviewed);
    // 「その他の学習」で自由入力した内容（＝ラベル）をメモの初期値に引き継ぎ、
    // 記録に「何をやったか」を残せるようにする。予定・参考書由来のラベルは対象外。
    const freeTextLabel =
      reviewed.planId == null &&
      reviewed.textbookId == null &&
      reviewed.label !== "その他の学習"
        ? reviewed.label
        : "";
    setReview({ ...reviewValuesOf(reviewed), memo: freeTextLabel });
    setSaveError(null);
    setConfirmDiscard(false);
  };

  const save = (asManual = false) => {
    if (!session) return;
    setSaveError(null);
    const planId = asManual ? null : session.planId;

    // 送る中身は経路で変わる。予定の完了側は、予定が既に持っている内容
    // （日付・科目・参考書）をサーバーが引き継ぐので送らない。
    const input =
      planId == null
        ? {
            planId: null as null,
            data: {
              date: todayYmd(),
              minutes: review.minutes,
              subject: session.subject,
              textbookId: session.textbookId,
              rangeStart: review.rangeStart,
              rangeEnd: review.rangeEnd,
              rangeUnit: review.rangeUnit,
              memo: review.memo,
            },
          }
        : {
            planId,
            data: {
              minutes: review.minutes,
              rangeStart: review.rangeStart,
              rangeEnd: review.rangeEnd,
              rangeUnit: review.rangeUnit,
              memo: review.memo,
            },
          };

    // 計測（trackEvent）と一覧の再取得はフック側。ここでは画面の後始末だけ行う。
    saveSession.mutate(input, {
      onSuccess: () => {
        clearSession();
        setConfirmDiscard(false);
        // Next.js ではサーバー描画分の更新に router.refresh() が要ったが、SPA では
        // フック側の invalidateQueries だけで画面が最新になるため不要。
        toast.success(
          planId == null
            ? "学習実績を保存しました"
            : "実績を保存し、予定を完了しました"
        );
      },
      // サーバーが理由を返したときはその文言を出す。通信自体が届かなかった
      // ときだけ、つなぎ直しを促す固定の文言にする。
      onError: (error) =>
        setSaveError(
          error instanceof ApiError
            ? error.message
            : "通信に失敗しました。接続を確認して、もう一度お試しください"
        ),
    });
  };

  const elapsed = session ? elapsedStudyMs(session, now) : 0;
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
          className={isHero ? "h-14 px-8 text-lg" : "h-11 w-full sm:w-auto"}
          disabled
        >
          タイマーを確認中…
        </Button>
      ) : session ? null : (
        <Button
          type="button"
          className={isHero ? "h-14 px-8 text-lg" : "h-11 w-full sm:w-auto"}
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
        <StudySessionTimerOverlay
          session={session}
          elapsed={elapsed}
          onPause={() => setSession(pauseStudySession(session))}
          onResume={() => setSession(resumeStudySession(session))}
          onFinish={moveToReview}
        />
      )}

      <StudySessionPickerDialog
        key={pickerKey}
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        selectablePlans={selectablePlans}
        onStart={beginSession}
        onManualLog={() => {
          setPickerOpen(false);
          setManualLogOpen(true);
        }}
      />

      <StudySessionReviewDialog
        session={session}
        open={session?.status === "reviewing"}
        onOpenChange={(open) => {
          if (!open && session && !saveSession.isPending) {
            setSession({ ...session, status: "paused" });
          }
        }}
        values={review}
        onChange={(patch) => setReview((current) => ({ ...current, ...patch }))}
        saving={saveSession.isPending}
        saveError={saveError}
        confirmDiscard={confirmDiscard}
        onSave={save}
        onAskDiscard={() => setConfirmDiscard(true)}
        onCancelDiscard={() => setConfirmDiscard(false)}
        onDiscard={clearSession}
        onBackToTimer={() =>
          session && setSession({ ...session, status: "paused" })
        }
      />

      <QuickManualStudyLogDialog
        open={manualLogOpen}
        onOpenChange={setManualLogOpen}
        onBack={() => {
          setManualLogOpen(false);
          setPickerOpen(true);
        }}
      />
    </div>
  );
}
