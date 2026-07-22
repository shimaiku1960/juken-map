import { client, type Blog } from "@/lib/microcms";
import Link from "next/link";
import { Card, CardContent } from "@/components/ui/card";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";

export const dynamic = "force-dynamic";

const BlogPage = async () => {
  const data = await client.getList<Blog>({
    endpoint: "blogs",
  });
  return (
    <PageShell>
      <PageHeader title="ブログ記事一覧" />
      <ul className="space-y-4">
        {data.contents.map((blog) => (
          <li key={blog.id}>
            <Link href={`/articles/${blog.id}`}>
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
    </PageShell>
  );
};

export default BlogPage;
