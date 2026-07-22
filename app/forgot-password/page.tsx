"use client";

import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleSubmit = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.requestPasswordReset({
      email,
      redirectTo: "/reset-password",
    });
    if (error) {
      setErrorMessage(error.message ?? "送信に失敗しました");
      setLoading(false);
      return;
    }
    setSent(true);
  };

  return (
    <PageShell className="max-w-md">
      <PageHeader title="パスワードの再設定" description="登録したメールアドレスへ再設定リンクを送ります。" />

      {sent ? (
        <InlineFeedback variant="success">
          入力されたメールアドレス宛に再設定用のリンクを送信しました。メールをご確認ください。
        </InlineFeedback>
      ) : (
        <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleSubmit(); }}>
          {errorMessage ? <InlineFeedback variant="error">{errorMessage}</InlineFeedback> : null}
          <div className="space-y-2">
            <Label htmlFor="reset-email">メールアドレス</Label>
            <Input id="reset-email" name="email" type="email" autoComplete="email" placeholder="name@example.com" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <Button type="submit" size="lg" className="h-11 w-full" disabled={loading}>{loading ? "送信中…" : "再設定リンクを送信"}</Button>
        </form>
      )}
    </PageShell>
  );
}
