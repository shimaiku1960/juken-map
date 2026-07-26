"use client";

import type { StudyLog } from "@/app/hooks/useStudyLogs";
import StudyLogForm from "@/app/components/StudyLogForm";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export default function StudyLogEditDialog({
  log,
  onOpenChange,
}: {
  log: StudyLog | null;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={log !== null} onOpenChange={onOpenChange}>
      <DialogContent className="top-auto bottom-0 max-h-[90dvh] grid-rows-[auto_minmax(0,1fr)] translate-y-0 overflow-hidden rounded-b-none sm:top-[15dvh] sm:bottom-auto sm:max-h-[80dvh] sm:max-w-lg sm:rounded-xl">
        <DialogHeader>
          <DialogTitle>学習実績を編集</DialogTitle>
          <DialogDescription>
            日付、学習時間、科目、教材やメモを変更できます。
          </DialogDescription>
        </DialogHeader>
        {log ? (
          <StudyLogForm
            key={log.id}
            variant="quick"
            initialLog={log}
            onSuccess={() => onOpenChange(false)}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
