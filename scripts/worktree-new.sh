#!/usr/bin/env bash
# 作業ごとの git worktree を作り、そのまま pnpm dev できる状態にする。
# コードを変える作業は、本体のチェックアウトではなくここで作った worktree で行う（AGENTS.md）。
#
#   pnpm wt:new fix/JUK-40-foo           origin/main から新しいブランチを切る
#   pnpm wt:new fix/JUK-40-foo <起点>    起点のコミットやブランチを指定する
#
# 既にあるブランチ名（ローカルか origin）を渡すと、新しく切らずにそのブランチを取り出す。
#
# やること:
#   1. ../juken-map-worktrees/<ブランチ名の / を - にしたもの> に worktree を作る
#      リポジトリの外に置くのは、中に置くと本体の Vitest（**/*.test.ts）や git status が拾うため。
#   2. gitignore 済みで worktree に来ないもの（.env など）を、本体の実体へのリンクにする
#      ⚠️ リンクなので、worktree で .env を書き換えると本体も変わる。worktree だけの値は
#      .env.worktree に書く（apps/api の dev が .env の後に読み、同じキーを上書きする）。
#   3. 空いている番号 N を選び、.env.worktree にポートを書く
#      Vite = 5173+N、API = 4000+N、E2E = 3010+N（3001 は Grafana なので避ける）
#      DB は docker-compose.yml の name 固定により、本体と同じコンテナを共有する。
#   4. pnpm install
set -euo pipefail

branch="${1:-}"
base="${2:-origin/main}"
if [[ -z "$branch" ]]; then
  echo "使い方: pnpm wt:new <ブランチ名> [起点]   例: pnpm wt:new fix/JUK-40-foo" >&2
  exit 1
fi
if ! git check-ref-format --branch "$branch" >/dev/null 2>&1; then
  echo "ブランチ名として使えません: $branch" >&2
  exit 1
fi

# どの worktree から実行しても、本体のチェックアウトを基準にする。
common_dir="$(git rev-parse --path-format=absolute --git-common-dir)"
main_root="$(dirname "$common_dir")"
dir="${main_root}-worktrees/${branch//\//-}"

if [[ -e "$dir" ]]; then
  echo "既にあります: $dir" >&2
  exit 1
fi

git -C "$main_root" fetch --quiet --prune origin

mkdir -p "$(dirname "$dir")"
if git -C "$main_root" show-ref --verify --quiet "refs/heads/$branch"; then
  git -C "$main_root" worktree add "$dir" "$branch"
elif git -C "$main_root" show-ref --verify --quiet "refs/remotes/origin/$branch"; then
  git -C "$main_root" worktree add --track -b "$branch" "$dir" "origin/$branch"
else
  # --no-track: 起点の origin/main を上流にすると、git pull で main を取り込んでしまう。
  # 上流は最初の git push -u で自分のブランチに設定する。
  git -C "$main_root" worktree add --no-track -b "$branch" "$dir" "$base"
fi

for f in .env .agent-memory .standards .codex .claude/settings.local.json; do
  if [[ -e "$main_root/$f" && ! -e "$dir/$f" ]]; then
    mkdir -p "$(dirname "$dir/$f")"
    ln -s "$main_root/$f" "$dir/$f"
  fi
done

# ほかの worktree が使っている番号と、実際に使用中のポートを避けて N を決める。
port_in_use() { lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1; }
used_slots=" "
while IFS= read -r line; do
  wt="${line#worktree }"
  [[ "$line" == worktree\ * && -f "$wt/.env.worktree" ]] || continue
  slot="$(sed -n 's/^WT_SLOT=//p' "$wt/.env.worktree")"
  used_slots+="$slot "
done < <(git -C "$main_root" worktree list --porcelain)

slot=""
for n in $(seq 1 89); do
  [[ "$used_slots" == *" $n "* ]] && continue
  if port_in_use $((5173 + n)) || port_in_use $((4000 + n)) || port_in_use $((3010 + n)); then
    continue
  fi
  slot="$n"
  break
done
if [[ -z "$slot" ]]; then
  echo "空いているポートの番号が見つかりません。使っていない worktree を pnpm wt:remove で片付けてください。" >&2
  exit 1
fi

web_port=$((5173 + slot))
cat > "$dir/.env.worktree" <<EOF
# scripts/worktree-new.sh が書いた、この worktree だけの値。
# .env は本体へのリンクなので、ここに書いたキーが .env の値を上書きする。
WT_SLOT=$slot
WEB_PORT=$web_port
API_PORT=$((4000 + slot))
E2E_PORT=$((3010 + slot))
BETTER_AUTH_URL=http://localhost:$web_port
EOF

pnpm --dir "$dir" install --frozen-lockfile

cat <<EOF

worktree を作りました。
  場所:     $dir
  ブランチ: $branch
  画面:     http://localhost:$web_port （API $((4000 + slot)) / E2E $((3010 + slot))）

次の手順:
  cd $dir
  pnpm dev        # DB は本体と同じコンテナを使う
  claude          # エージェントもこのディレクトリで起動する

⚠️ Google / GitHub ログインは、OAuth アプリに登録したコールバック URL とポートが合わないため
   この worktree では通らない見込みです。メールとパスワードでログインしてください。
EOF
