import { useState } from "react";
import { Button } from "@/web/components/ui/button";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import { useStartLineAccountLink } from "@/web/hooks/useLineLink";

export default function LineAccountLinkButton({
  linkToken,
}: {
  linkToken: string;
}) {
  const [error, setError] = useState<string | null>(null);
  const startLink = useStartLineAccountLink();

  // 成功したら LINE へ遷移する。遷移の間もボタンは「LINEへ移動中…」のままに
  // したいので、isPending だけでなく isSuccess も見る（成功で isPending は
  // false に戻るため、それだけだと表示が一瞬元に戻る）。
  const isLinking = startLink.isPending || startLink.isSuccess;

  const link = () => {
    setError(null);
    startLink.mutate(linkToken, {
      onSuccess: (result) => {
        window.location.href = result.redirectUrl;
      },
      onError: (cause) => setError(cause.message),
    });
  };

  return (
    <div className="space-y-4">
      {error ? <InlineFeedback variant="error">{error}</InlineFeedback> : null}
      <Button
        type="button"
        size="lg"
        className="h-11 w-full"
        disabled={isLinking}
        onClick={() => void link()}
      >
        {isLinking ? "LINEへ移動中…" : "このアカウントと連携する"}
      </Button>
    </div>
  );
}
