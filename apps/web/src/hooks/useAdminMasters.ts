import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";
import { universitiesKey } from "@/web/hooks/useUniversities";
import type {
  AdminTag,
  AdminUniversity,
  AdminUniversityDetail,
  AdminUniversityList,
} from "@/shared/dto/admin";
import type { CreateFacultyInput, FacultyInput, UniversityInput } from "@/shared/validations/master";

export type { AdminFaculty, AdminTag, AdminUniversity, AdminUniversityDetail } from "@/shared/dto/admin";

// 管理者ページのマスター編集（/admin/masters）。権限が無ければ API が 403 を返す。

const mastersKey = ["admin", "masters"] as const;
export const adminUniversitiesKey = (params: { q: string; page: number }) =>
  [...mastersKey, "universities", params] as const;
export const adminUniversityKey = (id: number) => [...mastersKey, "university", id] as const;
export const adminTagsKey = [...mastersKey, "tags"] as const;

const noRetryOnForbidden = (failureCount: number, error: unknown) =>
  (error as { status?: number }).status !== 403 && failureCount < 2;

export function useAdminUniversities(params: { q: string; page: number }) {
  const search = new URLSearchParams({ page: String(params.page), ...(params.q ? { q: params.q } : {}) });
  return useQuery({
    queryKey: adminUniversitiesKey(params),
    queryFn: () => api.get<AdminUniversityList>(`/api/admin/universities?${search}`),
    retry: noRetryOnForbidden,
    placeholderData: keepPreviousData,
  });
}

export function useAdminUniversity(id: number | null) {
  return useQuery({
    queryKey: adminUniversityKey(id ?? 0),
    queryFn: () => api.get<AdminUniversityDetail>(`/api/admin/universities/${id}`),
    enabled: id !== null,
    retry: noRetryOnForbidden,
  });
}

export function useAdminTags() {
  return useQuery({
    queryKey: adminTagsKey,
    queryFn: () => api.get<AdminTag[]>("/api/admin/tags"),
    retry: noRetryOnForbidden,
    staleTime: Infinity,
  });
}

// 変更のあとは管理画面の一覧・詳細に加えて、利用者向けの大学一覧（大学を探す）も取り直す。
function useInvalidateMasters() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: mastersKey }),
      queryClient.invalidateQueries({ queryKey: universitiesKey }),
    ]);
}

export function useSaveUniversity() {
  const invalidate = useInvalidateMasters();
  return useMutation({
    mutationFn: ({ id, data }: { id?: number; data: UniversityInput }) =>
      id === undefined
        ? api.post<AdminUniversity>("/api/admin/universities", data, { fallbackMessage: "大学を追加できませんでした" })
        : api.patch<AdminUniversity>(`/api/admin/universities/${id}`, data, {
            fallbackMessage: "大学を保存できませんでした",
          }),
    onSuccess: invalidate,
  });
}

export function useDeleteUniversity() {
  const invalidate = useInvalidateMasters();
  return useMutation({
    mutationFn: (id: number) =>
      api.del<void>(`/api/admin/universities/${id}`, { fallbackMessage: "大学を削除できませんでした" }),
    onSuccess: invalidate,
  });
}

export function useSaveFaculty() {
  const invalidate = useInvalidateMasters();
  return useMutation({
    mutationFn: (input: { id: number; data: FacultyInput } | { id?: undefined; data: CreateFacultyInput }) =>
      input.id === undefined
        ? api.post<unknown>("/api/admin/faculties", input.data, { fallbackMessage: "学部を追加できませんでした" })
        : api.patch<unknown>(`/api/admin/faculties/${input.id}`, input.data, {
            fallbackMessage: "学部を保存できませんでした",
          }),
    onSuccess: invalidate,
  });
}

export function useDeleteFaculty() {
  const invalidate = useInvalidateMasters();
  return useMutation({
    mutationFn: (id: number) =>
      api.del<void>(`/api/admin/faculties/${id}`, { fallbackMessage: "学部を削除できませんでした" }),
    onSuccess: invalidate,
  });
}
