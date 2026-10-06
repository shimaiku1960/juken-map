#!/usr/bin/env bash
# openapi/openapi.yaml（API の契約）から、TypeScript と Go の型を作る（JUK-76）。
#
#   bash scripts/openapi-generate.sh           # 両方を作る（pnpm openapi:generate）
#   bash scripts/openapi-generate.sh ts        # TypeScript だけ（src/shared/openapi.gen.ts）
#   bash scripts/openapi-generate.sh go        # Go だけ（apps/api/internal/apischema/openapi.gen.go）
#   bash scripts/openapi-generate.sh --check   # 作り直して、コミット済みのものと違えば失敗する（CI 用）
#   bash scripts/openapi-generate.sh --check ts
#
# 生成物はコミットする（ビルドのたびに作らない）。手で直さず、契約を直してからこのスクリプトを流す。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC="$ROOT/openapi/openapi.yaml"
TS_OUT="$ROOT/src/shared/openapi.gen.ts"
GO_OUT="$ROOT/apps/api/internal/apischema/openapi.gen.go"
# 版はここで固定する（go.mod に道具の依存を足さないため、go run で呼ぶ）。
OAPI_CODEGEN="github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0"

check=false
if [ "${1:-}" = "--check" ]; then check=true; shift; fi
target="${1:-all}"

gen_ts() {
  (cd "$ROOT" && pnpm exec openapi-typescript "$SPEC" --output "$TS_OUT" >/dev/null)
}
gen_go() {
  (cd "$ROOT/apps/api/internal/apischema" && go run "$OAPI_CODEGEN" -config oapi-codegen.yaml "$SPEC")
}

files=()
case "$target" in
  ts) gen_ts; files+=("$TS_OUT") ;;
  go) gen_go; files+=("$GO_OUT") ;;
  all) gen_ts; gen_go; files+=("$TS_OUT" "$GO_OUT") ;;
  *) echo "ts・go のどちらか（省くと両方）を指定してください" >&2; exit 2 ;;
esac

if [ "$check" = true ]; then
  if ! git -C "$ROOT" diff --quiet -- "${files[@]}"; then
    echo "openapi/openapi.yaml から作り直した型が、コミットされているものと違います。" >&2
    echo "pnpm openapi:generate を流して、生成物もコミットしてください。" >&2
    git -C "$ROOT" --no-pager diff --stat -- "${files[@]}" >&2
    exit 1
  fi
  echo "生成物は openapi/openapi.yaml と一致しています。"
fi
