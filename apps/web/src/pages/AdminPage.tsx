import { useState, type FormEvent } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import SectionHeader from "@/web/components/layout/SectionHeader";
import { Badge } from "@/web/components/ui/badge";
import { Button } from "@/web/components/ui/button";
import { Card, CardContent } from "@/web/components/ui/card";
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
import { useSession } from "@/web/lib/auth-client";
import { cn } from "@/web/lib/utils";
import { DEMO_EMAIL } from "@/shared/demo";
import {
  useAdminOverview,
  useAdminUsers,
  useBanUser,
  useDeleteUser,
  type AdminOverview,
  type AdminUser,
  type UserKind,
} from "@/web/hooks/useAdmin";

// 管理者ページ。利用状況の数字と、ユーザー一覧（読み取りのみ）。
// 権限の判定は API（requireAdmin）。ここは 403 を受けたら「権限がありません」を出すだけ。

const KIND_LABELS: Record<UserKind, string> = {
  real: "実ユーザー",
  sim: "シミュレーション",
  seed: "seed 合成",
  demo: "デモ",
};

const PROVIDER_LABELS: Record<string, string> = {
  credential: "メール",
  google: "Google",
  github: "GitHub",
};

const dateTimeFormat = new Intl.DateTimeFormat("ja-JP", {
  timeZone: "Asia/Tokyo",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
});

const formatDateTime = (iso: string | null) => (iso ? dateTimeFormat.format(new Date(iso)) : "—");

const isForbidden = (error: unknown) => (error as { status?: number } | null)?.status === 403;

const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : "読み込みに失敗しました";

export default function AdminPage() {
  const overview = useAdminOverview();

  if (isForbidden(overview.error)) {
    return (
      <PageShell>
        <PageHeader title="管理" />
        <Card>
          <CardContent className="py-6 text-sm text-muted-foreground">
            このページを見る権限がありません。
          </CardContent>
        </Card>
      </PageShell>
    );
  }

  return (
    <PageShell className="max-w-6xl">
      <PageHeader
        title="管理"
        description="利用状況とユーザーの一覧です。停止・削除もここから行います。"
        action={
          <Button asChild variant="outline" size="sm">
            <Link to="/admin/masters">マスター編集</Link>
          </Button>
        }
      />

      <section>
        <SectionHeader title="利用状況" />
        {overview.data ? (
          <OverviewSection overview={overview.data} />
        ) : overview.error ? (
          <p className="text-sm text-destructive">{errorMessage(overview.error)}</p>
        ) : (
          <p className="text-sm text-muted-foreground">読み込み中…</p>
        )}
      </section>

      <section className="mt-10">
        <SectionHeader title="ユーザー" />
        <UserListSection />
      </section>
    </PageShell>
  );
}

function OverviewSection({ overview }: { overview: AdminOverview }) {
  const [real, ...others] = overview.kinds;

  return (
    <div className="space-y-4">
      {real ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label="実ユーザー" value={real.total} note={`確認済み ${real.verified}`} />
          <Stat label="直近7日の新規登録" value={real.newLast7Days} />
          <Stat label="直近7日に記録した人" value={real.activeLast7Days} />
          <Stat label="直近30日に記録した人" value={real.activeLast30Days} />
        </div>
      ) : null}

      <SignupsByDay days={overview.realSignupsByDay} />

      {/* 合成ユーザーとデモは実ユーザーと混ぜずに、控えめに並べる */}
      <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs text-muted-foreground">
        {others.map((row) => (
          <span key={row.kind}>
            {KIND_LABELS[row.kind]}：{row.total}人（7日以内に記録 {row.activeLast7Days}）
          </span>
        ))}
      </div>
    </div>
  );
}

function Stat({ label, value, note }: { label: string; value: number; note?: string }) {
  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="mt-1 text-2xl font-semibold tabular-nums">{value.toLocaleString()}</p>
        {note ? <p className="mt-0.5 text-xs text-muted-foreground">{note}</p> : null}
      </CardContent>
    </Card>
  );
}

// API は登録があった日だけを返すので、登録0の日も埋めて30日分の棒にする。
function lastThirtyDays() {
  const format = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Tokyo" });
  return Array.from({ length: 30 }, (_, index) =>
    format.format(new Date(Date.now() - (29 - index) * 24 * 60 * 60 * 1000))
  );
}

