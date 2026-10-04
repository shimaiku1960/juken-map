import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { authClient } from "@/web/lib/auth-client";
import { Button } from "@/web/components/ui/button";
import { PasswordInput } from "@/web/components/ui/password-input";
import { Label } from "@/web/components/ui/label";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import PasswordHint from "@/web/components/auth/PasswordHint";
import { useTokenFromLink } from "@/web/hooks/useTokenFromLink";

// 再設定のメールのリンクで開く画面。トークンを使うのは、新しいパスワードを送る POST（認証基準 10 の E2）。
// 再設定が済んでも自動ではログインさせない（E3）。ほかの端末のログインもすべて解除される。
function ResetPasswordForm() {
  const navigate = useNavigate();
  const token = useTokenFromLink();
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
    const { error } = await authClient.resetPassword({ token, password });
    if (error) {
      setErrorMessage(error.message);
      setLoading(false);
      return;
    }
    toast.success("パスワードを再設定しました。新しいパスワードでログインしてください");
    navigate("/login");
  };

  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleSubmit(); }}>
      {errorMessage ? <InlineFeedback variant="error">{errorMessage}</InlineFeedback> : null}
      <div className="space-y-2">
        <Label htmlFor="new-password">新しいパスワード</Label>
        <PasswordInput id="new-password" name="password" autoComplete="new-password" required minLength={15} aria-describedby="new-password-hint" value={password} onChange={(e) => setPassword(e.target.value)} />
        <PasswordHint id="new-password-hint" />
      </div>
      <Button type="submit" size="lg" className="h-11 w-full" disabled={loading}>{loading ? "再設定中…" : "パスワードを再設定する"}</Button>
    </form>
  );
}

export default function ResetPasswordPage() {
  return (
    <PageShell className="max-w-md">
      <PageHeader title="新しいパスワードの設定" description="新しく使用するパスワードを入力してください。" />
      <ResetPasswordForm />
    </PageShell>
  );
}
