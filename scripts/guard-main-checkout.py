#!/usr/bin/env python3
"""本体のチェックアウトで main 以外のブランチへ切り替えるコマンドを止める。

Claude Code の PreToolUse(Bash) フック（.claude/settings.json）から呼ばれ、標準入力で
ツール呼び出しの JSON を受け取る。本体は main のまま置いておき、作業は
`pnpm wt:new` で作った worktree で行う決まり（AGENTS.md「作業は worktree で行う」）なので、
本体の HEAD を main 以外へ動かす操作だけを止める。

止めるもの（本体のチェックアウトに対して実行されるとき）:
  git switch <main 以外> / git switch -c ... / git switch -
  git checkout <main 以外のブランチやコミット> / git checkout -b ...
  gh pr checkout ...
止めないもの:
  main へ戻る操作、ファイルの復元（git checkout -- <path>、git checkout <path>）、
  worktree の中での切り替え、別のリポジトリ（.agent-memory など）での操作

コマンドの解釈は `cd <dir>` と `git -C <dir>` までを追う。読み取れない書き方で
すり抜けることはありうる（ルールを思い出させるための仕組みで、完全な強制ではない）。
"""
import json
import os
import re
import shlex
import subprocess
import sys

MAIN_BRANCH = "main"
# switch / checkout で「新しいブランチを作る・HEAD を外す」ことを表すオプション
# （-t / --track はリモートのブランチから同名のブランチを作る）
SWITCH_CREATE = {"-c", "-C", "--create", "--force-create", "--orphan", "-d", "--detach", "-t", "--track"}
CHECKOUT_CREATE = {"-b", "-B", "--orphan", "--detach", "-t", "--track"}
# 値を別の引数で取るオプション（次の引数をブランチ名と取り違えないため）
OPTS_WITH_VALUE = {"--conflict"}


def git(cwd, *args):
    try:
        out = subprocess.run(
            ["git", "-C", cwd, *args], capture_output=True, text=True, timeout=5
        )
    except (OSError, subprocess.TimeoutExpired):
        return None
    return out.stdout.strip() if out.returncode == 0 else None


def is_main_checkout_of_this_repo(path, project_common_dir):
    """path がこのリポジトリの本体のチェックアウト（worktree ではない）なら True。"""
    out = git(path, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
    if not out:
        return False
    git_dir, common_dir = out.splitlines()[:2]
    # worktree の git-dir は <common>/worktrees/<名前> なので、common と一致しない。
    return git_dir == common_dir == project_common_dir


def segments(command):
    """&& ; || | と改行で区切った、コマンドごとの引数の並び。"""
    for part in re.split(r"&&|\|\||[;|\n]", command):
        try:
            tokens = shlex.split(part, comments=True)
        except ValueError:
            continue
        # 先頭の VAR=value を読み飛ばす
        while tokens and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", tokens[0]):
            tokens.pop(0)
        if tokens:
            yield tokens


def switches_away_from_main(sub, args, cwd):
    """git switch / checkout の引数から、main 以外へ移るかを判断する。"""
    if "--" in args or any(a.startswith("--pathspec-from-file") for a in args):
        # checkout <tree-ish> -- <paths> はファイルの復元で、ブランチは変わらない。
        return False
    positional = []
    skip = False
    for a in args:
        if skip:
            skip = False
            continue
        if sub == "switch" and a in SWITCH_CREATE:
            return True
        if sub == "checkout" and a in CHECKOUT_CREATE:
            return True
        if a in OPTS_WITH_VALUE:
            skip = True
            continue
        if a.startswith("-") and a != "-":
            continue
        positional.append(a)
    if not positional:
        return False
    target = positional[0]
    if target == MAIN_BRANCH:
        return False
    if sub == "switch" or target == "-":
        # 「-」は直前にいたブランチへ戻る指定
        return True
    # checkout <x> は、x がコミットとして読めればブランチの切り替え、読めなければファイルの復元。
    return git(cwd, "rev-parse", "--verify", "--quiet", f"{target}^{{commit}}") is not None


def find_violation(command, cwd, project_common_dir):
    here = cwd
    for tokens in segments(command):
        if tokens[0] == "cd" and len(tokens) >= 2:
            here = os.path.normpath(os.path.join(here, os.path.expanduser(tokens[1])))
            continue
        if tokens[:3] == ["gh", "pr", "checkout"]:
            if is_main_checkout_of_this_repo(here, project_common_dir):
                return here
            continue
        if tokens[0] != "git":
            continue
        target_dir = here
        i = 1
        while i < len(tokens) and tokens[i].startswith("-"):
            if tokens[i] == "-C" and i + 1 < len(tokens):
                target_dir = os.path.normpath(os.path.join(target_dir, os.path.expanduser(tokens[i + 1])))
                i += 2
            elif tokens[i] == "-c" and i + 1 < len(tokens):
                i += 2
            else:
                i += 1
        if i >= len(tokens) or tokens[i] not in ("switch", "checkout"):
            continue
        if not is_main_checkout_of_this_repo(target_dir, project_common_dir):
            continue
        if switches_away_from_main(tokens[i], tokens[i + 1 :], target_dir):
            return target_dir
    return None


def main():
    try:
        payload = json.load(sys.stdin)
    except ValueError:
        return
    command = (payload.get("tool_input") or {}).get("command") or ""
    if not re.search(r"\b(checkout|switch)\b", command):
        return
    cwd = payload.get("cwd") or os.getcwd()
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR") or cwd
    project_common_dir = git(project_dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
    if not project_common_dir:
        return
    where = find_violation(command, cwd, project_common_dir)
    if not where:
        return
    reason = (
        f"本体のチェックアウト（{where}）は main のまま置いておく決まりです"
        "（AGENTS.md「作業は worktree で行う」）。main 以外のブランチで作業するときは、"
        "`pnpm wt:new <ブランチ名>` で worktree を作り、その中で操作してください。"
        "既にある worktree は `git worktree list` で確認できます。"
    )
    json.dump(
        {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": reason,
            }
        },
        sys.stdout,
        ensure_ascii=False,
    )


if __name__ == "__main__":
    main()
