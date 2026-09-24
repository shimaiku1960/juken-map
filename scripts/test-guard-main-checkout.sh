#!/usr/bin/env bash
# 本体のチェックアウトを main 以外へ動かすコマンドを止めるフック（scripts/guard-main-checkout.py）の
# 判定を試す。使い捨てのリポジトリ（本体・worktree・中の別リポジトリ）を作り、その上で流す。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOOK="$ROOT/scripts/guard-main-checkout.py"
WORK="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$WORK"' EXIT

MAIN="$WORK/repo"
WT="$WORK/repo-worktrees/feat-x"
git init -q -b main "$MAIN"
git -C "$MAIN" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
echo x > "$MAIN/file.txt"
git -C "$MAIN" add file.txt
git -C "$MAIN" -c user.name=t -c user.email=t@example.com commit -q -m file
git -C "$MAIN" branch other
git -C "$MAIN" worktree add -q -b feat/x "$WT"
# 本体の中にある別のリポジトリ（.agent-memory のようなもの）
git init -q -b main "$MAIN/.agent-memory"

fail=0
check() {  # $1=期待（deny/allow） $2=作業ディレクトリ $3=コマンド
  local out got
  out="$(jq -nc --arg c "$3" --arg d "$2" '{tool_name:"Bash",tool_input:{command:$c},cwd:$d}' \
    | CLAUDE_PROJECT_DIR="$2" python3 "$HOOK")"
  if [ -n "$out" ]; then got=deny; else got=allow; fi
  if [ "$got" = "$1" ]; then
    printf '  ✅ %-5s %s\n' "$1" "$3"
  else
    printf '  ❌ 期待 %s・実際 %s: %s（%s）\n' "$1" "$got" "$3" "$2"
    fail=1
  fi
}

echo "本体で main 以外へ移る → 止める"
check deny "$MAIN" 'git switch -c fix/JUK-99-x'
check deny "$MAIN" 'git checkout -b fix/JUK-99-x'
check deny "$MAIN" 'git switch other'
check deny "$MAIN" 'git checkout other'
check deny "$MAIN" 'git switch -'
check deny "$MAIN" 'git checkout -'
check deny "$MAIN" 'git checkout HEAD~1'
check deny "$MAIN" 'git checkout -t origin/some-branch'
check deny "$MAIN" 'gh pr checkout 266'
check deny "$MAIN" 'git fetch && git checkout -b x main'
check deny "$MAIN" 'FOO=1 git switch -c x'
check deny "$WT" "cd $MAIN && git switch -c x"
check deny "$WT" "git -C $MAIN checkout -b x"

echo "main へ戻る・ファイルの復元・読み取り → 通す"
check allow "$MAIN" 'git switch main'
check allow "$MAIN" 'git checkout main'
check allow "$MAIN" 'git checkout -- file.txt'
check allow "$MAIN" 'git checkout HEAD -- file.txt'
check allow "$MAIN" 'git checkout file.txt'
check allow "$MAIN" 'git status && git log --oneline -1'
check allow "$MAIN" 'echo "git switch -c x"'
check allow "$MAIN" 'git worktree add ../y -b y'

echo "worktree の中・別のリポジトリ → 通す"
check allow "$WT" 'git switch -c y'
check allow "$WT" 'git checkout other'
check allow "$MAIN" "git -C $WT switch -c y"
check allow "$MAIN" 'git -C .agent-memory switch -c y'
check allow "$WT" "cd $MAIN/.agent-memory && git checkout -b y"

exit "$fail"
