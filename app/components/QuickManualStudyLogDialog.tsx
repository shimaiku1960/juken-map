"use client";

import { ArrowLeft } from "lucide-react";
import StudyLogForm from "@/app/components/StudyLogForm";
import { Button } from "@/components/ui/button";
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
  initialDate,
  onBack,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialDate?: string;
  onBack?: () => void;
}) {
  const dateLabel = initialDate
    ? new Intl.DateTimeFormat("ja-JP", {
        year: "numeric",
        month: "long",
        day: "numeric",
      }).format(new Date(`${initialDate}T00:00:00`))
    : null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="top-auto bottom-0 max-h-[90dvh] grid-rows-[auto_minmax(0,1fr)] translate-y-0 overflow-hidden rounded-b-none sm:top-[15dvh] sm:bottom-auto sm:max-h-[80dvh] sm:max-w-lg sm:rounded-xl">
        <DialogHeader>
          {onBack && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="-ml-2 w-fit"
              onClick={onBack}
            >
              <ArrowLeft aria-hidden="true" />
              学習を始めるに戻る
            </Button>
          )}
          <DialogTitle>学習を記録</DialogTitle>
          <DialogDescription>
            {dateLabel
              ? `${dateLabel}の学習実績として記録します。`
              : "学習時間を入力するだけですぐ記録できます。"}
          </DialogDescription>
        </DialogHeader>
        <StudyLogForm
          variant="quick"
          initialDate={initialDate}
          onSuccess={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
