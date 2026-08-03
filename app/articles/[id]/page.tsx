import type { Metadata } from "next";
import Image from "next/image";
import { cache } from "react";

import { Card, CardContent } from "@/components/ui/card";
import { client, type Blog } from "@/lib/microcms";
import { SITE_URL } from "@/lib/site";

type ArticlePageProps = {
  params: Promise<{ id: string }>;
};

const getBlog = cache((id: string) =>
  client.get<Blog>({
    endpoint: "blogs",
    contentId: id,
  })
);

const createDescription = (blog: Blog) => {
  if (blog.description?.trim()) return blog.description.trim();

  const plainText = blog.content
    .replace(/<[^>]*>/g, " ")
    .replace(/&nbsp;/g, " ")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/\s+/g, " ")
    .trim();

  return plainText.slice(0, 120);
};

export const generateMetadata = async ({
  params,
}: ArticlePageProps): Promise<Metadata> => {
  const { id } = await params;
  const blog = await getBlog(id);
  const description = createDescription(blog);
  const url = `${SITE_URL}/articles/${id}`;
  const images = blog.eyecatch ? [blog.eyecatch.url] : undefined;

  return {
    title: `${blog.title}｜受験マップ`,
    description,
    alternates: { canonical: url },
    openGraph: {
      title: blog.title,
      description,
      type: "article",
      url,
      images,
      publishedTime: blog.createdAt,
      modifiedTime: blog.updatedAt,
    },
    twitter: {
      card: "summary_large_image",
      title: blog.title,
      description,
      images,
    },
  };
};

const ArticlePage = async ({ params }: ArticlePageProps) => {
  const { id } = await params;
  const blog = await getBlog(id);

  return (
    <main className="mx-auto w-full max-w-3xl p-8">
      <Card>
        <CardContent className="space-y-4">
          <h1 className="text-3xl font-bold">{blog.title}</h1>
          <time className="text-sm text-muted-foreground">
            {new Date(blog.createdAt).toLocaleDateString("ja-JP")}
          </time>
          {blog.eyecatch && (
            <Image
              src={blog.eyecatch.url}
              alt=""
              width={blog.eyecatch.width ?? 1200}
              height={blog.eyecatch.height ?? 630}
              sizes="(max-width: 768px) calc(100vw - 4rem), 704px"
              className="h-auto w-full rounded-lg"
            />
          )}
          <div
            className="prose"
            dangerouslySetInnerHTML={{ __html: blog.content }}
          />
        </CardContent>
      </Card>
    </main>
  );
};

export default ArticlePage;
