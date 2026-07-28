"use client";

import { useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { authClient } from "@/lib/auth-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";

const subscribeToPendingEmail = () => () => {};
const getPendingEmail = () =>
  sessionStorage.getItem("pendingVerificationEmail") ?? "";
const getPendingEmailOnServer = () => "";

export default function VerifyEmailPage() {
  const pendingEmail = useSyncExternalStore(
    subscribeToPendingEmail,
    getPendingEmail,
    getPendingEmailOnServer
  );
  const [email, setEmail] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const verificationEmail = email ?? pendingEmail;

  const handleResend = async () => {
    setLoading(true);
    setMessage(null);
    setErrorMessage(null);
    const { error } = await authClient.sendVerificationEmail({
      email: verificationEmail,
      callbackURL: "/dashboard",
    });
    if (error) {
      setErrorMessage(error.message ?? "確認メールの再送に失敗しました");
      setLoading(false);
      return;
    }
    sessionStorage.setItem("pendingVerificationEmail", verificationEmail);
    setMessage("確認メールを再送しました。");
    setLoading(false);
  };

  return (
    <PageShell className="max-w-md">
      <PageHeader
        title="メールをご確認ください"
        description="確認メール内のリンクを開くと登録が完了し、ダッシュボードへ移動します。"
      />

      <InlineFeedback variant="info" className="mb-4">
        メールが届かない場合は、迷惑メールフォルダをご確認ください。
      </InlineFeedback>

      {message ? (
        <InlineFeedback variant="success" className="mb-4">
          {message}
        </InlineFeedback>
      ) : null}
      {errorMessage ? (
        <InlineFeedback variant="error" className="mb-4">
          {errorMessage}
        </InlineFeedback>
      ) : null}

      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          void handleResend();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="verification-email">メールアドレス</Label>
          <Input
            id="verification-email"
            name="email"
            type="email"
            autoComplete="email"
            placeholder="name@example.com"
            required
            value={verificationEmail}
            onChange={(event) => setEmail(event.target.value)}
            className="h-11"
          />
        </div>
        <Button
          type="submit"
          size="lg"
          className="h-11 w-full"
          disabled={loading}
        >
          {loading ? "再送中…" : "確認メールを再送"}
        </Button>
      </form>

      <Link
        href="/login"
        className="mt-4 block text-center text-sm text-primary hover:underline"
      >
        確認済みの方はログインへ
      </Link>
    </PageShell>
  );
}
