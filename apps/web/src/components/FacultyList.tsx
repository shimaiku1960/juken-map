import { toast } from "sonner";
import { Button } from "@/web/components/ui/button";
import { useGoals, useCreateGoal } from "@/web/hooks/useGoals";
import { notifyDemoReadOnly } from "@/web/lib/demo-client";

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
  // ["goals"] を購読。読み込み前は SSR で渡された prop をフォールバックに使う（チラつき防止）
  const { data: goals } = useGoals();
  // 学部ID → 現在のステータス（candidate/decided）。未登録なら未定義。
  const statusByFaculty = goals
    ? new Map(goals.map((g) => [g.faculty.id, g.status]))
    : new Map(registeredFacultyIds.map((id) => [id, "decided" as const]));

  // 通信と一覧の再取得はフック側。ここでは結果をトーストで知らせるだけにする。
  const registerMutation = useCreateGoal();

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
            className="flex items-center justify-between rounded-lg border px-4 py-3"
          >
            <div>
              <p className="font-medium">{faculty.name}</p>
              <p className="text-sm text-muted-foreground">
                {faculty.tags.map((t) => t.name).join(" / ")}
              </p>
            </div>
            <Button
              onClick={() => {
                if (readOnly) {
                  notifyDemoReadOnly();
                  return;
                }
                registerMutation.mutate(faculty.id, {
                  onSuccess: ({ duplicated }) =>
                    duplicated
                      ? toast.info("すでに追加済みです")
                      : toast.success(
                          "候補に追加しました（志望校ページで比較できます）"
                        ),
                  onError: (error) => toast.error(error.message),
                });
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
