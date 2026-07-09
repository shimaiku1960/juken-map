"use client";

import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { toast } from "sonner";
import Link from "next/link";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [demoLoading, setDemoLoading] = useState(false);

  const handleSignIn = async () => {
    const { error } = await authClient.signIn.email({
      email,
      password,
    });
    if (error) {
      toast.error(error.message ?? "ログインに失敗しました");
      return;
    }
    window.location.href = "/";
  };

  // 面接官などがアカウント登録なしで中身を体験できる共有デモアカウント
  const handleDemoSignIn = async () => {
    setDemoLoading(true);
    const { error } = await authClient.signIn.email({
      email: "demo@juken-map.com",
      password: "demodemo1234",
    });
    if (error) {
      toast.error(error.message ?? "デモログインに失敗しました");
      setDemoLoading(false);
      return;
    }
    window.location.href = "/";
  };

  return (
    <main className="w-full mx-auto max-w-md p-8">
      <h1 className="text-3xl font-bold mb-6">ログイン</h1>

      <div className="mb-6 rounded-lg border border-dashed border-primary/40 bg-primary/5 p-4">
        <p className="mb-3 text-sm text-muted-foreground">
          登録せずにすぐ試せます。志望校や学習予定が入ったデモ用アカウントでログインします。
        </p>
        <Button
          className="w-full"
          onClick={handleDemoSignIn}
          disabled={demoLoading}
        >
          {demoLoading ? "ログイン中..." : "デモでログイン（登録不要）"}
        </Button>
      </div>

      <div className="flex flex-col gap-3 mb-3">
        <Input
          type="email"
          placeholder="メールアドレス"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <Input
          type="password"
          placeholder="パスワード"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </div>

      <Button className="w-full mb-6" onClick={handleSignIn}>
        ログイン
      </Button>

      <Link
        href="/signup"
        className="block text-sm text-muted-foreground underline mb-3 text-center"
      >
        アカウントをお持ちでない方はこちら
      </Link>

      <Link
        href="/forgot-password"
        className="block text-sm text-muted-foreground underline mb-6 text-center"
      >
        パスワードを忘れた方はこちら
      </Link>

      <Button
        className="w-full"
        variant="secondary"
        onClick={() =>
          authClient.signIn.social({ provider: "google", callbackURL: "/" })
        }
      >
        Googleでログイン
      </Button>
      <Button
        className="w-full mt-3"
        variant="secondary"
        onClick={() =>
          authClient.signIn.social({ provider: "github", callbackURL: "/" })
        }
      >
        GitHubでログイン
      </Button>
    </main>
  );
}
