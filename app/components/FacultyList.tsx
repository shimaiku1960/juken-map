"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { useGoals, goalsKey } from "@/app/hooks/useGoals";
import { notifyDemoReadOnly } from "@/lib/demo-client";

type Tag = { id: number; name: string };

type Faculty = {
  id: number;
  name: string;
  examDate: Date;
  tags: Tag[];
};

type Props = {
  faculties: Faculty[];
  registeredFacultyIds: number[];
  readOnly?: boolean;
};

export default function FacultyList({
  faculties,
  registeredFacultyIds,
  readOnly = false,
}: Props) {
  const queryClient = useQueryClient();

  // ["goals"] を購読。読み込み前は SSR で渡された prop をフォールバックに使う（チラつき防止）
  const { data: goals } = useGoals();
  // 学部ID → 現在のステータス（candidate/decided）。未登録なら未定義。
  const statusByFaculty = goals
    ? new Map(goals.map((g) => [g.faculty.id, g.status]))
    : new Map(registeredFacultyIds.map((id) => [id, "decided" as const]));

  // 探す画面からは「気になる（候補）」として追加する。比較検討は志望校ページで。
  const registerMutation = useMutation({
    mutationFn: async (facultyId: number) => {
      const res = await fetch("/api/goals", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ facultyId, status: "candidate" }),
      });
      if (res.ok) return { duplicated: false };
      const err = await res.json();
      // 409 = すでに登録済み。エラー扱いにせず、キャッシュ同期だけする
      if (res.status === 409) return { duplicated: true };
      throw new Error(err.error ?? "追加に失敗しました");
    },
    onSuccess: ({ duplicated }) => {
      // ["goals"] を無効化 → GoalList 含め同じキャッシュを見る全員が最新に
      queryClient.invalidateQueries({ queryKey: goalsKey });
      if (duplicated) {
        toast.info("すでに追加済みです");
      } else {
        toast.success("候補に追加しました（志望校ページで比較できます）");
      }
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <ul className="space-y-2">
      {faculties.map((faculty) => {
        const status = statusByFaculty.get(faculty.id);
        const label =
          status === "decided"
            ? "受験校"
            : status === "candidate"
              ? "候補に追加済み"
              : "気になる";
        return (
          <li
            key={faculty.id}
            className="border rounded px-4 py-3 flex justify-between items-center"
          >
            <div>
              <p className="font-medium">{faculty.name}</p>
              <p className="text-sm text-gray-500">
                {faculty.tags.map((t) => t.name).join(" / ")}
              </p>
            </div>
            <Button
              onClick={() => {
                if (readOnly) {
                  notifyDemoReadOnly();
                  return;
                }
                registerMutation.mutate(faculty.id);
              }}
              disabled={status !== undefined || registerMutation.isPending}
              title={readOnly && status === undefined ? "デモアカウントは閲覧専用です" : undefined}
              variant={status !== undefined ? "secondary" : "default"}
            >
              {label}
            </Button>
          </li>
        );
      })}
    </ul>
  );
}
