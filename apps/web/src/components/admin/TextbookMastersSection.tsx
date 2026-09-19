import { useState, type FormEvent } from "react";
import { toast } from "sonner";
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
import { Input } from "@/web/components/ui/input";
import { Label } from "@/web/components/ui/label";
import {
  useAdminTextbookMasters,
  useDeleteTextbookMaster,
  useSaveTextbookMaster,
  type AdminTextbookMaster,
} from "@/web/hooks/useAdminMasters";
import { textbookMasterInputSchema } from "@/shared/validations/master";
import { RANGE_UNITS } from "@/shared/validations/studyPlan";

// 管理者ページのマスター編集：参考書（/admin/masters?tab=textbooks）。
// 利用者が「参考書から選ぶ」ときの候補と、登録時に入る総量（isDefault の単位）を管理する。
// 削除は利用者の参考書に1冊も使われていないものだけ（使われていれば API が 409）。

const unitLabel = (unit: string) => RANGE_UNITS.find((item) => item.value === unit)?.label ?? unit;
const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : "読み込みに失敗しました";

export default function TextbookMastersSection() {
  const [draft, setDraft] = useState("");
  const [q, setQ] = useState("");
  // null＝閉じている、"new"＝追加、参考書＝その参考書の編集
  const [dialog, setDialog] = useState<AdminTextbookMaster | "new" | null>(null);
  const masters = useAdminTextbookMasters(q);
  const deleteMaster = useDeleteTextbookMaster();

  const handleSearch = (event: FormEvent) => {
    event.preventDefault();
    setQ(draft.trim());
  };

  const handleDelete = (master: AdminTextbookMaster) => {
    if (!window.confirm(`「${master.name}」を削除しますか？`)) return;
    deleteMaster.mutate(master.id, {
      onSuccess: () => toast.success("参考書を削除しました"),
      onError: (error) => toast.error(error.message),
    });
  };

  return (
    <div className="space-y-3">
      <div className="flex gap-2">
        <form onSubmit={handleSearch} className="flex flex-1 gap-2">
          <Input
            type="search"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="参考書名・出版社・ISBN で検索"
            aria-label="参考書名・出版社・ISBN で検索"
          />
          <Button type="submit" variant="outline" size="lg">
            検索
          </Button>
        </form>
        <Button type="button" size="lg" onClick={() => setDialog("new")}>
          追加
        </Button>
      </div>

      {!masters.data ? (
        masters.error ? (
          <p className="text-sm text-destructive">{errorMessage(masters.error)}</p>
        ) : (
          <p className="text-sm text-muted-foreground">読み込み中…</p>
        )
      ) : masters.data.length === 0 ? (
        <p className="text-sm text-muted-foreground">該当する参考書はありません。</p>
      ) : (
        <Card className="py-0">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[48rem] text-sm">
              <thead className="border-b text-left text-xs text-muted-foreground">
                <tr>
                  <th className="px-4 py-2 font-medium">参考書</th>
                  <th className="px-4 py-2 font-medium">ISBN</th>
                  <th className="px-4 py-2 font-medium">総量</th>
                  <th className="px-4 py-2 text-right font-medium">利用</th>
                  <th className="px-4 py-2" />
                </tr>
              </thead>
              <tbody className="divide-y">
                {masters.data.map((master) => (
                  <tr key={master.id} className="align-top">
                    <td className="px-4 py-2">
                      <p className="font-medium">{master.name}</p>
                      <p className="text-xs text-muted-foreground">
                        {[master.publisher, master.edition].filter(Boolean).join("・") || "—"}
                      </p>
                    </td>
                    <td className="whitespace-nowrap px-4 py-2 tabular-nums">{master.isbn}</td>
                    <td className="px-4 py-2 text-xs">
                      {master.metrics.map((metric) => (
                        <span key={metric.unit} className="mr-2 inline-block tabular-nums">
                          {metric.totalAmount.toLocaleString()}
                          {unitLabel(metric.unit)}
                          {metric.isDefault ? "（既定）" : ""}
                        </span>
                      ))}
                    </td>
                    <td className="px-4 py-2 text-right tabular-nums">{master.textbookCount}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-right">
                      <Button type="button" variant="ghost" size="sm" onClick={() => setDialog(master)}>
                        編集
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={master.textbookCount > 0 || deleteMaster.isPending}
                        title={master.textbookCount > 0 ? "利用者の参考書に使われているため削除できません" : undefined}
                        onClick={() => handleDelete(master)}
                      >
                        削除
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {dialog !== null ? (
        <TextbookMasterDialog master={dialog === "new" ? undefined : dialog} onClose={() => setDialog(null)} />
      ) : null}
    </div>
  );
}

type MetricDraft = { unit: string; totalAmount: string; isDefault: boolean };

function TextbookMasterDialog({ master, onClose }: { master?: AdminTextbookMaster; onClose: () => void }) {
  const save = useSaveTextbookMaster();
  const [name, setName] = useState(master?.name ?? "");
  const [publisher, setPublisher] = useState(master?.publisher ?? "");
  const [edition, setEdition] = useState(master?.edition ?? "");
  const [isbn, setIsbn] = useState(master?.isbn ?? "");
  const [metrics, setMetrics] = useState<MetricDraft[]>(
    master?.metrics.map((metric) => ({ ...metric, totalAmount: String(metric.totalAmount) })) ?? [
      { unit: "page", totalAmount: "", isDefault: true },
    ]
  );
  const [formError, setFormError] = useState<string | null>(null);

  const updateMetric = (index: number, change: Partial<MetricDraft>) =>
    setMetrics((current) => current.map((metric, i) => (i === index ? { ...metric, ...change } : metric)));
  const setDefault = (index: number) =>
    setMetrics((current) => current.map((metric, i) => ({ ...metric, isDefault: i === index })));
  const removeMetric = (index: number) =>
    setMetrics((current) => {
      const next = current.filter((_, i) => i !== index);
      // 既定の単位を消したら、残りの先頭を既定にする。
      if (next.length > 0 && !next.some((metric) => metric.isDefault)) next[0] = { ...next[0]!, isDefault: true };
      return next;
    });
  const unusedUnit = RANGE_UNITS.find((unit) => !metrics.some((metric) => metric.unit === unit.value));

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const parsed = textbookMasterInputSchema.safeParse({
      name,
      publisher,
      edition,
      isbn,
      metrics: metrics.map((metric) => ({
        ...metric,
        totalAmount: metric.totalAmount === "" ? Number.NaN : Number(metric.totalAmount),
      })),
    });
    if (!parsed.success) {
      setFormError(parsed.error.issues[0]?.message ?? "入力を確認してください");
      return;
    }
    setFormError(null);
    save.mutate(
      { id: master?.id, data: parsed.data },
      {
        onSuccess: () => {
          toast.success(master ? "参考書を保存しました" : "参考書を追加しました");
          onClose();
        },
        onError: (error) => setFormError(error.message),
      }
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{master ? "参考書を編集" : "参考書を追加"}</DialogTitle>
            <DialogDescription>
              総量を変えても、利用者がすでに登録した参考書の総量は変わりません（これから登録する人から効きます）。
            </DialogDescription>
          </DialogHeader>
          <div>
            <Label htmlFor="master-textbook-name">参考書名</Label>
            <Input id="master-textbook-name" className="mt-2" value={name} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="master-textbook-publisher">出版社（任意）</Label>
              <Input
                id="master-textbook-publisher"
                className="mt-2"
                value={publisher}
                onChange={(event) => setPublisher(event.target.value)}
              />
            </div>
            <div>
              <Label htmlFor="master-textbook-edition">版（任意）</Label>
              <Input
                id="master-textbook-edition"
                className="mt-2"
                value={edition}
                onChange={(event) => setEdition(event.target.value)}
              />
            </div>
          </div>
          <div>
            <Label htmlFor="master-textbook-isbn">ISBN</Label>
            <Input
              id="master-textbook-isbn"
              className="mt-2"
              inputMode="numeric"
              placeholder="978-4-01-034646-4"
              value={isbn}
              onChange={(event) => setIsbn(event.target.value)}
            />
          </div>
          <fieldset>
            <legend className="text-sm font-medium">総量（単位ごと）</legend>
            <p className="mt-1 text-xs text-muted-foreground">
              「既定」の単位の総量が、利用者の登録時に入ります。
            </p>
            <div className="mt-2 space-y-2">
              {metrics.map((metric, index) => (
                <div key={index} className="flex items-center gap-2">
                  <select
                    aria-label="単位"
                    className="h-9 rounded-lg border bg-transparent px-2 text-sm"
                    value={metric.unit}
                    onChange={(event) => updateMetric(index, { unit: event.target.value })}
                  >
                    {RANGE_UNITS.map((unit) => (
                      <option key={unit.value} value={unit.value}>
                        {unit.label}
                      </option>
                    ))}
                  </select>
                  <Input
                    aria-label="総量"
                    type="number"
                    inputMode="numeric"
                    min={1}
                    className="w-28"
                    value={metric.totalAmount}
                    onChange={(event) => updateMetric(index, { totalAmount: event.target.value })}
                  />
                  <label className="flex items-center gap-1 text-sm">
                    <input
                      type="radio"
                      name="master-textbook-default"
                      checked={metric.isDefault}
                      onChange={() => setDefault(index)}
                    />
                    既定
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={metrics.length <= 1}
                    onClick={() => removeMetric(index)}
                  >
                    外す
                  </Button>
                </div>
              ))}
            </div>
            {unusedUnit ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="mt-2"
                onClick={() =>
                  setMetrics((current) => [...current, { unit: unusedUnit.value, totalAmount: "", isDefault: false }])
                }
              >
                単位を追加
              </Button>
            ) : null}
          </fieldset>
          {formError ? <p className="text-sm text-destructive">{formError}</p> : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              キャンセル
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {save.isPending ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
