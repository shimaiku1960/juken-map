import { describe, expect, it } from "vitest";
import {
  getSupportSubscriptionAvailabilityNotice,
  getSupportSubscriptionDateLabel,
} from "@/lib/support-subscription-display";

const baseInput = {
  trialEndsAt: null,
  currentPeriodEndsAt: "2026-09-18T06:43:28.000Z",
  cancelAtPeriodEnd: false,
  cancelAt: null,
};

describe("support subscription date label", () => {
  it("契約中だけ次回更新日を表示する", () => {
    expect(
      getSupportSubscriptionDateLabel({ ...baseInput, status: "active" })
    ).toBe("次回更新日は2026年9月18日です。");
  });

  it("契約中の解約予約は次回更新日より優先する", () => {
    expect(
      getSupportSubscriptionDateLabel({
        ...baseInput,
        status: "active",
        cancelAtPeriodEnd: true,
      })
    ).toBe("2026年9月18日に解約予定です。");
  });

  it("無料体験中は無料体験終了日を表示する", () => {
    expect(
      getSupportSubscriptionDateLabel({
        ...baseInput,
        status: "trialing",
        trialEndsAt: "2026-08-18T06:43:28.000Z",
      })
    ).toBe("無料体験は2026年8月18日までです。");
  });

  it.each(["canceled", "past_due", "unpaid", "paused", "incomplete"])(
    "%sでは期間終了日が残っていても次回更新日を表示しない",
    (status) => {
      expect(
        getSupportSubscriptionDateLabel({ ...baseInput, status })
      ).toBeNull();
    }
  );

  it("未知の状態では日付を表示しない", () => {
    expect(
      getSupportSubscriptionDateLabel({ ...baseInput, status: "unknown" })
    ).toBeNull();
  });
});

describe("support subscription availability notice", () => {
  it("解約済みではLINE質問サポートを利用できない理由を表示する", () => {
    expect(getSupportSubscriptionAvailabilityNotice("canceled")).toBe(
      "契約期間が終了したため、LINE質問サポートは現在利用できません。再度利用する場合は、LINEから再申込みをご相談ください。"
    );
  });

  it.each(["trialing", "active", "past_due", "unpaid", "paused"])(
    "%sでは解約済み向けの案内を表示しない",
    (status) => {
      expect(getSupportSubscriptionAvailabilityNotice(status)).toBeNull();
    }
  );
});
