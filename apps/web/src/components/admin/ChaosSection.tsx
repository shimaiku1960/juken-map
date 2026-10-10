import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/web/components/ui/badge";
import { Button } from "@/web/components/ui/button";
import { Card } from "@/web/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/web/components/ui/dialog";
import { useAdminChaos, useStopChaos, type ChaosExperiment, type ChaosRecordKind } from "@/web/hooks/useAdmin";

// 管理者ページの「障害注入」（JUK-178）。予告なしの障害で原因を調べる練習をするので、ここは答え合わせの場所。
// 実行中の実験は API が返さないので出さない。止めるボタンは練習をやめたいときの緊急停止で、正解の直し方ではない
// （練習では本番と同じく CHAOS_ENABLED を消して再起動する。docs/incident-response.md）。

// host_* は予告なしのくじだけが EC2 の上で起こすホストの層の障害（JUK-174）。
const KIND_LABELS: Record<ChaosRecordKind, string> = {
  latency: "遅延",
  http_error: "5xx",
  db_error: "DB の失敗",
  outbound_timeout: "外部 API のタイムアウト",
  host_process_kill: "API のプロセスの kill",
  host_cpu: "CPU の圧迫",
  host_memory: "メモリの圧迫",
  host_disk: "ディスクを埋める",
  host_db_delay: "RDS までの通信の遅延",
  host_db_loss: "RDS までの通信の喪失",
};

const dateTimeFormat = new Intl.DateTimeFormat("ja-JP", {
  timeZone: "Asia/Tokyo",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
});

const formatDateTime = (iso: string) => dateTimeFormat.format(new Date(iso));

const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : "読み込みに失敗しました";

const detail = (e: ChaosExperiment) =>
  e.kind === "http_error" ? `${e.statusCode} を返す` : e.delayMs > 0 ? `${e.delayMs.toLocaleString()}ms` : "—";

// stoppedBy は admin:<userId>・job（機械の入口）・deploy（デプロイの前後、JUK-176）・failed（ホストで起こせなかった、JUK-174）。
const stoppedByLabel = (stoppedBy: string) =>
  stoppedBy === "failed"
    ? "ホストで起こせず停止"
    : `${stoppedBy === "deploy" ? "デプロイ" : stoppedBy === "job" ? "機械の入口" : "管理画面"}が停止`;

const routeLabel = (route: string) => (route === "*" ? "すべて" : route === "" ? "ホスト" : route);

export default function ChaosSection() {
  const chaos = useAdminChaos();
  const [confirming, setConfirming] = useState(false);

  if (!chaos.data) {
    return chaos.error ? (
      <p className="text-sm text-destructive">{errorMessage(chaos.error)}</p>
    ) : (
      <p className="text-sm text-muted-foreground">読み込み中…</p>
    );
  }

  const { enabled, experiments } = chaos.data;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {enabled ? (
            <>
              <Badge variant="secondary" className="mr-2">
                有効
              </Badge>
              終わった実験だけを表示します。
            </>
          ) : (
            <>
              <Badge variant="outline" className="mr-2">
                無効
              </Badge>
              CHAOS_ENABLED=on で起動していないので、障害は起きません。
            </>
          )}
        </p>
        {enabled ? (
          <Button type="button" variant="outline" size="sm" onClick={() => setConfirming(true)}>
            緊急停止
          </Button>
        ) : null}
      </div>

      {experiments.length === 0 ? (
        <p className="text-sm text-muted-foreground">終わった実験はまだありません。</p>
      ) : (
        <Card className="py-0">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[48rem] text-sm">
              <thead className="border-b text-left text-xs text-muted-foreground">
                <tr>
                  <th className="px-4 py-2 font-medium">始めた</th>
                  <th className="px-4 py-2 font-medium">終わった</th>
                  <th className="px-4 py-2 font-medium">種類</th>
                  <th className="px-4 py-2 font-medium">ルート</th>
                  <th className="px-4 py-2 text-right font-medium">割合</th>
                  <th className="px-4 py-2 font-medium">中身</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {experiments.map((e) => (
                  <tr key={e.id}>
                    <td className="whitespace-nowrap px-4 py-2 tabular-nums">{formatDateTime(e.startsAt)}</td>
                    <td className="whitespace-nowrap px-4 py-2 tabular-nums">
                      {formatDateTime(e.stoppedAt ?? e.endsAt)}
                      {e.stoppedBy ? (
                        <span className="ml-1 text-xs text-muted-foreground">
                          （{stoppedByLabel(e.stoppedBy)}）
                        </span>
                      ) : null}
                    </td>
                    <td className="whitespace-nowrap px-4 py-2">{KIND_LABELS[e.kind]}</td>
                    <td className="px-4 py-2 font-mono text-xs">{routeLabel(e.route)}</td>
                    <td className="px-4 py-2 text-right tabular-nums">{Math.round(e.rate * 100)}%</td>
                    <td className="whitespace-nowrap px-4 py-2 tabular-nums">{detail(e)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {confirming ? <StopDialog onClose={() => setConfirming(false)} /> : null}
    </div>
  );
}

function StopDialog({ onClose }: { onClose: () => void }) {
  const stop = useStopChaos();

  const handleStop = () => {
    stop.mutate(undefined, {
      onSuccess: () => {
        toast.success("止めました（実行中の実験があれば）");
        onClose();
      },
      onError: (error) => toast.error(errorMessage(error)),
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>障害注入を止める</DialogTitle>
          <DialogDescription>
            実行中の実験があれば止めます。練習の答えにならないよう、実験があったかどうかは表示しません。
            練習として直すときは、本番と同じく CHAOS_ENABLED を消して再起動してください。
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            やめる
          </Button>
          <Button type="button" variant="destructive" disabled={stop.isPending} onClick={handleStop}>
            {stop.isPending ? "止めています…" : "止める"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
