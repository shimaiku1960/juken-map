"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";

// 学部系統タグ（seed.ts の tagNames と対応）
const TAGS = [
  "法・政経系",
  "商・経営系",
  "文・文化系",
  "理工系",
  "情報系",
  "医歯薬系",
  "農・生命系",
];

type Props = {
  value: Set<string>;
  onChange: (next: Set<string>) => void;
  children: React.ReactNode; // トリガーボタン
};

export default function FacultyTagModal({ value, onChange, children }: Props) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Set<string>>(value);

  const handleOpenChange = (next: boolean) => {
    if (next) setDraft(new Set(value));
    setOpen(next);
  };

  const toggle = (tag: string) => {
    setDraft((prev) => {
      const next = new Set(prev);
      if (next.has(tag)) next.delete(tag);
      else next.add(tag);
      return next;
    });
  };

  const handleDecide = () => {
    onChange(new Set(draft));
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>学部系統を選択</DialogTitle>
        </DialogHeader>

        <div className="grid grid-cols-2 gap-2">
          {TAGS.map((tag) => (
            <label
              key={tag}
              className="flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm hover:bg-muted/50"
            >
              <Checkbox
                checked={draft.has(tag)}
                onCheckedChange={() => toggle(tag)}
              />
              {tag}
            </label>
          ))}
        </div>

        <DialogFooter className="sm:justify-between gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => setDraft(new Set())}
          >
            条件をリセット
          </Button>
          <Button type="button" onClick={handleDecide}>
            決定
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
