import { useState } from "react";
import { Sparkles } from "lucide-react";
import { Button } from "@/web/components/ui/button";
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";
import {
  useParseStudyLog,
  type StudyLogParseResult,
} from "@/web/hooks/useStudyLogs";
import { ApiError } from "@/web/lib/api-client";

// サーバーの上限（apps/api の parseTextMax）と同じ。超えると 400 になるので、入力の時点で止める。
const TEXT_MAX = 200;

type Notice = { kind: "status" | "alert"; text: string } | null;

/**
 * 「昨日 数学 青チャート 1時間」のような一文を AI で読み取り、記録のフォームを埋める入力欄（JUK-180）。
 *
 * 読み取るだけで保存はしない。結果をフォームへ入れたあと、利用者が確かめてから記録する。
 * AI は読み違えることがあるので、確かめる手間を省かない。
 *
 * 記録のフォーム（<form>）の中に置くので、ここでは <form> を使わない（入れ子にできない）。
 * そのため Enter キーは自分で受け、フォームの送信（＝記録）へ流れないよう止める。
 */
export default function QuickLogSentence({
  onParsed,
}: {
  /** 読み取れた項目が1つ以上あったとき、フォームへ入れるために呼ぶ */
  onParsed: (result: StudyLogParseResult) => void;
}) {
  const parse = useParseStudyLog();
  const [text, setText] = useState("");
  const [notice, setNotice] = useState<Notice>(null);
  // 鍵の無い環境（503）では、何度押しても同じなので欄ごと使えなくする
  const [unavailable, setUnavailable] = useState(false);

  const blank = text.trim() === "";

  const read = () => {
    if (blank || parse.isPending || unavailable) return;
    setNotice(null);
    parse.mutate(text.trim(), {
      onSuccess: (result) => {
        const found =
          result.date != null ||
          result.minutes != null ||
          result.subject != null ||
          result.textbookId != null;
        if (!found) {
          setNotice({
            kind: "alert",
            text: "読み取れる項目がありませんでした。時間や科目を入れて書いてください。",
          });
          return;
        }
        onParsed(result);
        setNotice({
          kind: "status",
          text: "読み取った内容を入れました。確かめてから記録してください。",
        });
      },
      onError: (error) => {
        if (error instanceof ApiError) {
          if (error.status === 503) setUnavailable(true);
          setNotice({ kind: "alert", text: error.message });
          return;
        }
        setNotice({
          kind: "alert",
          text: "通信できませんでした。接続を確かめてください。",
        });
      },
    });
  };

  return (
    <div className="space-y-2 rounded-lg border border-dashed p-3">
      <Label htmlFor="quick-log-sentence" className="flex items-center gap-1">
        <Sparkles className="size-4" aria-hidden="true" />
        一文で入力（AI）
      </Label>
      <div className="flex gap-2">
        <Input
          id="quick-log-sentence"
          value={text}
          maxLength={TEXT_MAX}
          placeholder="例：昨日 数学 青チャート 1時間半"
          className="h-11 min-w-0 flex-1 text-base"
          disabled={unavailable}
          onChange={(event) => setText(event.target.value)}
          onKeyDown={(event) => {
            // 日本語の変換を確定する Enter では読み取らない
            if (event.key !== "Enter" || event.nativeEvent.isComposing) return;
            event.preventDefault();
            read();
          }}
        />
        <Button
          type="button"
          variant="secondary"
          className="h-11"
          disabled={blank || parse.isPending || unavailable}
          onClick={read}
        >
          {parse.isPending ? "読み取り中…" : "読み取る"}
        </Button>
      </div>
      {notice && (
        <p
          role={notice.kind}
          className={
            notice.kind === "alert"
              ? "text-sm text-destructive"
              : "text-sm text-muted-foreground"
          }
        >
          {notice.text}
        </p>
      )}
    </div>
  );
}
