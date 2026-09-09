import { useParams } from "react-router";
import { Card, CardContent } from "@/web/components/ui/card";
import { useBlogDetail } from "@/web/hooks/useBlog";
import NotFoundPage from "@/web/pages/NotFoundPage";

export default function ArticlePage() {
  const { id = "" } = useParams();
  const { data: blog, isPending, isError } = useBlogDetail(id);

  if (isError) return <NotFoundPage />;
  if (isPending || !blog) {
    return (
      <main className="mx-auto w-full max-w-3xl p-8">
        <p className="text-sm text-muted-foreground">読み込み中…</p>
      </main>
    );
  }

  return (
    <main className="mx-auto w-full max-w-3xl p-8">
      <Card>
        <CardContent className="space-y-4">
          <h1 className="text-3xl font-bold">{blog.title}</h1>
          <time className="text-sm text-muted-foreground">
            {new Date(blog.createdAt).toLocaleDateString("ja-JP")}
          </time>
          {blog.eyecatch && (
            // next/image の最適化は無くなるので、素の img に幅高さを渡す。
            <img
              src={blog.eyecatch.url}
              alt=""
              width={blog.eyecatch.width ?? 1200}
              height={blog.eyecatch.height ?? 630}
              className="h-auto w-full rounded-lg"
            />
          )}
          {/* 本文は microCMS のリッチエディタが出す HTML。Next.js 版と同じ扱い。 */}
          <div
            className="prose"
            dangerouslySetInnerHTML={{ __html: blog.content }}
          />
        </CardContent>
      </Card>
    </main>
  );
}