function SignupsByDay({ days }: { days: AdminOverview["realSignupsByDay"] }) {
  const counts = new Map(days.map((day) => [day.date, day.count]));
  const series = lastThirtyDays().map((date) => ({ date, count: counts.get(date) ?? 0 }));
  const max = Math.max(1, ...series.map((day) => day.count));
  const total = series.reduce((sum, day) => sum + day.count, 0);

  return (
    <Card>
      <CardContent className="py-4">
        <p className="text-xs text-muted-foreground">
          実ユーザーの新規登録（直近30日・合計 {total}人）
        </p>
        <div className="mt-3 flex h-20 items-end gap-0.5" role="img" aria-label={`直近30日の新規登録 合計${total}人`}>
          {series.map((day) => (
            <div
              key={day.date}
              title={`${day.date}：${day.count}人`}
              className={cn("flex-1 rounded-sm", day.count > 0 ? "bg-primary" : "bg-muted")}
              style={{ height: `${Math.max(4, (day.count / max) * 100)}%` }}
            />
          ))}
        </div>
        <div className="mt-1 flex justify-between text-[10px] text-muted-foreground">
          <span>{series[0]?.date.slice(5)}</span>
          <span>{series[series.length - 1]?.date.slice(5)}</span>
        </div>
      </CardContent>
    </Card>
  );
}

