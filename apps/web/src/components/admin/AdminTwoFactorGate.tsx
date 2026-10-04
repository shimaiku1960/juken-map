import { useState, type ReactNode } from "react";
import { QRCodeSVG } from "qrcode.react";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import { Button } from "@/web/components/ui/button";
import { Card, CardContent } from "@/web/components/ui/card";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";
import { PasswordInput } from "@/web/components/ui/password-input";
import { authClient, useSession } from "@/web/lib/auth-client";
import { useHasPassword } from "@/web/hooks/useTwoFactor";

// 管理画面の入口。管理 API は、2段階認証を通して作られたセッションでないと 403 を返す
// （apps/api-go の router.go の admin）。守るのは API で、ここはその手前で
// 「何をすれば入れるか」を案内するだけ。管理者でない人はそのまま中身を出し、API の 403 で
// 「権限がありません」になる。
//
// 案内は3通り。
//   - 2段階認証が有効：このセッションは通していない（有効にする前からのログインなど）→ ログインし直し
//   - 未設定でパスワードが無い：有効にするにはパスワードが要る → 「パスワードを忘れた方」から作る
//   - 未設定でパスワードがある：ここで設定する（QR を読み、コードを確かめる）

export default function AdminTwoFactorGate({ children }: { children: ReactNode }) {
  const { data: session } = useSession();

  if (!session || session.user.role !== "admin" || session.session.twoFactorVerified) {
    return children;
  }

  return (
    <PageShell className="max-w-xl">
      <PageHeader title="管理" description="管理画面を開くには、2段階認証が必要です。" />
      {session.user.twoFactorEnabled ? (
        <SignInAgain />
      ) : (
        <SetupOrPassword email={session.user.email} />
      )}
    </PageShell>
  );
}

async function signOutTo(path: string) {
  await authClient.signOut();
  window.location.href = path;
}

function SignInAgain() {
  return (
    <Card>
      <CardContent className="space-y-4 py-6 text-sm">
        <p>このログインは2段階認証を通していません（2段階認証を有効にする前からのログインなど）。</p>
        <p>ログアウトしてログインし直し、認証アプリのコードを入力してください。</p>
        <Button onClick={() => void signOutTo("/login?callbackURL=%2Fadmin")}>ログアウトしてログインし直す</Button>
      </CardContent>
    </Card>
  );
}

function SetupOrPassword({ email }: { email: string }) {
  const hasPassword = useHasPassword();

  if (hasPassword.isPending) {
    return <p className="text-sm text-muted-foreground">読み込み中…</p>;
  }
  if (hasPassword.error) {
    return <p className="text-sm text-destructive">{hasPassword.error.message}</p>;
  }
  if (!hasPassword.data) {
    return (
      <Card>
        <CardContent className="space-y-4 py-6 text-sm">
          <p>2段階認証を設定するには、先にこのアカウントのパスワードが必要です（今は Google・GitHub でだけログインしています）。</p>
          <ol className="list-decimal space-y-1 pl-5">
            <li>ログアウトし、ログイン画面の「パスワードを忘れた方」から {email} に再設定のメールを送る</li>
            <li>メールのリンクからパスワードを決める</li>
            <li>メールアドレスとパスワードでログインし、もう一度この画面を開く</li>
          </ol>
          <Button onClick={() => void signOutTo("/forgot-password")}>ログアウトしてパスワードを設定する</Button>
        </CardContent>
      </Card>
    );
  }
  return <TwoFactorSetup />;
}

function TwoFactorSetup() {
  const [password, setPassword] = useState("");
  const [enrollment, setEnrollment] = useState<{ totpURI: string; backupCodes: string[] } | null>(null);
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleEnable = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { data, error } = await authClient.mfa.setup({ password });
    setLoading(false);
    if (error) {
      setErrorMessage(error.message);
      return;
    }
    setPassword("");
    setEnrollment(data);
  };

  const handleVerify = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.mfa.confirm({ code: code.trim() });
    if (error) {
      setErrorMessage(error.message);
      setLoading(false);
      return;
    }
    // 確認に通ると、2段階認証を通したセッションに差し替わる。読み直して管理画面を出す。
    window.location.reload();
  };

  const errorFeedback = errorMessage ? (
    <InlineFeedback variant="error">
      <p>{errorMessage}</p>
    </InlineFeedback>
  ) : null;

  if (!enrollment) {
    return (
      <Card>
        <CardContent className="space-y-4 py-6 text-sm">
          <p>
            認証アプリ（Google Authenticator など）で2段階認証を設定します。設定すると、次からのログインで
            パスワードのあとに6桁のコードを入力します。
          </p>
          {errorFeedback}
          <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleEnable(); }}>
            <div className="space-y-2">
              <Label htmlFor="two-factor-password">パスワード</Label>
              <PasswordInput
                id="two-factor-password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </div>
            <Button type="submit" disabled={loading}>{loading ? "準備中…" : "設定を始める"}</Button>
          </form>
        </CardContent>
      </Card>
    );
  }

  const secret = new URL(enrollment.totpURI).searchParams.get("secret");

  return (
    <Card>
      <CardContent className="space-y-6 py-6 text-sm">
        <section className="space-y-3">
          <h2 className="font-medium">1. 認証アプリで QR コードを読み取る</h2>
          {/* 暗い配色でも読み取れるよう、QR の周りは白で固定する */}
          <div className="inline-block rounded-lg bg-white p-3">
            <QRCodeSVG value={enrollment.totpURI} size={176} />
          </div>
          {secret ? (
            <p className="text-muted-foreground">
              読み取れないときは、このキーを手で入力してください：
              <code className="ml-1 break-all rounded bg-muted px-1.5 py-0.5 font-mono text-foreground">{secret}</code>
            </p>
          ) : null}
        </section>

        <section className="space-y-3">
          <h2 className="font-medium">2. 予備コードを保存する</h2>
          <p className="text-muted-foreground">
            スマホを無くしたときに、認証アプリのコードの代わりに1回ずつ使えます。この画面を閉じると二度と表示されないので、
            パスワード管理ツールなどに保存してください。
          </p>
          <ul className="grid grid-cols-2 gap-1 font-mono sm:grid-cols-3">
            {enrollment.backupCodes.map((backupCode) => (
              <li key={backupCode} className="rounded bg-muted px-2 py-1">{backupCode}</li>
            ))}
          </ul>
        </section>

        <section className="space-y-3">
          <h2 className="font-medium">3. 認証アプリの6桁のコードを入力する</h2>
          {errorFeedback}
          <form className="flex gap-2" onSubmit={(event) => { event.preventDefault(); void handleVerify(); }}>
            <Input
              aria-label="認証アプリの6桁のコード"
              autoComplete="one-time-code"
              inputMode="numeric"
              required
              value={code}
              onChange={(event) => setCode(event.target.value)}
              className="h-10 max-w-40"
            />
            <Button type="submit" className="h-10" disabled={loading}>{loading ? "確認中…" : "確認して有効にする"}</Button>
          </form>
        </section>
      </CardContent>
    </Card>
  );
}
