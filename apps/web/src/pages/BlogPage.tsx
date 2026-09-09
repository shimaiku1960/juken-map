import { Link } from "react-router";
import { Card, CardContent } from "@/web/components/ui/card";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import { useBlogList } from "@/web/hooks/useBlog";

export default function BlogPage() {
  const { data, isPending, isError } = useBlogList();

  return (
    <PageShell>
      <PageHeader title="ブログ記事一覧" />
      {isPending ? (
        <p className="text-sm text-muted-foreground">読み込み中…</p>
      ) : isError ? (
        <p className="text-sm text-muted-foreground">
          記事を取得できませんでした。時間をおいてもう一度お試しください。
        </p>
      ) : (
        <ul className="space-y-4">
          {data.contents.map((blog) => (
            <li key={blog.id}>
              <Link to={`/articles/${blog.id}`}>
                <Card>
                  <CardContent>
                    <h2 className="text-xl font-semibold">{blog.title}</h2>
                    <time className="text-sm text-muted-foreground">
                      {new Date(blog.createdAt).toLocaleDateString("ja-JP")}
                    </time>
                  </CardContent>
                </Card>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </PageShell>
  );
}
