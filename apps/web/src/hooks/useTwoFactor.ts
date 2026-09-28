import { useQuery } from "@tanstack/react-query";
import { authClient } from "@/web/lib/auth-client";

// 管理者の2段階認証まわりの読み取り。2段階認証を有効にするにはパスワードが要るので、
// 管理画面はこれでパスワードの有無を見て、先にパスワードの設定を案内するか決める。

export const linkedAccountsKey = ["auth", "accounts"] as const;

/** この人がメール＋パスワードでログインできるか（パスワードのアカウントがあるか）。 */
export function useHasPassword() {
  return useQuery({
    queryKey: linkedAccountsKey,
    queryFn: async () => {
      const { data, error } = await authClient.listAccounts();
      if (error) throw new Error(error.message ?? "ログイン方法を読み込めませんでした");
      return data.some((account) => account.providerId === "credential");
    },
  });
}
