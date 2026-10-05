# 手元で使うポートを決める（source して使う）。本体のチェックアウトは番号 0、worktree は
# scripts/worktree-new.sh が .env.worktree に書いた WT_SLOT の番号で、ほかのチェックアウトとぶつからないようにずらす。
#
# 開発（pnpm dev）
#   画面   WEB_PORT   5173+N  Vite。/api は PROXY_PORT へ送る
#   振分け PROXY_PORT 4200+N  nginx（本番と同じ振り分けファイル）。全部 GO_PORT へ（JUK-109）
#   Go     GO_PORT    4100+N
# E2E（scripts/e2e-server.sh）
#   振分け E2E_PORT      3000（本体）・3010+N（worktree）  ブラウザが開くのはここ
#   Go     E2E_GO_PORT   4400+N
#
# 環境変数で渡した値と .env.worktree に書いた値は、そちらを優先する（古い worktree の .env.worktree には
# GO_PORT などが無いので、WT_SLOT から決める）。apps/web/vite.config.ts と playwright.config.ts も同じ規則で読む。

_local_ports_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# .env.worktree の値を読む。無ければ空。呼ぶ側は set -e なので、ファイルが無くても失敗を返さない
# （本体のチェックアウトと CI には .env.worktree が無い）。
_local_ports_get() {
  local key=$1 file="$_local_ports_root/.env.worktree"
  if [ -f "$file" ]; then sed -n "s/^$key=//p" "$file" | tail -1; fi
}

WT_SLOT=${WT_SLOT:-$(_local_ports_get WT_SLOT)}
WT_SLOT=${WT_SLOT:-0}
for _key in WEB_PORT GO_PORT PROXY_PORT E2E_PORT E2E_GO_PORT; do
  [ -n "${!_key:-}" ] || printf -v "$_key" '%s' "$(_local_ports_get "$_key")"
done
WEB_PORT=${WEB_PORT:-$((5173 + WT_SLOT))}
GO_PORT=${GO_PORT:-$((4100 + WT_SLOT))}
PROXY_PORT=${PROXY_PORT:-$((4200 + WT_SLOT))}
if [ "$WT_SLOT" = 0 ]; then E2E_PORT=${E2E_PORT:-3000}; else E2E_PORT=${E2E_PORT:-$((3010 + WT_SLOT))}; fi
E2E_GO_PORT=${E2E_GO_PORT:-$((4400 + WT_SLOT))}
unset _key
