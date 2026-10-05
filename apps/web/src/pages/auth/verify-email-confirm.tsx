import { useState } from "react";
import { Link } from "react-router";
import { authClient } from "@/web/lib/auth-client";
import { Button } from "@/web/components/ui/button";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import { useSafeCallbackURL } from "@/web/hooks/useBrowserNavigation";
import { useTokenFromLink } from "@/web/hooks/useTokenFromLink";

// 確認メールのリンクで開く画面。リンクを開いただけではトークンを使わず、ボタンの POST で使う
// （認証基準 10 の E2）。メールのセキュリティ製品やプレビューがリンクを先に開いても、確認は済まない。
// 確認できてもログインはさせず、ログインの画面へ案内する。
export default function VerifyEmailConfirmPage() {
  const token = useTokenFromLink();
  const callbackURL = useSafeCallbackURL("/dashboard");
  const [state, setState] = useState<"ready" | "loading" | "done">("ready");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const loginHref = callbackURL === "/dashboard" ? "/login" : `/login?callbackURL=${encodeURIComponent(callbackURL)}`;

  const handleConfirm = async () => {
    if (!token) {
      setErrorMessage("リンクが無効です。確認メールを送り直してください。");
      return;
    }
    setState("loading");
    setErrorMessage(null);
    const { error } = await authClient.verifyEmail({ token });
    if (error) {
      setErrorMessage(error.message);
      setState("ready");
      return;
    }
    setState("done");
  };

  if (state === "done") {
    return (
      <PageShell className="max-w-md">
        <PageHeader title="メールアドレスを確認しました" description="登録が完了しました。ログインして始めましょう。" />
        <Button asChild size="lg" className="h-11 w-full">
          <Link to={loginHref}>ログインする</Link>
        </Button>
      </PageShell>
    );
  }

  return (
    <PageShell className="max-w-md">
      <PageHeader title="メールアドレスの確認" description="下のボタンを押すと、メールアドレスの確認が完了します。" />
      {errorMessage ? (
        <InlineFeedback variant="error" className="mb-4">
          <p>{errorMessage}</p>
          <Link to="/verify-email" className="mt-2 inline-flex min-h-11 items-center text-primary hover:underline">
            確認メールを送り直す
          </Link>
        </InlineFeedback>
      ) : null}
      <Button size="lg" className="h-11 w-full" disabled={state === "loading"} onClick={() => void handleConfirm()}>
        {state === "loading" ? "確認中…" : "メールアドレスを確認する"}
      </Button>
    </PageShell>
  );
}
