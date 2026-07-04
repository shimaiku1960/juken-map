"use client";

import { useState } from "react";
import Link from "next/link";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { toast } from "sonner";

export default function SignUpPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const handleSignUp = async () => {
    const { error } = await authClient.signUp.email({
      email,
      password,
      name: email,
    });
    if (error) {
      toast.error(error.message ?? "登録に失敗しました");
      return;
    }
    toast.success("登録しました");
    window.location.href = "/";
  };

  return (
    <main className="w-full mx-auto max-w-md p-8">
      <h1 className="text-3xl font-bold mb-6">新規登録</h1>

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

      <Button className="w-full mb-6" onClick={handleSignUp}>
        新規登録
      </Button>

      <Link
        href="/login"
        className="block text-sm text-muted-foreground underline text-center"
      >
        すでにアカウントをお持ちの方はこちら
      </Link>
    </main>
  );
}
