"use client";

import StudyLogForm from "@/app/components/StudyLogForm";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export default function QuickManualStudyLogDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-auto bottom-0 max-h-[90dvh] translate-y-0 overflow-y-auto rounded-b-none sm:top-1/2 sm:bottom-auto sm:max-w-lg sm:-translate-y-1/2 sm:rounded-xl">
        <DialogHeader>
          <DialogTitle>学習を記録</DialogTitle>
          <DialogDescription>
            学習時間を入力するだけですぐ記録できます。
          </DialogDescription>
        </DialogHeader>
        <StudyLogForm
          variant="quick"
          onSuccess={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
