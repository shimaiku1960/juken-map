import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";

export type TodayPlan = {
  id: number;
  content: string;
  done: boolean;
};

export default function TodayStudyPlans({
  plans,
  weekCount,
}: {
  plans: TodayPlan[];
  weekCount: number;
}) {
  const doneCount = plans.filter((p) => p.done).length;

  return (
    <Card>
      <CardContent className="py-5">
        {plans.length === 0 ? (
          <p className="text-gray-600">今日の学習予定はありません。</p>
        ) : (
          <>
            <p className="mb-2 text-sm font-medium text-gray-600">
              達成 {doneCount} / {plans.length}
            </p>
            <ul className="space-y-1">
              {plans.map((p) => (
                <li key={p.id} className="flex items-start gap-2">
                  <span>{p.done ? "✅" : "⬜️"}</span>
                  <span className={p.done ? "line-through text-gray-400" : ""}>
                    {p.content}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
        <div className="mt-3 flex items-center justify-between text-sm text-gray-500">
          <span>今後7日間の予定：{weekCount} 件</span>
          <Link href="/schedule" className="text-blue-600 hover:underline">
            学習予定カレンダー →
          </Link>
        </div>
      </CardContent>
    </Card>
  );
}
