import { Link, useSearchParams } from "react-router";
import ProfileEdit from "@/web/components/ProfileEdit";
import NotificationPreferenceForm from "@/web/components/NotificationPreferenceForm";
import { Card, CardContent } from "@/web/components/ui/card";
import { DEMO_EMAIL } from "@/shared/demo";
import { LINE_OFFICIAL_ACCOUNT_URL } from "@/shared/site";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import SectionHeader from "@/web/components/layout/SectionHeader";
import { buttonVariants } from "@/web/components/ui/button";
import { cn } from "@/web/lib/utils";
import { useSession } from "@/web/lib/auth-client";
import {
  useLineConnection,
  useNotificationPreferences,
} from "@/web/hooks/useNotificationPreferences";

export default function ProfilePage() {
  const { data: session } = useSession();
  const [searchParams] = useSearchParams();
  const lineResult = searchParams.get("line") ?? undefined;
  const { data: preference, isPending } = useNotificationPreferences();
  const { data: lineConnection } = useLineConnection();

  if (!session) return null;

  const user = session.user as typeof session.user & {
    nickname?: string | null;
  };
  const nickname = user.nickname ?? user.name ?? "ユーザー";
  const readOnly = user.email === DEMO_EMAIL;

  return (
    <PageShell>
      <PageHeader
        title="プロフィール"
        description="アカウント情報を確認・変更できます。"
      />
      <Card>
        <CardContent className="divide-y">
          <div className="flex items-center gap-3 pb-4">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-info/15 text-lg font-bold text-primary">
              {nickname.charAt(0)}
            </div>
            <p className="font-medium">{nickname}</p>
          </div>
          <div className="py-4">
            <p className="text-sm text-muted-foreground mb-1">ニックネーム</p>
            <ProfileEdit
              currentNickname={user.nickname ?? ""}
              readOnly={readOnly}
            />
          </div>
          <div className="pt-4">
            <p className="text-sm text-muted-foreground mb-1">メールアドレス</p>
            <p className="text-base">{user.email}</p>
          </div>
        </CardContent>
      </Card>

      <section className="mt-10">
        <SectionHeader title="志望校" />
        <Card>
          <CardContent className="flex flex-wrap items-center justify-between gap-3 py-5">
            <p className="text-sm text-muted-foreground">
              志望校の設定・第一志望の切り替えは「志望校」ページに移動しました。
            </p>
            <Link
              to="/goals"
              className={cn(
                buttonVariants({ variant: "link", size: "lg" }),
                "h-11"
              )}
            >
              志望校ページへ →
            </Link>
          </CardContent>
        </Card>
      </section>

      <section id="notification-settings" className="mt-10 scroll-mt-24">
        <SectionHeader title="通知設定" />
        <Card>
          <CardContent className="py-5">
            {isPending ? (
              <p className="text-sm text-muted-foreground">読み込み中…</p>
            ) : (
              <NotificationPreferenceForm
                emailMorningEnabled={preference?.emailMorningEnabled ?? false}
                emailEveningEnabled={preference?.emailEveningEnabled ?? false}
                lineMorningEnabled={preference?.lineMorningEnabled ?? false}
                lineEveningEnabled={preference?.lineEveningEnabled ?? false}
                initialLineConnected={lineConnection?.connected ?? false}
                lineOfficialAccountUrl={LINE_OFFICIAL_ACCOUNT_URL}
                lineResult={lineResult}
                readOnly={readOnly}
              />
            )}
          </CardContent>
        </Card>
      </section>
    </PageShell>
  );
}
