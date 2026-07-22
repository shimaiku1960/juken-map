"use client";

import { useState } from "react";
import Link from "next/link";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { toast } from "sonner";
import { Label } from "@/components/ui/label";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";

export default function SignUpPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleSignUp = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.signUp.email({
      email,
      password,
      name: email,
    });
    if (error) {
      setErrorMessage(error.message ?? "登録に失敗しました");
      setLoading(false);
      return;
    }
    toast.success("登録しました");
    window.location.href = "/";
  };

  return (
    <PageShell className="max-w-md">
      <PageHeader title="新規登録" description="学習予定と記録を、自分のアカウントで管理しましょう。" />
      {errorMessage ? <InlineFeedback variant="error" className="mb-4">{errorMessage}</InlineFeedback> : null}
      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleSignUp(); }}>
        <div className="space-y-2">
          <Label htmlFor="signup-email">メールアドレス</Label>
          <Input id="signup-email" name="email" type="email" autoComplete="email" placeholder="name@example.com" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="signup-password">パスワード</Label>
          <Input id="signup-password" name="password" type="password" autoComplete="new-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <Button type="submit" size="lg" className="h-11 w-full" disabled={loading}>
          {loading ? "登録中…" : "新規登録"}
        </Button>
      </form>

      <Link
        href="/login"
        className="mt-6 block text-center text-sm text-primary hover:underline"
      >
        すでにアカウントをお持ちの方はこちら
      </Link>
    </PageShell>
  );
}
