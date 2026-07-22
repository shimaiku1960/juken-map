import { Badge } from "@/components/ui/badge";

// 連続達成日数を「🔥 N日連続」で表示する（0日のときは控えめな未着手表示）
export default function StreakBadge({ streak }: { streak: number }) {
  if (streak === 0) {
    return (
      <Badge variant="secondary" className="gap-1 text-muted-foreground">
        <span>🔥</span>
        <span>今日から連続記録を始めよう</span>
      </Badge>
    );
  }

  return (
    <Badge variant="warning" className="gap-1 text-base">
      <span>🔥</span>
      <span>{streak}日連続</span>
    </Badge>
  );
}
