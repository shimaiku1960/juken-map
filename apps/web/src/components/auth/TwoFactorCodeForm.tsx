import { useState } from "react";
import { authClient } from "@/web/lib/auth-client";
import { Button } from "@/web/components/ui/button";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";

// ログインでメール＋パスワードが通ったあと、2段階認証を有効にしている人に認証コードを求める。
// 認証アプリの6桁のコードか、スマホを無くしたとき用の予備コード（1回ずつ使い捨て）で通れる。
// 通るとセッションが作られるので、呼び出し側が行き先へ移る。

type Props = {
  onVerified: () => void;
  onCancel: () => void;
};

export default function TwoFactorCodeForm({ onVerified, onCancel }: Props) {
  const [code, setCode] = useState("");
  const [useBackupCode, setUseBackupCode] = useState(false);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const handleVerify = async () => {
    setLoading(true);
    setErrorMessage(null);
    const trimmed = code.trim();
    const { error } = useBackupCode
      ? await authClient.twoFactor.verifyBackupCode({ code: trimmed })
      : await authClient.twoFactor.verifyTotp({ code: trimmed });
    if (error) {
      setErrorMessage(error.message ?? "コードを確認できませんでした");
      setLoading(false);
      return;
    }
    onVerified();
  };

  const toggleBackupCode = () => {
    setUseBackupCode((current) => !current);
    setCode("");
    setErrorMessage(null);
  };

  return (
    <div className="space-y-4">
      {errorMessage ? (
        <InlineFeedback variant="error">
          <p>{errorMessage}</p>
        </InlineFeedback>
      ) : null}

      <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void handleVerify(); }}>
        <div className="space-y-2">
          <Label htmlFor="two-factor-code">{useBackupCode ? "予備コード" : "認証アプリの6桁のコード"}</Label>
          <Input
            id="two-factor-code"
            name="code"
            autoComplete="one-time-code"
            inputMode={useBackupCode ? "text" : "numeric"}
            required
            autoFocus
            value={code}
            onChange={(event) => setCode(event.target.value)}
            className="h-11"
          />
        </div>
        <Button type="submit" size="lg" className="h-11 w-full" disabled={loading}>
          {loading ? "確認中…" : "確認してログイン"}
        </Button>
      </form>

      <div className="flex flex-col items-center gap-1 text-sm">
        <button type="button" className="min-h-11 text-primary hover:underline" onClick={toggleBackupCode}>
          {useBackupCode ? "認証アプリのコードを使う" : "予備コードを使う"}
        </button>
        <button type="button" className="min-h-11 text-muted-foreground hover:underline" onClick={onCancel}>
          ログインをやり直す
        </button>
      </div>
    </div>
  );
}
