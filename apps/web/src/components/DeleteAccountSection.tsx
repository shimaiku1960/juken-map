import { useState } from "react";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import { Button } from "@/web/components/ui/button";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";
import { PasswordInput } from "@/web/components/ui/password-input";
import { authClient } from "@/web/lib/auth-client";
import { notifyDemoReadOnly } from "@/web/lib/demo-client";
import { useHasPassword } from "@/web/hooks/useTwoFactor";

// プロフィールの「退会」（06 G3、JUK-123）。取り消せないので、最初はボタンだけを出し、押したら
// 本人の確認の欄を開く。確認の中身は API（apps/api/internal/feature/auth/delete_account.go）と同じで、
// パスワードがあればパスワード、無ければメールアドレスの打ち込み、2段階認証が有効ならそのコードも。

type Props = {
  email: string;
  twoFactorEnabled: boolean;
  isAdmin: boolean;
  readOnly: boolean;
};

export default function DeleteAccountSection({ email, twoFactorEnabled, isAdmin, readOnly }: Props) {
  const [open, setOpen] = useState(false);

  if (isAdmin) {
    return (
      <p className="text-sm text-muted-foreground">
        管理者のアカウントは退会できません。先に管理者の権限を外してください。
      </p>
    );
  }

  return (
    <div className="space-y-4 text-sm">
      <p>
        アカウントと、学習記録・学習予定・参考書・志望校・通知の設定・LINE の連携をすべて削除します。
        <strong className="font-semibold">削除したデータは元に戻せません。</strong>
      </p>
      {open ? (
        <DeleteAccountForm email={email} twoFactorEnabled={twoFactorEnabled} onCancel={() => setOpen(false)} />
      ) : (
        <Button
          type="button"
          variant="destructive"
          size="lg"
          className="h-11"
          onClick={() => {
            if (readOnly) {
              notifyDemoReadOnly();
              return;
            }
            setOpen(true);
          }}
        >
          退会する
        </Button>
      )}
    </div>
  );
}

function DeleteAccountForm({
  email,
  twoFactorEnabled,
  onCancel,
}: {
  email: string;
  twoFactorEnabled: boolean;
  onCancel: () => void;
}) {
  const hasPassword = useHasPassword();
  const [password, setPassword] = useState("");
  const [typedEmail, setTypedEmail] = useState("");
  const [code, setCode] = useState("");
  const [useBackup, setUseBackup] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  if (hasPassword.isPending) {
    return <p className="text-muted-foreground">読み込み中…</p>;
  }
  if (hasPassword.error) {
    return <p className="text-destructive">{hasPassword.error.message}</p>;
  }

  const handleSubmit = async () => {
    setLoading(true);
    setErrorMessage(null);
    const { error } = await authClient.deleteAccount({
      ...(hasPassword.data ? { password } : { email: typedEmail }),
      ...(twoFactorEnabled ? { code: code.trim(), method: useBackup ? "backup" : "totp" } : {}),
    });
    if (error) {
      setErrorMessage(error.message);
      setLoading(false);
      return;
    }
    // セッションの Cookie は API が消している。ページを読み直して、ログイン前のトップへ戻す。
    window.location.assign("/?deleted=1");
  };

  return (
    <form
      className="space-y-4"
      onSubmit={(event) => {
        event.preventDefault();
        void handleSubmit();
      }}
    >
      {errorMessage ? (
        <InlineFeedback variant="error">
          <p>{errorMessage}</p>
        </InlineFeedback>
      ) : null}
      {hasPassword.data ? (
        <div className="space-y-2">
          <Label htmlFor="delete-account-password">確認のため、パスワードを入力してください</Label>
          <PasswordInput
            id="delete-account-password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
          />
        </div>
      ) : (
        <div className="space-y-2">
          <Label htmlFor="delete-account-email">
            確認のため、メールアドレス（{email}）を入力してください
          </Label>
          <Input
            id="delete-account-email"
            type="email"
            autoComplete="off"
            required
            value={typedEmail}
            onChange={(event) => setTypedEmail(event.target.value)}
            className="h-11"
          />
        </div>
      )}
      {twoFactorEnabled ? (
        <div className="space-y-2">
          <Label htmlFor="delete-account-code">
            {useBackup ? "予備コード" : "認証アプリの6桁のコード"}
          </Label>
          <Input
            id="delete-account-code"
            autoComplete="one-time-code"
            inputMode={useBackup ? "text" : "numeric"}
            required
            value={code}
            onChange={(event) => setCode(event.target.value)}
            className="h-11 max-w-60"
          />
          <Button
            type="button"
            variant="link"
            className="h-auto px-0"
            onClick={() => {
              setUseBackup((current) => !current);
              setCode("");
            }}
          >
            {useBackup ? "認証アプリのコードを使う" : "予備コードを使う"}
          </Button>
        </div>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="destructive" size="lg" className="h-11" disabled={loading}>
          {loading ? "削除中…" : "すべて削除して退会する"}
        </Button>
        <Button type="button" variant="outline" size="lg" className="h-11" disabled={loading} onClick={onCancel}>
          やめる
        </Button>
      </div>
    </form>
  );
}
