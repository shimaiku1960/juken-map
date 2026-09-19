import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router";
import { toast } from "sonner";
import TextbookMastersSection from "@/web/components/admin/TextbookMastersSection";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import SectionHeader from "@/web/components/layout/SectionHeader";
import { Badge } from "@/web/components/ui/badge";
import { Button } from "@/web/components/ui/button";
import { Card, CardContent } from "@/web/components/ui/card";
import { Checkbox } from "@/web/components/ui/checkbox";
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
import { cn } from "@/web/lib/utils";
import {
  useAdminTags,
  useAdminUniversities,
  useAdminUniversity,
  useDeleteFaculty,
  useDeleteUniversity,
  useSaveFaculty,
  useSaveUniversity,
  type AdminFaculty,
  type AdminUniversity,
} from "@/web/hooks/useAdminMasters";
import { REGIONS } from "@/shared/prefectures";
import {
  UNIVERSITY_TYPES,
  facultyInputSchema,
  universityInputSchema,
} from "@/shared/validations/master";

// 管理者ページのマスター編集。タブで大学・学部と参考書（components/admin/TextbookMastersSection）を切り替える。
// 大学・学部は、大学を検索して選ぶとその学部の一覧が出る。
// 削除は誰にも使われていない行だけ（使われていれば API が 409 と理由を返す）。
// 権限の判定は API（requireAdmin）。403 なら「権限がありません」を出すだけ。

const isForbidden = (error: unknown) => (error as { status?: number } | null)?.status === 403;
const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : "読み込みに失敗しました";

const selectClass = "mt-2 h-10 w-full rounded-lg border bg-transparent px-3 text-sm";

type Tab = "universities" | "textbooks";

const TAB_LABELS: Record<Tab, string> = { universities: "大学・学部", textbooks: "参考書" };

export default function AdminMastersPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: Tab = searchParams.get("tab") === "textbooks" ? "textbooks" : "universities";
  const selectedId = Number(searchParams.get("university")) || null;
  const select = (id: number | null) =>
    setSearchParams(id === null ? {} : { university: String(id) }, { replace: true });
  const switchTab = (next: Tab) =>
    setSearchParams(next === "textbooks" ? { tab: "textbooks" } : {}, { replace: true });

  return (
    <PageShell className="max-w-6xl">
      <PageHeader
        title="マスター編集"
        description="大学・学部と参考書を追加・編集します。変更はすぐに利用者の画面（大学を探す・志望校・参考書の候補）に反映されます。"
        action={
          <Button asChild variant="outline" size="sm">
            <Link to="/admin">管理トップへ</Link>
          </Button>
        }
      />
      <div className="mb-6 flex gap-1" role="tablist" aria-label="マスターの種類">
        {(Object.keys(TAB_LABELS) as Tab[]).map((value) => (
          <Button
            key={value}
            type="button"
            role="tab"
            aria-selected={tab === value}
            variant={tab === value ? "default" : "outline"}
            size="sm"
            onClick={() => switchTab(value)}
          >
            {TAB_LABELS[value]}
          </Button>
        ))}
      </div>
      {tab === "textbooks" ? (
        <TextbookMastersSection />
      ) : (
        <div className="grid gap-8 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
          <section>
            <SectionHeader title="大学" />
            <UniversityListSection selectedId={selectedId} onSelect={select} />
          </section>
          <section>
            <SectionHeader title="学部" />
            {selectedId === null ? (
              <p className="text-sm text-muted-foreground">左の一覧から大学を選んでください。</p>
            ) : (
              <UniversityDetailSection id={selectedId} onDeleted={() => select(null)} />
            )}
          </section>
        </div>
      )}
    </PageShell>
  );
}

