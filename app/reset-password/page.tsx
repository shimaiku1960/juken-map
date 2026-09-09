"use client";

import { useState, Suspense } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { authClient } from "@/frontend/lib/auth-client";
import { Button } from "@/frontend/components/ui/button";
import { PasswordInput } from "@/frontend/components/ui/password-input";
import { Label } from "@/frontend/components/ui/label";
import InlineFeedback from "@/frontend/components/feedback/InlineFeedback";
import PageShell from "@/frontend/components/layout/PageShell";
import PageHeader from "@/frontend/components/layout/PageHeader";

function ResetPasswordForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
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
    router.push("/login");
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
