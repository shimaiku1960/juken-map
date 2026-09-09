import { useQuery } from "@tanstack/react-query";

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
    queryFn: async (): Promise<{ contents: Blog[] }> => {
      const res = await fetch("/api/blog");
      if (!res.ok) throw new Error("記事の取得に失敗しました");
      return res.json();
    },
  });
}

export function useBlogDetail(id: string) {
  return useQuery({
    queryKey: blogDetailKey(id),
    enabled: Boolean(id),
    queryFn: async (): Promise<Blog> => {
      const res = await fetch(`/api/blog/${id}`);
      if (!res.ok) throw new Error("記事の取得に失敗しました");
      return res.json();
    },
  });
}
