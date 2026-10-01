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
# 本体の外にある実体へのリンク（ローカルの .agent-memory はこの形）
git init -q -b main "$WORK/memory"
ln -s "$WORK/memory" "$MAIN/linked-memory"
# gitignore 済みのファイル（.env のようなもの）
echo '.env' > "$MAIN/.gitignore"

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

check_edit() {  # $1=期待（deny/allow） $2=作業ディレクトリ $3=ツール名 $4=書き換えるファイル
  local out got key=file_path
  [ "$3" = NotebookEdit ] && key=notebook_path
  out="$(jq -nc --arg t "$3" --arg k "$key" --arg p "$4" --arg d "$2" \
    '{tool_name:$t,tool_input:{($k):$p},cwd:$d}' \
    | CLAUDE_PROJECT_DIR="$2" python3 "$HOOK")"
  if [ -n "$out" ]; then got=deny; else got=allow; fi
  if [ "$got" = "$1" ]; then
    printf '  ✅ %-5s %s %s\n' "$1" "$3" "${4#"$WORK"/}"
  else
    printf '  ❌ 期待 %s・実際 %s: %s %s（%s）\n' "$1" "$got" "$3" "$4" "$2"
    fail=1
  fi
}

echo "本体のファイルを書き換える → 止める"
check_edit deny "$MAIN" Edit "$MAIN/file.txt"
check_edit deny "$MAIN" Write "$MAIN/new-dir/new.txt"
check_edit deny "$MAIN" NotebookEdit "$MAIN/note.ipynb"
check_edit deny "$MAIN" Edit file.txt
check_edit deny "$WT" Edit "$MAIN/file.txt"

echo "worktree・別のリポジトリ・gitignore 済み・リポジトリの外 → 通す"
check_edit allow "$MAIN" Edit "$WT/file.txt"
check_edit allow "$WT" Write "$WT/new.txt"
check_edit allow "$MAIN" Edit "$MAIN/.agent-memory/MEMORY.md"
check_edit allow "$MAIN" Write "$MAIN/linked-memory/topic.md"
check_edit allow "$MAIN" Edit "$MAIN/.env"
check_edit allow "$MAIN" Write "$WORK/outside.txt"

exit "$fail"
