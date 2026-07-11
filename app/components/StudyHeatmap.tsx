import type { HeatmapCell } from "@/lib/studyStats";
import { formatMinutes } from "@/lib/studyLog";

// 合計学習時間（分）→ 色の濃さ（4段階）。0分は薄いグレー。
function levelClass(minutes: number): string {
  if (minutes <= 0) return "bg-gray-100";
  if (minutes < 30) return "bg-green-200";
  if (minutes < 60) return "bg-green-400";
  if (minutes < 120) return "bg-green-500";
  return "bg-green-700";
}

const WEEKDAYS = ["日", "月", "火", "水", "木", "金", "土"];

export default function StudyHeatmap({
  weeks,
  monthLabel,
}: {
  weeks: HeatmapCell[][];
  monthLabel: string;
}) {
  return (
    <div>
      <p className="mb-2 text-sm font-medium text-gray-600">
        {monthLabel} の学習時間
      </p>

      {/* 曜日ヘッダー */}
      <div className="mb-1 grid grid-cols-7 gap-1">
        {WEEKDAYS.map((w) => (
          <div key={w} className="text-center text-xs text-gray-400">
            {w}
          </div>
        ))}
      </div>

      {/* 週 × 7列のマス */}
      <div className="space-y-1">
        {weeks.map((week, wi) => (
          <div key={wi} className="grid grid-cols-7 gap-1">
            {week.map((cell, ci) =>
              cell.ymd === null ? (
                <div key={ci} className="aspect-square" />
              ) : (
                <div
                  key={ci}
                  title={`${cell.ymd}：${formatMinutes(cell.minutes)}`}
                  className={`flex aspect-square items-center justify-center rounded-sm text-[10px] text-gray-500 ${levelClass(
                    cell.minutes
                  )}`}
                >
                  {Number(cell.ymd.slice(-2))}
                </div>
              )
            )}
          </div>
        ))}
      </div>

      {/* 凡例 */}
      <div className="mt-3 flex items-center justify-end gap-1 text-xs text-gray-400">
        <span>少</span>
        <span className="h-3 w-3 rounded-sm bg-gray-100" />
        <span className="h-3 w-3 rounded-sm bg-green-200" />
        <span className="h-3 w-3 rounded-sm bg-green-400" />
        <span className="h-3 w-3 rounded-sm bg-green-500" />
        <span className="h-3 w-3 rounded-sm bg-green-700" />
        <span>多</span>
      </div>
    </div>
  );
}
