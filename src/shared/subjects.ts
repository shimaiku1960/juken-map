// 学習予定の科目（固定リスト）と色。カレンダーの色分け・セレクトで共有する。
export type Subject = {
  value: string;
  label: string;
  color: string;
};

export const SUBJECTS: Subject[] = [
  { value: "english", label: "英語", color: "#2563eb" },
  { value: "math", label: "数学", color: "#16a34a" },
  { value: "japanese", label: "国語", color: "#db2777" },
  { value: "science", label: "理科", color: "#ea580c" },
  { value: "social", label: "社会", color: "#9333ea" },
  { value: "other", label: "その他", color: "#64748b" },
];

// 未設定（科目なし）のときの色
export const NO_SUBJECT_COLOR = "#94a3b8";

const byValue = new Map(SUBJECTS.map((s) => [s.value, s]));

export const subjectColor = (value: string | null | undefined) =>
  (value && byValue.get(value)?.color) || NO_SUBJECT_COLOR;

export const subjectLabel = (value: string | null | undefined) =>
  (value && byValue.get(value)?.label) || "未設定";

// 科目の値として妥当か（バリデーション用）
export const SUBJECT_VALUES = SUBJECTS.map((s) => s.value);