function UniversityListSection({
  selectedId,
  onSelect,
}: {
  selectedId: number | null;
  onSelect: (id: number) => void;
}) {
  const [draft, setDraft] = useState("");
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const [creating, setCreating] = useState(false);
  const universities = useAdminUniversities({ q, page });
  const list = universities.data;
  const totalPages = list ? Math.max(1, Math.ceil(list.total / list.pageSize)) : 1;

  const handleSearch = (event: FormEvent) => {
    event.preventDefault();
    setQ(draft.trim());
    setPage(1);
  };

  if (isForbidden(universities.error)) {
    return <p className="text-sm text-muted-foreground">このページを見る権限がありません。</p>;
  }

  return (
    <div className="space-y-3">
      <div className="flex gap-2">
        <form onSubmit={handleSearch} className="flex flex-1 gap-2">
          <Input
            type="search"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="大学名で検索"
            aria-label="大学名で検索"
          />
          <Button type="submit" variant="outline" size="lg">
            検索
          </Button>
        </form>
        <Button type="button" size="lg" onClick={() => setCreating(true)}>
          追加
        </Button>
      </div>

      {!list ? (
        universities.error ? (
          <p className="text-sm text-destructive">{errorMessage(universities.error)}</p>
        ) : (
          <p className="text-sm text-muted-foreground">読み込み中…</p>
        )
      ) : list.universities.length === 0 ? (
        <p className="text-sm text-muted-foreground">該当する大学はありません。</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">{list.total.toLocaleString()}校</p>
          <Card className="py-0">
            <ul className="divide-y">
              {list.universities.map((university) => (
                <li key={university.id}>
                  <button
                    type="button"
                    onClick={() => onSelect(university.id)}
                    aria-current={selectedId === university.id}
                    className={cn(
                      "flex w-full items-center justify-between gap-3 px-4 py-2.5 text-left text-sm hover:bg-muted/50",
                      selectedId === university.id && "bg-info/10"
                    )}
                  >
                    <span className="min-w-0">
                      <span className="block truncate font-medium">{university.name}</span>
                      <span className="text-xs text-muted-foreground">
                        {university.prefecture}・{university.type}
                      </span>
                    </span>
                    <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                      学部 {university.facultyCount}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </Card>
          <div className="flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((current) => current - 1)}
            >
              前へ
            </Button>
            <span className="text-xs tabular-nums text-muted-foreground">
              {page} / {totalPages}
            </span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((current) => current + 1)}
            >
              次へ
            </Button>
          </div>
        </>
      )}

      {creating ? (
        <UniversityDialog
          onClose={() => setCreating(false)}
          onSaved={(saved) => {
            setCreating(false);
            onSelect(saved.id);
          }}
        />
      ) : null}
    </div>
  );
}

function UniversityDetailSection({ id, onDeleted }: { id: number; onDeleted: () => void }) {
  const detail = useAdminUniversity(id);
  const deleteUniversity = useDeleteUniversity();
  const deleteFaculty = useDeleteFaculty();
  const [editingUniversity, setEditingUniversity] = useState(false);
  // null＝閉じている、"new"＝追加、学部＝その学部の編集
  const [facultyDialog, setFacultyDialog] = useState<AdminFaculty | "new" | null>(null);

  if (!detail.data) {
    return detail.error ? (
      <p className="text-sm text-destructive">{errorMessage(detail.error)}</p>
    ) : (
      <p className="text-sm text-muted-foreground">読み込み中…</p>
    );
  }
  const { university, faculties } = detail.data;

  const handleDeleteUniversity = () => {
    if (!window.confirm(`「${university.name}」と、その学部${faculties.length}件を削除しますか？`)) return;
    deleteUniversity.mutate(university.id, {
      onSuccess: () => {
        toast.success("大学を削除しました");
        onDeleted();
      },
      onError: (error) => toast.error(error.message),
    });
  };

  const handleDeleteFaculty = (faculty: AdminFaculty) => {
    if (!window.confirm(`「${faculty.name}」を削除しますか？`)) return;
    deleteFaculty.mutate(faculty.id, {
      onSuccess: () => toast.success("学部を削除しました"),
      onError: (error) => toast.error(error.message),
    });
  };

  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="flex flex-wrap items-start justify-between gap-3 py-4">
          <div className="min-w-0">
            <p className="text-lg font-semibold">{university.name}</p>
            <p className="text-sm text-muted-foreground">
              {university.prefecture}・{university.type}・志望校に使われている {university.goalCount}件
            </p>
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setEditingUniversity(true)}>
              編集
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={university.goalCount > 0 || deleteUniversity.isPending}
              title={university.goalCount > 0 ? "学部が志望校に使われているため削除できません" : undefined}
              onClick={handleDeleteUniversity}
            >
              削除
            </Button>
          </div>
        </CardContent>
      </Card>

      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">学部 {faculties.length}件</p>
        <Button type="button" size="sm" onClick={() => setFacultyDialog("new")}>
          学部を追加
        </Button>
      </div>

      {faculties.length === 0 ? (
        <p className="text-sm text-muted-foreground">まだ学部がありません。</p>
      ) : (
        <Card className="py-0">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[36rem] text-sm">
              <thead className="border-b text-left text-xs text-muted-foreground">
                <tr>
                  <th className="px-4 py-2 font-medium">学部</th>
                  <th className="px-4 py-2 font-medium">受験日</th>
                  <th className="px-4 py-2 font-medium">系統</th>
                  <th className="px-4 py-2 text-right font-medium">志望校</th>
                  <th className="px-4 py-2" />
                </tr>
              </thead>
              <tbody className="divide-y">
                {faculties.map((faculty) => (
                  <tr key={faculty.id} className="align-top">
                    <td className="px-4 py-2 font-medium">{faculty.name}</td>
                    <td className="whitespace-nowrap px-4 py-2 tabular-nums">{faculty.examDate}</td>
                    <td className="px-4 py-2">
                      <div className="flex flex-wrap gap-1">
                        {faculty.tags.map((tag) => (
                          <Badge key={tag.id} variant="outline">
                            {tag.name}
                          </Badge>
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-2 text-right tabular-nums">{faculty.goalCount}</td>
                    <td className="whitespace-nowrap px-4 py-2 text-right">
                      <Button type="button" variant="ghost" size="sm" onClick={() => setFacultyDialog(faculty)}>
                        編集
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={faculty.goalCount > 0 || deleteFaculty.isPending}
                        title={faculty.goalCount > 0 ? "志望校に使われているため削除できません" : undefined}
                        onClick={() => handleDeleteFaculty(faculty)}
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

      {editingUniversity ? (
        <UniversityDialog
          university={university}
          onClose={() => setEditingUniversity(false)}
          onSaved={() => setEditingUniversity(false)}
        />
      ) : null}
      {facultyDialog !== null ? (
        <FacultyDialog
          universityId={university.id}
          faculty={facultyDialog === "new" ? undefined : facultyDialog}
          onClose={() => setFacultyDialog(null)}
        />
      ) : null}
    </div>
  );
}

function UniversityDialog({
  university,
  onClose,
  onSaved,
}: {
  university?: AdminUniversity;
  onClose: () => void;
  onSaved: (saved: AdminUniversity) => void;
}) {
  const save = useSaveUniversity();
  const [name, setName] = useState(university?.name ?? "");
  const [prefecture, setPrefecture] = useState(university?.prefecture ?? "");
  const [type, setType] = useState(university?.type ?? "");
  const [formError, setFormError] = useState<string | null>(null);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const parsed = universityInputSchema.safeParse({ name, prefecture, type });
    if (!parsed.success) {
      setFormError(parsed.error.issues[0]?.message ?? "入力を確認してください");
      return;
    }
    setFormError(null);
    save.mutate(
      { id: university?.id, data: parsed.data },
      {
        onSuccess: (saved) => {
          toast.success(university ? "大学を保存しました" : "大学を追加しました");
          onSaved(saved);
        },
        onError: (error) => setFormError(error.message),
      }
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{university ? "大学を編集" : "大学を追加"}</DialogTitle>
            <DialogDescription>大学名は他の大学と重ならないようにしてください。</DialogDescription>
          </DialogHeader>
          <div>
            <Label htmlFor="master-university-name">大学名</Label>
            <Input
              id="master-university-name"
              className="mt-2"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="master-university-prefecture">都道府県</Label>
              <select
                id="master-university-prefecture"
                className={selectClass}
                value={prefecture}
                onChange={(event) => setPrefecture(event.target.value)}
              >
                <option value="">選択</option>
                {REGIONS.map((region) => (
                  <optgroup key={region.region} label={region.region}>
                    {region.prefs.map((pref) => (
                      <option key={pref} value={pref}>
                        {pref}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </div>
            <div>
              <Label htmlFor="master-university-type">種別</Label>
              <select
                id="master-university-type"
                className={selectClass}
                value={type}
                onChange={(event) => setType(event.target.value)}
              >
                <option value="">選択</option>
                {UNIVERSITY_TYPES.map((value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
              </select>
            </div>
          </div>
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

function FacultyDialog({
  universityId,
  faculty,
  onClose,
}: {
  universityId: number;
  faculty?: AdminFaculty;
  onClose: () => void;
}) {
  const save = useSaveFaculty();
  const tags = useAdminTags();
  const [name, setName] = useState(faculty?.name ?? "");
  const [examDate, setExamDate] = useState(faculty?.examDate ?? "");
  const [tagIds, setTagIds] = useState<number[]>(faculty?.tags.map((tag) => tag.id) ?? []);
  const [formError, setFormError] = useState<string | null>(null);

  const toggleTag = (id: number, checked: boolean) =>
    setTagIds((current) => (checked ? [...current, id] : current.filter((tagId) => tagId !== id)));

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const parsed = facultyInputSchema.safeParse({ name, examDate, tagIds });
    if (!parsed.success) {
      setFormError(parsed.error.issues[0]?.message ?? "入力を確認してください");
      return;
    }
    setFormError(null);
    save.mutate(
      faculty ? { id: faculty.id, data: parsed.data } : { data: { ...parsed.data, universityId } },
      {
        onSuccess: () => {
          toast.success(faculty ? "学部を保存しました" : "学部を追加しました");
          onClose();
        },
        onError: (error) => setFormError(error.message),
      }
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{faculty ? "学部を編集" : "学部を追加"}</DialogTitle>
            <DialogDescription>
              受験日は、志望校にしている利用者のカウントダウンにそのまま反映されます。
            </DialogDescription>
          </DialogHeader>
          <div>
            <Label htmlFor="master-faculty-name">学部名</Label>
            <Input
              id="master-faculty-name"
              className="mt-2"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="master-faculty-exam-date">受験日</Label>
            <Input
              id="master-faculty-exam-date"
              type="date"
              className="mt-2"
              value={examDate}
              onChange={(event) => setExamDate(event.target.value)}
            />
          </div>
          <fieldset>
            <legend className="text-sm font-medium">系統</legend>
            {tags.data ? (
              <div className="mt-2 grid grid-cols-2 gap-2">
                {tags.data.map((tag) => (
                  <label key={tag.id} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={tagIds.includes(tag.id)}
                      onCheckedChange={(checked) => toggleTag(tag.id, checked === true)}
                    />
                    {tag.name}
                  </label>
                ))}
              </div>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                {tags.error ? errorMessage(tags.error) : "読み込み中…"}
              </p>
            )}
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
