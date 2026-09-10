import { Link, useSearchParams } from "react-router";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import InlineFeedback from "@/web/components/feedback/InlineFeedback";
import LineAccountLinkButton from "@/web/components/LineAccountLinkButton";
import { buttonVariants } from "@/web/components/ui/button";
import { cn } from "@/web/lib/utils";
import { useSession } from "@/web/lib/auth-client";

// LINE で「連携」と送ると、この画面への URL が LINE のトークに届く。
// 本人確認のため、ログインしてから /api/line/account-link を叩く。
export default function LineLinkPage() {
  const [searchParams] = useSearchParams();
  const linkToken = searchParams.get("linkToken");
  const { data: session, isPending } = useSession();

  if (!linkToken) {
    return (
      <PageShell className="max-w-lg">
        <PageHeader title="LINE連携" description="連携情報を確認できませんでした。" />
        <InlineFeedback variant="error">
          LINEで「連携」と送って、届いたリンクからもう一度お試しください。
        </InlineFeedback>
      </PageShell>
    );
  }

  // セッション取得前に未ログイン扱いで描くと、ログイン済みの人に一瞬ログイン導線が見える。
  if (isPending) {
    return (
      <PageShell className="max-w-lg">
        <PageHeader title="LINE連携" description="受験マップのアカウントとLINEを安全に接続します。" />
        <p className="text-sm text-muted-foreground">読み込み中…</p>
      </PageShell>
    );
  }

  const callbackURL = `/line/link?linkToken=${encodeURIComponent(linkToken)}`;

  return (
    <PageShell className="max-w-lg">
      <PageHeader
        title="LINE連携"
        description="受験マップのアカウントとLINEを安全に接続します。"
      />
      {session ? (
        <>
          <InlineFeedback variant="info" className="mb-4">
            <p>
              <strong>{session.user.email}</strong> とLINEを連携します。
            </p>
            <p className="mt-1">
              連携後、プロフィールで朝・夜のLINE通知を選べます。
            </p>
          </InlineFeedback>
          <LineAccountLinkButton linkToken={linkToken} />
        </>
      ) : (
        <>
          <InlineFeedback variant="info" className="mb-4">
            本人確認のため、先に受験マップへログインしてください。
          </InlineFeedback>
          <Link
            to={`/login?callbackURL=${encodeURIComponent(callbackURL)}`}
            className={cn(buttonVariants({ size: "lg" }), "h-11 w-full")}
          >
            ログインして連携を続ける
          </Link>
        </>
      )}
    </PageShell>
  );
}
