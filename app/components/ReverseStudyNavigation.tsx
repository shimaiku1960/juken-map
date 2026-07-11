"use client";

import { useState } from "react";
import { toast } from "sonner";
import type { StudyLog } from "@/app/hooks/useStudyLogs";
import type { Textbook } from "@/app/hooks/useStudyPlans";
import {
  useTextbooks,
  useTextbookMasters,
  useUpdateTextbookProgress,
} from "@/app/hooks/useTextbooks";
import type { TextbookMaster } from "@/app/hooks/useTextbooks";
import {
  computeCurrentAmount,
  computeStudyNavigation,
} from "@/lib/reverseStudyNavigation";
import { todayYmd, ymdLocal } from "@/lib/date";
import { RANGE_UNITS } from "@/lib/validations/studyPlan";
import type { UpdateTextbookProgressInput } from "@/lib/validations/textbook";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

const unitLabel = (unit: string) =>
  RANGE_UNITS.find((candidate) => candidate.value === unit)?.label ?? unit;

const isProgressUnit = (
  value: string | null
): value is UpdateTextbookProgressInput["rangeUnit"] =>
  RANGE_UNITS.some((unit) => unit.value === value);

function TextbookNavigationItem({
  textbook,
  logs,
  examDate,
  master,
}: {
  textbook: Textbook;
  logs: StudyLog[];
  examDate: string | null;
  master: TextbookMaster | null;
}) {
  const updateProgress = useUpdateTextbookProgress();
  const [editing, setEditing] = useState(textbook.totalAmount == null);
  const [totalAmount, setTotalAmount] = useState<number | null>(
    textbook.totalAmount
  );
  const [rangeUnit, setRangeUnit] = useState<
    UpdateTextbookProgressInput["rangeUnit"]
  >(
    isProgressUnit(textbook.rangeUnit)
      ? textbook.rangeUnit
      : "page"
  );
  const [targetDate, setTargetDate] = useState(
    textbook.targetDate == null ? "" : ymdLocal(textbook.targetDate)
  );

  const save = () => {
    if (totalAmount == null || totalAmount < 1) {
      toast.error("総量を1以上で入力してください");
      return;
    }
    updateProgress.mutate(
      {
        id: textbook.id,
        data: { totalAmount, rangeUnit, targetDate: targetDate || null },
      },
      {
        onSuccess: () => {
          toast.success("逆算設定を保存しました");
          setEditing(false);
        },
        onError: (error) => toast.error(error.message),
      }
    );
  };

  if (editing || textbook.totalAmount == null || textbook.rangeUnit == null) {
    return (
      <li className="rounded-lg border p-3">
        <p className="mb-2 font-medium">{textbook.name}</p>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            type="number"
            min={1}
            max={100000}
            value={totalAmount ?? ""}
            onChange={(event) =>
              setTotalAmount(
                event.target.value === "" ? null : Number(event.target.value)
              )
            }
            placeholder="総量"
            className="h-9 w-28"
          />
          <select
            value={rangeUnit}
            onChange={(event) => {
              const nextUnit =
                event.target.value as UpdateTextbookProgressInput["rangeUnit"];
              setRangeUnit(nextUnit);
              const metric = master?.metrics.find(
                (candidate) => candidate.unit === nextUnit
              );
              if (metric) setTotalAmount(metric.totalAmount);
            }}
            className="h-9 rounded-md border bg-transparent px-2 text-sm"
          >
            {RANGE_UNITS.map((unit) => (
              <option key={unit.value} value={unit.value}>
                {unit.label}
              </option>
            ))}
          </select>
          <label className="flex items-center gap-2 text-sm text-gray-500">
            完了目標日
            <Input
              type="date"
              value={targetDate}
              onChange={(event) => setTargetDate(event.target.value)}
              className="h-9 w-40"
            />
          </label>
          <Button
            type="button"
            size="sm"
            onClick={save}
            disabled={updateProgress.isPending}
          >
            保存
          </Button>
          {textbook.totalAmount != null && (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => setEditing(false)}
            >
              キャンセル
            </Button>
          )}
        </div>
      </li>
    );
  }

  const today = todayYmd();
  const deadline = textbook.targetDate ?? examDate;
  const currentAmount = computeCurrentAmount(
    textbook.id,
    textbook.rangeUnit,
    logs,
    today
  );
  const label = unitLabel(textbook.rangeUnit);

  if (deadline == null) {
    return (
      <li className="rounded-lg border p-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="font-medium">{textbook.name}</p>
            <p className="mt-1 text-sm text-gray-500">
              完了目標日または第一志望の受験日を設定してください
            </p>
          </div>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            onClick={() => setEditing(true)}
          >
            設定
          </Button>
        </div>
      </li>
    );
  }

  const navigation = computeStudyNavigation({
    totalAmount: textbook.totalAmount,
    currentAmount,
    examDate: deadline,
    today,
  });
  const todayRange =
    navigation.todayStart === navigation.todayEnd
      ? `${navigation.todayStart}${label}`
      : `${navigation.todayStart}〜${navigation.todayEnd}${label}`;

  return (
    <li className="rounded-lg border p-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="font-medium">{textbook.name}</p>
          {navigation.status === "active" && (
            <p className="mt-1 text-lg font-bold text-blue-700">
              今日：{todayRange}
            </p>
          )}
          {navigation.status === "completed" && (
            <p className="mt-1 font-bold text-green-700">完了しました</p>
          )}
          {navigation.status === "expired" && (
            <p className="mt-1 font-bold text-red-700">
              {textbook.targetDate == null
                ? "受験日を迎えています"
                : "完了目標日を迎えています"}
            </p>
          )}
          <p className="mt-1 text-sm text-gray-500">
            現在 {navigation.currentAmount}/{textbook.totalAmount}{label}・残り
            {navigation.remainingAmount}{label}・あと{navigation.remainingDays}日
          </p>
          <p className="mt-1 text-xs text-gray-400">
            期限：{ymdLocal(deadline)}
            {textbook.targetDate == null ? "（受験日）" : "（完了目標日）"}
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => setEditing(true)}
        >
          設定
        </Button>
      </div>
    </li>
  );
}

export default function ReverseStudyNavigation({
  initialTextbooks,
  logs,
  examDate,
}: {
  initialTextbooks: Textbook[];
  logs: StudyLog[];
  examDate: string | null;
}) {
  const { data: textbooks = [] } = useTextbooks(initialTextbooks);
  const { data: masters = [] } = useTextbookMasters();

  return (
    <section className="mb-8">
      <h2 className="mb-3 text-xl font-bold">今日やること</h2>
      <Card>
        <CardContent className="py-5">
          {textbooks.length === 0 ? (
            <p className="text-sm text-gray-500">
              学習実績の入力欄から参考書を追加すると、今日のノルマを表示できます。
            </p>
          ) : (
            <ul className="space-y-3">
              {textbooks.map((textbook) => (
                <TextbookNavigationItem
                  key={textbook.id}
                  textbook={textbook}
                  logs={logs}
                  examDate={examDate}
                  master={
                    masters.find((master) => master.id === textbook.masterId) ??
                    null
                  }
                />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </section>
  );
}
