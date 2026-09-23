// ダッシュボード（1画面ぶんをまとめて返す集約API）のキャッシュの住所。
//
// 実績のフックと予定のフックの両方がこれを無効化する必要があるが、useDashboard から
// import すると循環参照になる（useDashboard は両方のフックを使うため）。
// キーだけをこのファイルに置いて、三者が同じものを見られるようにしている。
export const dashboardKey = ["dashboard"] as const;
