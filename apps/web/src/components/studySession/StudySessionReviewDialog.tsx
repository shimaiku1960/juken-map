import {
  type ActiveStudySession,
  elapsedStudyMs,
  formatStudyElapsed,
} from "@/web/lib/studySession";
import { NumberStepper, RangeUnitSelect } from "@/web/components/StudyFields";
import { Button } from "@/web/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/web/components/ui/dialog";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";

/** 保存前に手直しできる項目。 */
export type ReviewValues = {
  minutes: number;
  rangeStart: number | null;
  rangeEnd: number | null;
  rangeUnit: string | null;
  memo: string;
};

/**
 * 計測を終えたあとの「おつかれさまでした」ダイアログ。
 *
 * 値そのものは親が持つ。中断したセッションを localStorage から復元したときも
 * 同じ値を埋め直す必要があり、その出どころは親のセッション管理だから。
 * ここは見せ方と操作の受け取りに徹する。
 */
export default function StudySessionReviewDialog({
  session,
  open,
  onOpenChange,
  values,
  onChange,
  saving,
  saveError,
  confirmDiscard,
  onSave,
  onAskDiscard,
  onCancelDiscard,
  onDiscard,
  onBackToTimer,
}: {
  session: ActiveStudySession | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  values: ReviewValues;
  onChange: (patch: Partial<ReviewValues>) => void;
  saving: boolean;
  saveError: string | null;
  confirmDiscard: boolean;
  /** asManual=true で「その他の実績として」保存する */
  onSave: (asManual: boolean) => void;
  onAskDiscard: () => void;
  onCancelDiscard: () => void;
  onDiscard: () => void;
  onBackToTimer: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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
              <div
                role="alert"
                className="rounded-lg bg-destructive/10 p-3 text-sm text-destructive"
              >
                <p>{saveError}</p>
                {session.planId != null && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="mt-2"
                    disabled={saving}
                    onClick={() => onSave(true)}
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
                value={values.minutes}
                onChange={(event) =>
                  onChange({ minutes: Number(event.target.value) })
                }
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
                  value={values.rangeStart}
                  onChange={(rangeStart) => onChange({ rangeStart })}
                />
                <span className="pt-2 text-muted-foreground">〜</span>
                <NumberStepper
                  ariaLabel="実施範囲の終了"
                  placeholder="終了"
                  value={values.rangeEnd}
                  onChange={(rangeEnd) => onChange({ rangeEnd })}
                />
                <RangeUnitSelect
                  ariaLabel="範囲の単位"
                  value={values.rangeUnit}
                  onChange={(rangeUnit) => onChange({ rangeUnit })}
                />
              </div>
            </div>

            <div className="space-y-1">
              <Label htmlFor="session-memo">メモ（任意）</Label>
              <Input
                id="session-memo"
                value={values.memo}
                maxLength={500}
                onChange={(event) => onChange({ memo: event.target.value })}
              />
            </div>

            {confirmDiscard && (
              <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3">
                <p className="text-sm">計測結果を保存せずに破棄しますか？</p>
                <div className="mt-2 flex gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={onCancelDiscard}
                  >
                    戻る
                  </Button>
                  <Button
                    type="button"
                    variant="destructive"
                    size="sm"
                    onClick={onDiscard}
                  >
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
            onClick={onAskDiscard}
          >
            保存せず終了
          </Button>
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            onClick={onBackToTimer}
          >
            タイマーへ戻る
          </Button>
          <Button
            type="button"
            disabled={saving || values.minutes < 1 || values.minutes > 1440}
            onClick={() => onSave(false)}
          >
            {saving ? "保存中…" : "実績を保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
