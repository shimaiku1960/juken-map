"use client";

import { toast } from "sonner";
import { type StudyLog, useDeleteStudyLog } from "@/app/hooks/useStudyLogs";
import { studyLogLabel } from "@/lib/studyLog";
import { subjectColor } from "@/lib/subjects";
import { Button } from "@/components/ui/button";
import { notifyDemoReadOnly } from "@/lib/demo-client";

const toDateStr = (d: string) => d.slice(0, 10);

// 最近の学習実績を新しい順に数件表示する（削除ボタン付き）。
export default function RecentStudyLogs({
  logs,
  limit = 5,
  readOnly = false,
}: {
  logs: StudyLog[];
  limit?: number;
  readOnly?: boolean;
}) {
  const deleteLog = useDeleteStudyLog();

  if (logs.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        まだ記録がありません。画面上部の「学習を記録」から始めましょう。
      </p>
    );
  }

  const recent = logs.slice(0, limit);

  return (
    <ul className="space-y-2">
      {recent.map((log) => (
        <li
          key={log.id}
          className="flex items-center gap-3 rounded border px-3 py-2"
        >
          <span
            className="h-3 w-3 shrink-0 rounded-full"
            style={{ backgroundColor: subjectColor(log.subject) }}
          />
          <div className="min-w-0 flex-1">
            <p className="text-xs text-muted-foreground">{toDateStr(log.date)}</p>
            <p className="truncate text-sm font-medium">{studyLogLabel(log)}</p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            title={readOnly ? "デモアカウントは閲覧専用です" : undefined}
            onClick={() => {
              if (readOnly) {
                notifyDemoReadOnly();
                return;
              }
              deleteLog.mutate(log.id, {
                onSuccess: () => toast.success("削除しました"),
                onError: (error) => toast.error(error.message),
              });
            }}
            disabled={deleteLog.isPending}
            aria-label="削除"
          >
            削除
          </Button>
        </li>
      ))}
    </ul>
  );
}
