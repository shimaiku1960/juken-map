import { Link } from "react-router";
import { buttonVariants } from "@/web/components/ui/button";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";

export default function NotFoundPage() {
  return (
    <PageShell>
      <PageHeader
        title="ページが見つかりません"
        description="URL が変わったか、削除された可能性があります。"
      />
      <Link to="/" className={buttonVariants()}>
        トップへ戻る
      </Link>
    </PageShell>
  );
}