function UserListSection() {
  const [kind, setKind] = useState<UserKind>("real");
  const [draft, setDraft] = useState("");
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const [deleteTarget, setDeleteTarget] = useState<AdminUser | null>(null);
  const users = useAdminUsers({ kind, q, page });
  const list = users.data;

  // 自分自身には手を出せない（API も 409 で断るが、押せないほうが分かりやすい）。
  const { data: session } = useSession();
  const banUser = useBanUser();

  const handleBan = (user: AdminUser) => {
    const banned = user.bannedAt === null;
    const label = userLabel(user);
    if (banned && !window.confirm(`${label} を停止しますか？ ログイン中の画面はすぐに切れます。`))
      return;
    banUser.mutate(
      { id: user.id, banned },
      {
        onSuccess: () => toast.success(banned ? "停止しました" : "停止を解除しました"),
        onError: (error) => toast.error(error.message),
      }
    );
  };

  const handleSearch = (event: FormEvent) => {
    event.preventDefault();
    setQ(draft.trim());
    setPage(1);
  };

  const totalPages = list ? Math.max(1, Math.ceil(list.total / list.pageSize)) : 1;

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1" role="tablist" aria-label="ユーザーの種別">
          {(Object.keys(KIND_LABELS) as UserKind[]).map((value) => (
            <Button
              key={value}
              type="button"
              role="tab"
              aria-selected={kind === value}
              variant={kind === value ? "default" : "outline"}
              size="sm"
              onClick={() => {
                setKind(value);
                setPage(1);
              }}
            >
              {KIND_LABELS[value]}
            </Button>
          ))}
        </div>
        <form onSubmit={handleSearch} className="flex w-full gap-2 sm:w-auto">
          <Input
            type="search"
            value={draft}
            onChange={(event) => setDraft(event.target.value)}
            placeholder="メールアドレスで検索"
            aria-label="メールアドレスで検索"
            className="sm:w-64"
          />
          <Button type="submit" variant="outline" size="lg">
            検索
          </Button>
        </form>
      </div>

      {!list ? (
        users.error ? (
          <p className="text-sm text-destructive">{errorMessage(users.error)}</p>
        ) : (
          <p className="text-sm text-muted-foreground">読み込み中…</p>
        )
      ) : list.users.length === 0 ? (
        <p className="text-sm text-muted-foreground">該当するユーザーはいません。</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {list.total.toLocaleString()}人中 {(page - 1) * list.pageSize + 1}〜
            {(page - 1) * list.pageSize + list.users.length}人目
          </p>
          <Card className="py-0">
            {/* 表は横に長いので、ページ全体ではなくこの枠の中だけ横にスクロールさせる */}
            <div className="overflow-x-auto">
              <table className="w-full min-w-[56rem] text-sm">
                <thead className="border-b text-left text-xs text-muted-foreground">
                  <tr>
                    <th className="px-4 py-2 font-medium">ユーザー</th>
                    <th className="px-4 py-2 font-medium">登録</th>
                    <th className="px-4 py-2 font-medium">認証方法</th>
                    <th className="px-4 py-2 font-medium">最終ログイン</th>
                    <th className="px-4 py-2 text-right font-medium">記録</th>
                    <th className="px-4 py-2 font-medium">最終記録</th>
                    <th className="px-4 py-2 text-right font-medium">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {list.users.map((user) => (
                    <UserRow
                      key={user.id}
                      user={user}
                      protectedReason={protectedReason(user, session?.user.id)}
                      busy={banUser.isPending}
                      onBan={() => handleBan(user)}
                      onDelete={() => setDeleteTarget(user)}
                    />
                  ))}
                </tbody>
              </table>
            </div>
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

      {deleteTarget ? (
        <DeleteUserDialog user={deleteTarget} onClose={() => setDeleteTarget(null)} />
      ) : null}
    </div>
  );
}

const userLabel = (user: AdminUser) =>
  user.email ?? user.nickname?.trim() ?? user.name?.trim() ?? user.id;

/**
 * 停止・削除ができない相手なら、その理由の文言を返す（できるなら null）。
 * 判定は API（admin-service.ts の findTarget）と同じ3つ。ここは押せなくするためだけで、
 * 守っているのはサーバー側である。
 */
function protectedReason(user: AdminUser, currentUserId: string | undefined): string | null {
  if (user.id === currentUserId) return "自分自身は操作できません";
  if (user.role === "admin") return "他の管理者は操作できません";
  if (user.email === DEMO_EMAIL) return "デモアカウントは操作できません";
  return null;
}

function UserRow({
  user,
  protectedReason: reason,
  busy,
  onBan,
  onDelete,
}: {
  user: AdminUser;
  protectedReason: string | null;
  busy: boolean;
  onBan: () => void;
  onDelete: () => void;
}) {
  const displayName = user.nickname?.trim() || user.name?.trim() || "（名前なし）";

  return (
    <tr className={cn("align-top", user.bannedAt && "bg-muted/40")}>
      <td className="px-4 py-2">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="font-medium">{displayName}</span>
          {user.role === "admin" ? <Badge variant="info">管理者</Badge> : null}
          {user.bannedAt ? <Badge variant="destructive">停止中</Badge> : null}
          {!user.emailVerified ? <Badge variant="warning">未確認</Badge> : null}
        </div>
        <p className="text-xs text-muted-foreground">{user.email ?? "—"}</p>
        {user.bannedAt ? (
          <p className="text-xs text-muted-foreground">{formatDateTime(user.bannedAt)} に停止</p>
        ) : null}
      </td>
      <td className="whitespace-nowrap px-4 py-2 tabular-nums">{formatDateTime(user.createdAt)}</td>
      <td className="px-4 py-2">
        {user.providers.length > 0
          ? user.providers.map((provider) => PROVIDER_LABELS[provider] ?? provider).join("・")
          : "—"}
      </td>
      <td className="whitespace-nowrap px-4 py-2 tabular-nums">{formatDateTime(user.lastLoginAt)}</td>
      <td className="px-4 py-2 text-right tabular-nums">{user.studyLogCount.toLocaleString()}</td>
      <td className="whitespace-nowrap px-4 py-2 tabular-nums">
        {formatDateTime(user.lastStudyLogAt)}
      </td>
      <td className="whitespace-nowrap px-4 py-2 text-right">
        <div className="flex justify-end gap-1" title={reason ?? undefined}>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={reason !== null || busy}
            onClick={onBan}
          >
            {user.bannedAt ? "解除" : "停止"}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-destructive hover:text-destructive"
            disabled={reason !== null}
            onClick={onDelete}
          >
            削除
          </Button>
        </div>
      </td>
    </tr>
  );
}

/**
 * 削除の確認。押し間違いで消えないよう、本人のメールアドレスを打ち込ませる。
 * 打った文字はそのままサーバーへ送り、あちらでも本人のものと突き合わせる
 * （一覧が古くて別の行を指していた場合に、id だけを信じないため）。
 */
function DeleteUserDialog({ user, onClose }: { user: AdminUser; onClose: () => void }) {
  const [typed, setTyped] = useState("");
  const deleteUser = useDeleteUser();
  const email = user.email ?? "";
  const matches = typed.trim().toLowerCase() === email.toLowerCase() && email !== "";

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!matches) return;
    deleteUser.mutate(
      { id: user.id, email: typed.trim() },
      {
        onSuccess: () => {
          toast.success("ユーザーを削除しました");
          onClose();
        },
        onError: (error) => toast.error(error.message),
      }
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>ユーザーを削除</DialogTitle>
            <DialogDescription>
              学習記録・予定・参考書・志望校もすべて消えます。取り消せません。
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2">
            <Label htmlFor="delete-user-email">
              確認のため <span className="font-medium text-foreground">{email || "（メール未設定）"}</span> と入力
            </Label>
            <Input
              id="delete-user-email"
              value={typed}
              onChange={(event) => setTyped(event.target.value)}
              autoComplete="off"
              placeholder={email}
            />
            <p className="text-xs text-muted-foreground">
              記録 {user.studyLogCount.toLocaleString()} 件が一緒に消えます。
            </p>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              やめる
            </Button>
            <Button type="submit" variant="destructive" disabled={!matches || deleteUser.isPending}>
              {deleteUser.isPending ? "削除中…" : "削除する"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
