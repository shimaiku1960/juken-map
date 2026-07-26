"use client";

import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import Link from "next/link";
import { Label } from "@/components/ui/label";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [demoLoading, setDemoLoading] = useState(false);
  const [signInLoading, setSignInLoading] = useState(false);
  const [socialLoading, setSocialLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const authLoading = demoLoading || signInLoading || socialLoading;

  const handleSocialSignIn = async (provider: "google" | "github") => {
    setSocialLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.signIn.social({ provider, callbackURL: "/" });
    if (error) {
      setErrorMessage(error.message ?? "外部サービスでのログインに失敗しました");
      setSocialLoading(false);
    }
  };

  const handleSignIn = async () => {
    setSignInLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.signIn.email({
      email,
      password,
    });
    if (error) {
      setErrorMessage(error.message ?? "ログインに失敗しました");
      setSignInLoading(false);
      return;
    }
    window.location.href = "/";
  };

  // 面接官などがアカウント登録なしで中身を体験できる共有デモアカウント
  const handleDemoSignIn = async () => {
    setDemoLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.signIn.email({
      email: "demo@juken-map.com",
      password: "demodemo1234",
    });
    if (error) {
      setErrorMessage(error.message ?? "デモログインに失敗しました");
      setDemoLoading(false);
      return;
    }
    window.location.href = "/";
  };

  return (
    <PageShell className="max-w-md">
      <PageHeader title="ログイン" description="アカウント情報を入力してください。" />

      {errorMessage ? <InlineFeedback variant="error" className="mb-4">{errorMessage}</InlineFeedback> : null}

      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleSignIn(); }}>
        <div className="space-y-2">
          <Label htmlFor="login-email">メールアドレス</Label>
          <Input id="login-email" name="email" type="email" autoComplete="email" placeholder="name@example.com" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="space-y-2">
          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="login-password">パスワード</Label>
            <Link href="/forgot-password" className="text-sm text-primary hover:underline">パスワードを忘れた方</Link>
          </div>
          <Input id="login-password" name="password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <Button type="submit" size="lg" className="h-11 w-full" disabled={authLoading}>
          {signInLoading ? "ログイン中…" : "ログイン"}
        </Button>
      </form>

      <Link
        href="/signup"
        className="mt-4 block text-center text-sm text-primary hover:underline"
      >
        アカウントをお持ちでない方はこちら
      </Link>

      <div className="my-6 flex items-center gap-3 text-xs text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">または</div>
      <Button
        className="w-full"
        variant="outline"
        disabled={authLoading}
        onClick={() => void handleSocialSignIn("google")}
      >
        Googleでログイン
      </Button>
      <Button
        className="w-full mt-3"
        variant="outline"
        disabled={authLoading}
        onClick={() => void handleSocialSignIn("github")}
      >
        GitHubでログイン
      </Button>

      <div
        id="demo-login"
        className="mt-6 scroll-mt-24 rounded-xl border border-dashed border-primary/35 bg-primary/5 p-4"
      >
        <p className="font-medium">登録せずにデモを見る</p>
        <p className="mt-1 text-sm text-muted-foreground">志望校や学習予定が入ったデモ用アカウントを体験できます。</p>
        <Button className="mt-4 w-full" variant="secondary" onClick={handleDemoSignIn} disabled={authLoading}>
          {demoLoading ? "ログイン中…" : "デモを見る"}
        </Button>
      </div>
    </PageShell>
  );
}
