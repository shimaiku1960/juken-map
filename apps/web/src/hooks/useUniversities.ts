import { useQuery } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";

// Next.js では Server Component がサービス層を直接呼んでいた部分。
// SPA では HTTP になるため、他のサーバー状態と同じくフックへ集約する。

export type ExploreUniversity = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  faculties: { tags: { name: string }[] }[];
};

export type UniversityDetail = {
  id: number;
  name: string;
  prefecture: string;
  type: string;
  faculties: {
    id: number;
    name: string;
    examDate: string;
    tags: { id: number; name: string }[];
  }[];
};

export const universitiesKey = ["universities"] as const;
export const universityDetailKey = (id: number) =>
  ["universities", id] as const;

export function useUniversities() {
  return useQuery({
    queryKey: universitiesKey,
    queryFn: () =>
      api.get<ExploreUniversity[]>("/api/universities", {
        fallbackMessage: "大学一覧の取得に失敗しました",
      }),
  });
}

export function useUniversityDetail(id: number) {
  return useQuery({
    queryKey: universityDetailKey(id),
    enabled: Number.isInteger(id),
    queryFn: () =>
      api.get<{
        university: UniversityDetail;
        registeredFacultyIds: number[];
      }>(`/api/universities/${id}`, {
        fallbackMessage: "大学情報の取得に失敗しました",
      }),
  });
}
