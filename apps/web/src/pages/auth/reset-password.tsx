import { useState, Suspense } from "react";
import { useNavigate, useSearchParams } from "react-router";
import { authClient } from "@/web/lib/auth-client";
import { Button } from "@/web/components/ui/button";
import { PasswordInput } from "@/web/components/ui/password-input";
import { Label } from "@/web/components/ui/label";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";

function ResetPasswordForm() {
  const navigate = useNavigate();
  // react-router の useSearchParams は [params, setParams] のタプルを返す
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleSubmit = async () => {
    setErrorMessage(null);
    if (!token) {
      setErrorMessage("リンクが無効です。お手数ですが再度お試しください");
      return;
    }
    setLoading(true);
    const { error } = await authClient.resetPassword({
      newPassword: password,
      token,
    });
    if (error) {
      setErrorMessage(error.message ?? "再設定に失敗しました");
      setLoading(false);
      return;
    }
    navigate("/login");
  };

  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleSubmit(); }}>
      {errorMessage ? <InlineFeedback variant="error">{errorMessage}</InlineFeedback> : null}
      <div className="space-y-2">
        <Label htmlFor="new-password">新しいパスワード</Label>
        <PasswordInput id="new-password" name="password" autoComplete="new-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
      </div>
      <Button type="submit" size="lg" className="h-11 w-full" disabled={loading}>{loading ? "再設定中…" : "パスワードを再設定する"}</Button>
    </form>
  );
}

export default function ResetPasswordPage() {
  return (
    <PageShell className="max-w-md">
      <PageHeader title="新しいパスワードの設定" description="新しく使用するパスワードを入力してください。" />
      <Suspense>
        <ResetPasswordForm />
      </Suspense>
    </PageShell>
  );
}
