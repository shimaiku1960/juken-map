import { Pause, Play, Square } from "lucide-react";
import {
  type ActiveStudySession,
  formatStudyElapsed,
} from "@/web/lib/studySession";
import { Button } from "@/web/components/ui/button";

/**
 * 計測中・一時停止中に全画面で出るタイマー。
 * 状態は持たず、押されたことを親へ伝えるだけ。
 */
export default function StudySessionTimerOverlay({
  session,
  elapsed,
  onPause,
  onResume,
  onFinish,
}: {
  session: ActiveStudySession;
  /** 経過ミリ秒。1秒ごとに数え直すのは親の仕事。 */
  elapsed: number;
  onPause: () => void;
  onResume: () => void;
  onFinish: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-8 bg-background px-6 text-center">
      <div className="flex flex-col items-center gap-3">
        <p className="text-sm font-medium text-primary">
          {session.status === "running" ? "● 計測中" : "一時停止中"}
        </p>
        <p className="max-w-md truncate text-lg font-medium">{session.label}</p>
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
            onClick={onPause}
          >
            <Pause aria-hidden="true" />
            一時停止
          </Button>
        ) : (
          <Button type="button" className="h-12 sm:w-40" onClick={onResume}>
            <Play aria-hidden="true" />
            再開
          </Button>
        )}
        <Button type="button" className="h-12 sm:w-40" onClick={onFinish}>
          <Square aria-hidden="true" />
          学習を終了
        </Button>
      </div>
    </div>
  );
}
