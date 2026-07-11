import type { SubjectMinutes } from "@/lib/studyStats";
import { formatMinutes } from "@/lib/studyLog";

// 科目別の学習時間を横棒で可視化（外部ライブラリ不要・div 幅％の自作バー）。
export default function SubjectMinutesBars({
  data,
}: {
  data: SubjectMinutes[];
}) {
  const max = Math.max(1, ...data.map((d) => d.minutes));
  const total = data.reduce((sum, d) => sum + d.minutes, 0);

  if (total === 0) {
    return (
      <p className="text-sm text-gray-500">
        今週の記録はまだありません。学習を記録すると科目ごとの時間が表示されます。
      </p>
    );
  }

  return (
    <div className="space-y-2">
      {data.map((d) => (
        <div key={d.value} className="flex items-center gap-3">
          <span className="w-10 shrink-0 text-sm text-gray-600">{d.label}</span>
          <div className="h-4 flex-1 overflow-hidden rounded bg-gray-100">
            <div
              className="h-full rounded"
              style={{
                width: `${(d.minutes / max) * 100}%`,
                backgroundColor: d.color,
              }}
            />
          </div>
          <span className="w-20 shrink-0 text-right text-sm tabular-nums text-gray-600">
            {d.minutes === 0 ? "—" : formatMinutes(d.minutes)}
          </span>
        </div>
      ))}
    </div>
  );
}
