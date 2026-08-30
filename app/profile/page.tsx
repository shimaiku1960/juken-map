import type { Metadata } from "next";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import { redirect } from "next/navigation";
import { NOINDEX } from "@/lib/site";
import Link from "next/link";
import ProfileEdit from "@/app/components/ProfileEdit";
import { Card, CardContent } from "@/components/ui/card";
import { DEMO_EMAIL } from "@/lib/demo";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";
import SectionHeader from "@/app/components/layout/SectionHeader";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import prisma from "@/lib/prisma";
import NotificationPreferenceForm from "@/app/components/NotificationPreferenceForm";

// ログイン必須のページなので検索結果には載せない。
export const metadata: Metadata = { robots: NOINDEX };

const ProfilePage = async () => {
    const session = await auth.api.getSession({
        headers: await headers(),
    });

    if (!session) {
        redirect("/login");
    }

    const user = session.user;
    const nickname = user.nickname ?? user.name ?? "ユーザー";
    const notificationPreference =
      await prisma.notificationPreference.findUnique({
        where: { userId: user.id },
        select: { morningEnabled: true, eveningEnabled: true },
      });

      return (
        <PageShell>
          <PageHeader title="プロフィール" description="アカウント情報を確認・変更できます。" />
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
          readOnly={user.email === DEMO_EMAIL}
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
        href="/goals"
        className={cn(buttonVariants({ variant: "link", size: "lg" }), "h-11")}
      >
        志望校ページへ →
      </Link>
    </CardContent>
  </Card>
  </section>

  <section className="mt-10">
    <SectionHeader title="メール通知" />
    <Card>
      <CardContent className="py-5">
        <NotificationPreferenceForm
          morningEnabled={notificationPreference?.morningEnabled ?? false}
          eveningEnabled={notificationPreference?.eveningEnabled ?? false}
          readOnly={user.email === DEMO_EMAIL}
        />
      </CardContent>
    </Card>
  </section>
        </PageShell>
      );
    };
  



 
        


export default ProfilePage;
