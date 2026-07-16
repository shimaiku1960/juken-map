"use client";

import { useState } from "react";
import { toast } from "sonner";
import { SUBJECTS } from "@/lib/subjects";
import { RANGE_UNITS } from "@/lib/validations/studyPlan";
import {
  useTextbooks,
  useCreateTextbook,
  useTextbookMasters,
} from "@/app/hooks/useTextbooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

// 学習予定（StudyPlan）と学習実績（StudyLog）のフォームで共有する入力部品。
// 科目セレクト・参考書セレクト・範囲単位セレクト・数値ステッパー。

// 科目セレクト（固定リスト。空 = 科目なし）
export const SubjectSelect = ({
  value,
  onChange,
  id,
  ariaLabel,
  className,
}: {
  value: string | null | undefined;
  onChange: (v: string | null) => void;
  id?: string;
  ariaLabel?: string;
  className?: string;
}) => (
  <select
    id={id}
    aria-label={ariaLabel}
    value={value ?? ""}
    onChange={(e) => onChange(e.target.value || null)}
    className={cn(
      "h-9 shrink-0 rounded-md border bg-transparent px-2 text-sm",
      className
    )}
  >
    <option value="">科目なし</option>
    {SUBJECTS.map((s) => (
      <option key={s.value} value={s.value}>
        {s.label}
      </option>
    ))}
  </select>
);

// 参考書セレクト（登録済みから選択＋新規追加）。空 = 参考書なし
export const TextbookSelect = ({
  value,
  onChange,
  id,
  ariaLabel,
  className,
}: {
  value: number | null | undefined;
  onChange: (v: number | null) => void;
  id?: string;
  ariaLabel?: string;
  className?: string;
}) => {
  const { data: textbooks = [] } = useTextbooks();
  const { data: masters = [] } = useTextbookMasters();
  const createTextbook = useCreateTextbook();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");

  const submitNew = () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    createTextbook.mutate({ name: trimmed }, {
      onSuccess: (created) => {
        onChange(created.id); // 追加した参考書をそのまま選択状態に
        setName("");
        setAdding(false);
      },
      onError: (error) => toast.error(error.message),
    });
  };

  if (adding) {
    return (
      <div className="flex items-center gap-1">
        <Input
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="参考書名"
          className="h-9 w-40"
        />
        <Button
          type="button"
          size="sm"
          onClick={submitNew}
          disabled={createTextbook.isPending}
        >
          追加
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={() => {
            setAdding(false);
            setName("");
          }}
        >
          ✕
        </Button>
      </div>
    );
  }

  return (
    <select
      id={id}
      aria-label={ariaLabel}
      value={value ?? ""}
      onChange={(e) => {
        if (e.target.value === "__new__") {
          setAdding(true);
          return;
        }
        if (e.target.value.startsWith("master:")) {
          const masterId = Number(e.target.value.slice("master:".length));
          createTextbook.mutate(
            { masterId },
            {
              onSuccess: (created) => onChange(created.id),
              onError: (error) => toast.error(error.message),
            }
          );
          return;
        }
        onChange(e.target.value ? Number(e.target.value) : null);
      }}
      disabled={createTextbook.isPending}
      className={cn(
        "h-9 min-w-0 rounded-md border bg-transparent px-2 text-sm",
        className
      )}
    >
      <option value="">参考書なし</option>
      {textbooks.map((t) => (
        <option key={t.id} value={t.id}>
          {t.name}
        </option>
      ))}
      {masters
        .filter(
          (master) =>
            !textbooks.some((textbook) => textbook.masterId === master.id)
        )
        .map((master) => (
          <option key={`master-${master.id}`} value={`master:${master.id}`}>
            ＋ {master.name}
            {master.edition ? `（${master.edition}）` : ""}
          </option>
        ))}
      <option value="__new__">＋ 新しい参考書を追加</option>
    </select>
  );
};

// 範囲の単位セレクト（ページ/問題/章）。空 = 未選択
export const RangeUnitSelect = ({
  value,
  onChange,
  disabled = false,
  ariaLabel,
}: {
  value: string | null | undefined;
  onChange: (v: string | null) => void;
  disabled?: boolean;
  ariaLabel?: string;
}) => (
  <select
    aria-label={ariaLabel}
    value={value ?? ""}
    onChange={(e) => onChange(e.target.value || null)}
    disabled={disabled}
    className="h-9 rounded-md border bg-transparent px-2 text-sm"
  >
    <option value="">単位</option>
    {RANGE_UNITS.map((u) => (
      <option key={u.value} value={u.value}>
        {u.label}
      </option>
    ))}
  </select>
);

// 数値入力＋自前の増減ボタン。ユーザー好みで「上＝減る / 下＝増える」に固定する
// （ネイティブのスピナーは上＝増で反転できないため隠し、向きを自分で制御する）
export const NumberStepper = ({
  value,
  onChange,
  placeholder,
  min = 1,
  ariaLabel,
}: {
  value: number | null | undefined;
  onChange: (v: number | null) => void;
  placeholder?: string;
  min?: number;
  ariaLabel?: string;
}) => {
  const step = (delta: number) => {
    if (value == null) {
      // 空欄のとき：増やす(+1)は min から開始、減らす(-1)は空のまま
      onChange(delta > 0 ? min : null);
      return;
    }
    const next = value + delta;
    onChange(next < min ? min : next);
  };

  return (
    <div className="flex items-stretch gap-1">
      <Input
        aria-label={ariaLabel}
        type="number"
        min={min}
        placeholder={placeholder}
        className="h-9 w-16 text-center [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        value={value ?? ""}
        onChange={(e) =>
          onChange(e.target.value === "" ? null : Number(e.target.value))
        }
      />
      <div className="flex flex-col">
        {/* 上ボタン＝増やす */}
        <button
          type="button"
          aria-label="1つ増やす"
          onClick={() => step(1)}
          className="flex h-[18px] w-6 items-center justify-center rounded-t-md border text-[10px] leading-none hover:bg-accent"
        >
          ▲
        </button>
        {/* 下ボタン＝減らす */}
        <button
          type="button"
          aria-label="1つ減らす"
          onClick={() => step(-1)}
          className="flex h-[18px] w-6 items-center justify-center rounded-b-md border border-t-0 text-[10px] leading-none hover:bg-accent"
        >
          ▼
        </button>
      </div>
    </div>
  );
};
