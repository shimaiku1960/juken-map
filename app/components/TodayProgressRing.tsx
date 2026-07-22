type Props = {
  done: number;
  total: number;
};

// 今日の達成率を円形プログレスリングで表示する（表示専用）
export default function TodayProgressRing({ done, total }: Props) {
  const ratio = total === 0 ? 0 : done / total;
  const percent = Math.round(ratio * 100);

  const size = 96; // 全体のピクセルサイズ
  const stroke = 8; // 線の太さ
  const radius = (size - stroke) / 2; // 中心から線の中央までの半径
  const circumference = 2 * Math.PI * radius; // 円周の長さ
  const offset = circumference * (1 - ratio); // 進捗ぶんだけ残す

  return (
    <div className="relative inline-flex items-center justify-center">
      <svg width={size} height={size} className="-rotate-90">
        {/* 背景の円 */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="currentColor"
          strokeWidth={stroke}
          className="text-muted"
        />
        {/* 進捗の円 */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="currentColor"
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          className="text-primary transition-all"
        />
      </svg>
      <span className="absolute text-lg font-bold">{percent}%</span>
    </div>
  );
}
