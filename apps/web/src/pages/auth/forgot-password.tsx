import { useState } from "react";
import { authClient } from "@/web/lib/auth-client";
import { Button } from "@/web/components/ui/button";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleSubmit = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.forgotPassword({ email });
    if (error) {
      setErrorMessage(error.message);
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
          登録されているメールアドレスなら、再設定用のリンクを送りました（有効期限は1時間）。メールをご確認ください。
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
