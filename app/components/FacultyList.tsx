"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";

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
};

// GoalList と同じ ["goals"] キャッシュを共有するため、goals を丸ごと取得する
// （queryKey が同じなら data の形も揃える必要がある）
async function fetchGoals(): Promise<{ faculty: { id: number } }[]> {
  const res = await fetch("/api/goals");
  if (!res.ok) throw new Error("志望校の取得に失敗しました");
  return res.json();
}

export default function FacultyList({
  faculties,
  registeredFacultyIds,
}: Props) {
  const queryClient = useQueryClient();

  // ["goals"] を購読。読み込み前は SSR で渡された prop をフォールバックに使う（チラつき防止）
  const { data: goals } = useQuery({
    queryKey: ["goals"],
    queryFn: fetchGoals,
  });
  const registeredIds = goals
    ? goals.map((g) => g.faculty.id)
    : registeredFacultyIds;

  const registerMutation = useMutation({
    mutationFn: async (facultyId: number) => {
      const res = await fetch("/api/goals", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ facultyId }),
      });
      if (res.ok) return { duplicated: false };
      const err = await res.json();
      // 409 = すでに登録済み。エラー扱いにせず、キャッシュ同期だけする
      if (res.status === 409) return { duplicated: true };
      throw new Error(err.error ?? "追加に失敗しました");
    },
    onSuccess: ({ duplicated }) => {
      // ["goals"] を無効化 → GoalList 含め同じキャッシュを見る全員が最新に
      queryClient.invalidateQueries({ queryKey: ["goals"] });
      if (duplicated) {
        toast.info("すでに登録済みです");
      } else {
        toast.success("志望校に追加しました");
      }
    },
    onError: (error) => toast.error(error.message),
  });

  return (
    <ul className="space-y-2">
      {faculties.map((faculty) => {
        const isRegistered = registeredIds.includes(faculty.id);
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
              onClick={() => registerMutation.mutate(faculty.id)}
              disabled={isRegistered || registerMutation.isPending}
              variant={isRegistered ? "secondary" : "default"}
            >
              {isRegistered ? "登録済み" : "志望校に追加"}
            </Button>
          </li>
        );
      })}
    </ul>
  );
}
