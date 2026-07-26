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
import { SUBJECTS, SUBJECT_VALUES, subjectColor, subjectLabel } from "@/lib/subjects";
import type { UpdateTextbookProgressInput } from "@/lib/validations/textbook";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

const unitLabel = (unit: string) =>
  RANGE_UNITS.find((candidate) => candidate.value === unit)?.label ?? unit;

const isProgressUnit = (
  value: string | null
): value is NonNullable<UpdateTextbookProgressInput["rangeUnit"]> =>
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
    NonNullable<UpdateTextbookProgressInput["rangeUnit"]>
  >(
    isProgressUnit(textbook.rangeUnit)
      ? textbook.rangeUnit
      : "page"
  );
  const [targetDate, setTargetDate] = useState(
    textbook.targetDate == null ? "" : ymdLocal(textbook.targetDate)
  );
  const [subject, setSubject] = useState<string>(textbook.subject ?? "");

  const save = () => {
    if (totalAmount == null || totalAmount < 1) {
      toast.error("総量を1以上で入力してください");
      return;
    }
    updateProgress.mutate(
      {
        id: textbook.id,
        data: {
          totalAmount,
          rangeUnit,
          targetDate: targetDate || null,
          subject: subject === "" ? null : (subject as (typeof SUBJECT_VALUES)[number]),
        },
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
                event.target.value as NonNullable<UpdateTextbookProgressInput["rangeUnit"]>;
              setRangeUnit(nextUnit);
              const metric = master?.metrics.find(
                (candidate) => candidate.unit === nextUnit
              );
              if (metric) setTotalAmount(metric.totalAmount);
            }}
            className="h-9 rounded-lg border bg-transparent px-2 text-sm"
          >
            {RANGE_UNITS.map((unit) => (
              <option key={unit.value} value={unit.value}>
                {unit.label}
              </option>
            ))}
          </select>
          <select
            value={subject}
            onChange={(event) => setSubject(event.target.value)}
            className="h-9 rounded-lg border bg-transparent px-2 text-sm"
            aria-label="対策科目"
          >
            <option value="">科目（未設定）</option>
            {SUBJECTS.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </select>
          <label className="flex items-center gap-2 text-sm text-muted-foreground">
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
            <div className="flex flex-wrap items-center gap-2">
              <p className="font-medium">{textbook.name}</p>
              {textbook.subject && (
                <Badge
                  variant="outline"
                  className="border-transparent px-1.5 font-normal"
                  style={{
                    backgroundColor: `${subjectColor(textbook.subject)}1a`,
                    color: subjectColor(textbook.subject),
                  }}
                >
                  {subjectLabel(textbook.subject)}
                </Badge>
              )}
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
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
            <p className="mt-1 text-lg font-bold text-primary">
              今日：{todayRange}
            </p>
          )}
          {navigation.status === "completed" && (
            <p className="mt-1 font-bold text-success">完了しました</p>
          )}
          {navigation.status === "expired" && (
            <p className="mt-1 font-bold text-destructive">
              {textbook.targetDate == null
                ? "受験日を過ぎています"
                : "完了目標日を過ぎています"}
            </p>
          )}
          <p className="mt-1 text-sm text-muted-foreground">
            現在 {navigation.currentAmount}/{textbook.totalAmount}{label}・残り
            {navigation.remainingAmount}{label}・あと{navigation.remainingDays}日
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
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

  // 科目ごとにまとめて表示（志望校→科目→参考書の因果を読み取りやすく）。
  // 並びは SUBJECTS の順→未設定を最後に。
  const groups = [
    ...SUBJECTS.map((s) => s.value),
    null as string | null,
  ]
    .map((subjectValue) => ({
      subjectValue,
      items: textbooks.filter(
        (textbook) => (textbook.subject ?? null) === subjectValue
      ),
    }))
    .filter((group) => group.items.length > 0);

  const renderItem = (textbook: Textbook) => (
    <TextbookNavigationItem
      key={textbook.id}
      textbook={textbook}
      logs={logs}
      examDate={examDate}
      master={masters.find((master) => master.id === textbook.masterId) ?? null}
    />
  );

  return (
    <section className="mb-8">
      <h2 className="mb-3 text-xl font-bold">今日やること</h2>
      <Card>
        <CardContent className="py-5">
          {textbooks.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              学習実績の入力欄から参考書を追加すると、今日のノルマを表示できます。
            </p>
          ) : (
            <div className="space-y-5">
              {groups.map((group) => (
                <div key={group.subjectValue ?? "none"}>
                  <div className="mb-2 flex items-center gap-2">
                    <span
                      className="h-2.5 w-2.5 rounded-full"
                      style={{ backgroundColor: subjectColor(group.subjectValue) }}
                    />
                    <h3 className="text-sm font-bold text-muted-foreground">
                      {subjectLabel(group.subjectValue)}
                    </h3>
                  </div>
                  <ul className="space-y-3">{group.items.map(renderItem)}</ul>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </section>
  );
}
