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
import { REGIONS } from "@/lib/prefectures";

type Props = {
  value: Set<string>;
  onChange: (next: Set<string>) => void;
  children: React.ReactNode; // トリガーボタン
};

export default function PrefectureModal({ value, onChange, children }: Props) {
  const [open, setOpen] = useState(false);
  // モーダル内の一時選択（決定するまで親に反映しない）
  const [draft, setDraft] = useState<Set<string>>(value);

  // 開くたびに現在の選択で初期化
  const handleOpenChange = (next: boolean) => {
    if (next) setDraft(new Set(value));
    setOpen(next);
  };

  const toggle = (pref: string) => {
    setDraft((prev) => {
      const next = new Set(prev);
      if (next.has(pref)) next.delete(pref);
      else next.add(pref);
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
      <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>都道府県を選択</DialogTitle>
        </DialogHeader>

        <div className="space-y-5">
          {REGIONS.map(({ region, prefs }) => (
            <div key={region}>
              <p className="text-sm font-bold mb-2">{region}</p>
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                {prefs.map((pref) => (
                  <label
                    key={pref}
                    className="flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm hover:bg-muted/50"
                  >
                    <Checkbox
                      checked={draft.has(pref)}
                      onCheckedChange={() => toggle(pref)}
                    />
                    {pref}
                  </label>
                ))}
              </div>
            </div>
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
