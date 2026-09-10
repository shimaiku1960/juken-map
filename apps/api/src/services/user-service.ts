import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

/** ニックネームなどプロフィールを更新する。 */
export function updateProfile(userId: string, data: { nickname: string }) {
  return measured("user.updateProfile", () =>
    prisma.user.update({ where: { id: userId }, data })
  );
}

/**
 * 新規登録の計測を「まだ送っていなければ」記録する。
 *
 * where に analyticsSignUpTrackedAt: null を入れて DB 側で判定させることで、
 * 同じユーザーが同時に2回開いても計測が二重に飛ばない。
 * 戻り値 true は「今回が初回だった」の意味。
 */
export function markSignUpTracked(userId: string) {
  return measured("user.markSignUpTracked", async () => {
    const tracked = await prisma.user.updateMany({
      where: { id: userId, analyticsSignUpTrackedAt: null },
      data: { analyticsSignUpTrackedAt: new Date() },
    });
    return tracked.count > 0;
  });
}

/** 登録に使われた認証方法を、最初に作られたアカウントから判定する。 */
export function findSignUpMethod(userId: string) {
  return measured("user.findSignUpMethod", async () => {
    const account = await prisma.account.findFirst({
      where: { userId },
      select: { providerId: true },
      orderBy: { createdAt: "asc" },
    });
    return account?.providerId === "google" || account?.providerId === "github"
      ? account.providerId
      : "email";
  });
}
