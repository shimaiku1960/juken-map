"use client";

import { useState } from "react";
import Link from "next/link";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { PasswordInput } from "@/components/ui/password-input";
import { toast } from "sonner";
import { Label } from "@/components/ui/label";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";
import { useIsLineInAppBrowser, useSafeCallbackURL } from "@/app/hooks/useBrowserNavigation";
import { isLineInAppBrowser } from "@/lib/browser";

export default function SignUpPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [socialLoading, setSocialLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const isLineBrowser = useIsLineInAppBrowser();
  const callbackURL = useSafeCallbackURL("/dashboard");
  const authLoading = loading || socialLoading;

  const handleSignUp = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.signUp.email({
      email,
      password,
      name: email,
      callbackURL,
    });
    if (error) {
      setErrorMessage(error.message ?? "登録に失敗しました");
      setLoading(false);
      return;
    }
    sessionStorage.setItem("pendingVerificationEmail", email);
    toast.success("確認メールを送信しました");
    window.location.href = callbackURL === "/dashboard"
      ? "/verify-email"
      : `/verify-email?callbackURL=${encodeURIComponent(callbackURL)}`;
  };

  // OAuth はサインイン＝サインアップ兼用。アカウントが無ければここで作成される。
  const handleSocialSignUp = async (provider: "google" | "github") => {
    if (provider === "google" && isLineInAppBrowser(window.navigator.userAgent)) {
      return;
    }
    setSocialLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.signIn.social({ provider, callbackURL });
    if (error) {
      setErrorMessage(error.message ?? "外部サービスでの登録に失敗しました");
      setSocialLoading(false);
    }
  };

  return (
    <PageShell className="max-w-md">
      <PageHeader title="新規登録" description="学習予定と記録を、自分のアカウントで管理しましょう。" />
      {errorMessage ? <InlineFeedback variant="error" className="mb-4">{errorMessage}</InlineFeedback> : null}
      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleSignUp(); }}>
        <div className="space-y-2">
          <Label htmlFor="signup-email">メールアドレス</Label>
          <Input id="signup-email" name="email" type="email" autoComplete="email" placeholder="name@example.com" required value={email} onChange={(e) => setEmail(e.target.value)} className="h-11" />
        </div>
        <div className="space-y-2">
          <Label htmlFor="signup-password">パスワード</Label>
          <PasswordInput id="signup-password" name="password" autoComplete="new-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <Button type="submit" size="lg" className="h-11 w-full" disabled={authLoading}>
          {loading ? "登録中…" : "新規登録"}
        </Button>
      </form>

      <Link
        href={callbackURL === "/dashboard" ? "/login" : `/login?callbackURL=${encodeURIComponent(callbackURL)}`}
        className="mt-2 flex min-h-11 items-center justify-center text-sm text-primary hover:underline"
      >
        すでにアカウントをお持ちの方はこちら
      </Link>

      <div className="my-6 flex items-center gap-3 text-xs text-muted-foreground before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">または</div>
      {isLineBrowser ? (
        <InlineFeedback variant="warning" className="mb-4">
          <p className="font-medium">Google登録はLINE内ブラウザでは利用できません</p>
          <p className="mt-1">
            LINEのブラウザメニューから「デフォルトのブラウザで開く」などの項目を選び、SafariまたはChromeで続けてください。
          </p>
        </InlineFeedback>
      ) : null}
      {/* 上の登録ボタンと同じ h-11 に揃える（既定の h-8 はスマホで押しにくい）。 */}
      <Button
        className="h-11 w-full"
        size="lg"
        variant="outline"
        disabled={authLoading || isLineBrowser}
        onClick={() => void handleSocialSignUp("google")}
      >
        {isLineBrowser ? "Google登録は外部ブラウザで利用" : "Googleで登録"}
      </Button>
      <Button
        className="mt-3 h-11 w-full"
        size="lg"
        variant="outline"
        disabled={authLoading}
        onClick={() => void handleSocialSignUp("github")}
      >
        GitHubで登録
      </Button>
      <p className="mt-4 text-center text-xs leading-5 text-muted-foreground">
        いずれかの方法で登録すると、
        <Link href="/terms" className="text-primary hover:underline">
          利用規約
        </Link>
        と
        <Link href="/privacy" className="text-primary hover:underline">
          プライバシーポリシー
        </Link>
        に同意したものとします。
      </p>
    </PageShell>
  );
}
