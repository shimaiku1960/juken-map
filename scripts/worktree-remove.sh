#!/usr/bin/env bash
# pnpm wt:new で作った worktree を片付ける。作業内容を失う操作はしない。
#
#   pnpm wt:remove fix/JUK-40-foo      ブランチ名で指定する
#
# 1. 未コミットの変更・未追跡ファイル・想定外の gitignore 済みファイルがあれば止まる
#    （.env のようなファイルは git status に出ないので、ここで見る）。
# 2. worktree のディレクトリを消す。
# 3. ブランチは、作業が main に入っているとき（PR がマージ済みでローカルの先端と一致する、
#    または main に無いコミットが1つも無い）だけ消す。そうでなければ残して知らせる。
#    worktree を消してもコミットはブランチに残るので、あとで pnpm wt:new <同じ名前> で戻れる。
set -euo pipefail

branch="${1:-}"
if [[ -z "$branch" ]]; then
  echo "使い方: pnpm wt:remove <ブランチ名>" >&2
  git worktree list >&2
  exit 1
fi

common_dir="$(git rev-parse --path-format=absolute --git-common-dir)"
main_root="$(dirname "$common_dir")"

dir=""
current=""
while IFS= read -r line; do
  case "$line" in
    worktree\ *) current="${line#worktree }" ;;
    "branch refs/heads/$branch") dir="$current" ;;
  esac
done < <(git -C "$main_root" worktree list --porcelain)

if [[ -z "$dir" ]]; then
  echo "このブランチを取り出している worktree がありません: $branch" >&2
  git -C "$main_root" worktree list >&2
  exit 1
fi
if [[ "$dir" == "$main_root" ]]; then
  echo "本体のチェックアウトは消せません: $dir" >&2
  exit 1
fi

# 1. 消してよい状態か確かめる
dirty="$(git -C "$dir" status --porcelain --untracked-files=all)"
if [[ -n "$dirty" ]]; then
  echo "未コミットの変更か未追跡ファイルがあるため止めました（${dir}）:" >&2
  echo "$dirty" >&2
  exit 1
fi

# gitignore 済みのファイルのうち、作り直せるもの（依存・ビルド成果物・テスト結果）と
# wt:new が置いたリンク以外が残っていたら止まる。負荷試験の結果などを消さないため。
unexpected=""
while IFS= read -r path; do
  case "$path" in
    node_modules/ | apps/*/node_modules/ | apps/web/dist/ | test-results/ | playwright-report/ | \
      blob-report/ | e2e/.auth/ | sim/.state/ | logs/ | *.tsbuildinfo | .env.worktree | .DS_Store | */.DS_Store)
      continue ;;
    .env | .agent-memory | .standards | .codex | .claude/settings.local.json)
      [[ -L "$dir/$path" ]] && continue ;;
    # 中身が settings.local.json のリンクだけだと、ディレクトリごと ignore 済みとして出てくる。
    .claude/)
      if [[ -L "$dir/.claude/settings.local.json" &&
        -z "$(find "$dir/.claude" -mindepth 1 ! -path "$dir/.claude/settings.local.json" -print -quit)" ]]; then
        continue
      fi ;;
  esac
  unexpected+="  $path"$'\n'
done < <(git -C "$dir" ls-files --others --ignored --exclude-standard --directory)
if [[ -n "$unexpected" ]]; then
  echo "gitignore 済みのファイルが残っているため止めました。中身を確認して、要らなければ消してから再実行してください:" >&2
  printf '%s' "$unexpected" >&2
  exit 1
fi

# 2. worktree を消す（ignore 済みのファイルは git worktree remove が残すことがあるので、確認済みの上で消す）
case "$PWD/" in
  "$dir"/*) echo "⚠️ 今いるディレクトリ（${PWD}）を消します。終わったら $main_root へ移動してください。" >&2 ;;
esac
git -C "$main_root" worktree remove --force "$dir"
rm -rf "$dir"
git -C "$main_root" worktree prune
echo "worktree を消しました: $dir"

# 3. ブランチを消してよいか確かめる
git -C "$main_root" fetch --quiet --prune origin
tip="$(git -C "$main_root" rev-parse "refs/heads/$branch")"
if [[ -z "$(git -C "$main_root" rev-list origin/main.."$branch")" ]]; then
  git -C "$main_root" branch -D "$branch" >/dev/null
  echo "ブランチを消しました: ${branch}（main に無いコミットはありません）"
  exit 0
fi

# このリポジトリは squash マージなので、マージ後もブランチのコミットは main の祖先にならない。
# PR がマージ済みで、その PR の先端とローカルの先端が同じなら、作業はすべて main に入っている。
merged_head=""
if command -v gh >/dev/null 2>&1; then
  merged_head="$(cd "$main_root" && gh pr list --head "$branch" --state merged --limit 1 \
    --json headRefOid --jq '.[0].headRefOid // ""' 2>/dev/null || true)"
fi
if [[ -n "$merged_head" && "$merged_head" == "$tip" ]]; then
  git -C "$main_root" branch -D "$branch" >/dev/null
  echo "ブランチを消しました: ${branch}（PR はマージ済み）"
else
  echo "ブランチは残しました: ${branch}（main に入っていないコミットがあります。続けるときは pnpm wt:new ${branch}）"
fi
