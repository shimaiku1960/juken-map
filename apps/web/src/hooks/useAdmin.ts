import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { api } from "@/web/lib/api-client";
import type { AdminOverview, AdminUserList, UserKind } from "@/shared/dto/admin";

export type { AdminOverview, AdminUser, AdminUserList, UserKind } from "@/shared/dto/admin";

// 管理者ページ（/admin）の読み取り。権限が無ければ API が 403 を返し、
// 画面は ApiError.status で「権限がありません」に切り替える。

export const adminOverviewKey = ["admin", "overview"] as const;
export const adminUsersKey = (params: { kind: UserKind; q: string; page: number }) =>
  ["admin", "users", params] as const;

// 403 は何度やり直しても変わらないので、再試行しない。
const noRetryOnForbidden = (failureCount: number, error: unknown) =>
  (error as { status?: number }).status !== 403 && failureCount < 2;

export function useAdminOverview() {
  return useQuery({
    queryKey: adminOverviewKey,
    queryFn: () => api.get<AdminOverview>("/api/admin/overview"),
    retry: noRetryOnForbidden,
  });
}

export function useAdminUsers(params: { kind: UserKind; q: string; page: number }) {
  const search = new URLSearchParams({
    kind: params.kind,
    page: String(params.page),
    ...(params.q ? { q: params.q } : {}),
  });
  return useQuery({
    queryKey: adminUsersKey(params),
    queryFn: () => api.get<AdminUserList>(`/api/admin/users?${search}`),
    retry: noRetryOnForbidden,
    // ページ送り・種別の切り替えの間、前の一覧を出したままにして表がちらつかないようにする。
    placeholderData: keepPreviousData,
  });
}
