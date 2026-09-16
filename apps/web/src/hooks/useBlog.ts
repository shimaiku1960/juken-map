import { useQuery } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";

// microCMS のキーはサーバーに閉じているので、apps/api の中継を経由する。
export type Blog = {
  id: string;
  title: string;
  description?: string;
  content: string;
  eyecatch?: { url: string; width: number; height: number };
  createdAt: string;
  updatedAt: string;
};

export const blogListKey = ["blog"] as const;
export const blogDetailKey = (id: string) => ["blog", id] as const;

export function useBlogList() {
  return useQuery({
    queryKey: blogListKey,
    queryFn: () =>
      api.get<{ contents: Blog[] }>("/api/blog", {
        fallbackMessage: "記事の取得に失敗しました",
      }),
  });
}

export function useBlogDetail(id: string) {
  return useQuery({
    queryKey: blogDetailKey(id),
    enabled: Boolean(id),
    queryFn: () =>
      api.get<Blog>(`/api/blog/${id}`, {
        fallbackMessage: "記事の取得に失敗しました",
      }),
  });
}
