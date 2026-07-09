import { describe, it, expect } from "vitest";
import {
  subjectColor,
  subjectLabel,
  NO_SUBJECT_COLOR,
} from "./subjects";

describe("subjectColor", () => {
  it("妥当な科目の色を返す", () => {
    expect(subjectColor("english")).toBe("#2563eb");
    expect(subjectColor("math")).toBe("#16a34a");
  });

  it("未知の値はフォールバック色を返す", () => {
    expect(subjectColor("foo")).toBe(NO_SUBJECT_COLOR);
  });

  it("null / undefined / 空文字はフォールバック色を返す", () => {
    expect(subjectColor(null)).toBe(NO_SUBJECT_COLOR);
    expect(subjectColor(undefined)).toBe(NO_SUBJECT_COLOR);
    expect(subjectColor("")).toBe(NO_SUBJECT_COLOR);
  });
});

describe("subjectLabel", () => {
  it("妥当な科目のラベル（日本語）を返す", () => {
    expect(subjectLabel("english")).toBe("英語");
    expect(subjectLabel("social")).toBe("社会");
  });

  it("未知の値は「未設定」を返す", () => {
    expect(subjectLabel("foo")).toBe("未設定");
  });

  it("null / undefined / 空文字は「未設定」を返す", () => {
    expect(subjectLabel(null)).toBe("未設定");
    expect(subjectLabel(undefined)).toBe("未設定");
    expect(subjectLabel("")).toBe("未設定");
  });
});
