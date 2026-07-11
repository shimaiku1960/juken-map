import { Badge } from "@/components/ui/badge";

// 連続達成日数を「🔥 N日連続」で表示する（0日のときは控えめな未着手表示）
export default function StreakBadge({ streak }: { streak: number }) {
  if (streak === 0) {
    return (
      <Badge variant="secondary" className="gap-1 text-gray-500">
        <span>🔥</span>
        <span>今日から連続記録を始めよう</span>
      </Badge>
    );
  }

  return (
    <Badge className="gap-1 bg-orange-500 hover:bg-orange-500 text-base">
      <span>🔥</span>
      <span>{streak}日連続</span>
    </Badge>
  );
}
