"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import QuickManualStudyLogDialog from "@/app/components/QuickManualStudyLogDialog";
import { Button } from "@/components/ui/button";

export default function QuickManualStudyLogButton() {
  const [isOpen, setIsOpen] = useState(false);

  return (
    <>
      <Button
        type="button"
        className="h-11 w-full sm:w-auto"
        onClick={() => setIsOpen(true)}
      >
        <Plus aria-hidden="true" />
        学習を記録
      </Button>
      <QuickManualStudyLogDialog open={isOpen} onOpenChange={setIsOpen} />
    </>
  );
}
