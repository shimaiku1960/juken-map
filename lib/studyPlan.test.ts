import { describe, it, expect } from "vitest";
import { studyPlanLabel } from "@/lib/studyPlan";

// studyPlanLabel の引数（省略分は null 埋め）を作るヘルパー
const plan = (over: {
  textbook?: { name: string } | null;
  rangeStart?: number | null;
  rangeEnd?: number | null;
  rangeUnit?: string | null;
  content?: string | null;
}) => ({
  textbook: over.textbook ?? null,
  rangeStart: over.rangeStart ?? null,
  rangeEnd: over.rangeEnd ?? null,
  rangeUnit: over.rangeUnit ?? null,
  content: over.content ?? null,
});

describe("studyPlanLabel", () => {
  it("参考書＋範囲＋メモを全部つなげる", () => {
    const label = studyPlanLabel(
      plan({
        textbook: { name: "青チャートIA" },
        rangeStart: 12,
        rangeEnd: 45,
        rangeUnit: "page",
        content: "文法だけ",
      })
    );
    expect(label).toBe("青チャートIA 12〜45ページ（文法だけ）");
  });

  it("参考書だけ", () => {
    const label = studyPlanLabel(plan({ textbook: { name: "青チャートIA" } }));
    expect(label).toBe("青チャートIA");
  });

  it("参考書＋範囲（メモなし）", () => {
    const label = studyPlanLabel(
      plan({
        textbook: { name: "青チャートIA" },
        rangeStart: 12,
        rangeEnd: 45,
        rangeUnit: "page",
      })
    );
    expect(label).toBe("青チャートIA 12〜45ページ");
  });

  it("範囲だけ（参考書なし）", () => {
    const label = studyPlanLabel(
      plan({ rangeStart: 1, rangeEnd: 10, rangeUnit: "question" })
    );
    expect(label).toBe("1〜10問題");
  });

  it("メモだけ", () => {
    const label = studyPlanLabel(plan({ content: "単語100個" }));
    expect(label).toBe("単語100個");
  });

  it("参考書＋メモ（範囲なし）", () => {
    const label = studyPlanLabel(
      plan({ textbook: { name: "ターゲット1900" }, content: "1章" })
    );
    expect(label).toBe("ターゲット1900（1章）");
  });

  it("単位ごとにラベルが変わる", () => {
    expect(
      studyPlanLabel(plan({ rangeStart: 1, rangeEnd: 3, rangeUnit: "chapter" }))
    ).toBe("1〜3章");
    expect(
      studyPlanLabel(plan({ rangeStart: 5, rangeEnd: 8, rangeUnit: "number" }))
    ).toBe("5〜8番");
  });

  it("範囲は開始と終了の両方が揃っていないと出さない", () => {
    // 開始だけ → 範囲は表示されず、他が空なので「（内容なし）」
    expect(studyPlanLabel(plan({ rangeStart: 12 }))).toBe("（内容なし）");
    // 終了だけ → 同上
    expect(studyPlanLabel(plan({ rangeEnd: 45 }))).toBe("（内容なし）");
  });

  it("単位が null/未知の値なら単位表記は空になる", () => {
    expect(studyPlanLabel(plan({ rangeStart: 1, rangeEnd: 5 }))).toBe("1〜5");
    expect(
      studyPlanLabel(plan({ rangeStart: 1, rangeEnd: 5, rangeUnit: "unknown" }))
    ).toBe("1〜5");
  });

  it("開始 = 終了（1ページだけ）も表示する", () => {
    const label = studyPlanLabel(
      plan({ rangeStart: 20, rangeEnd: 20, rangeUnit: "page" })
    );
    expect(label).toBe("20〜20ページ");
  });

  it("中身が全部空なら「（内容なし）」", () => {
    expect(studyPlanLabel(plan({}))).toBe("（内容なし）");
  });
});
