#!/usr/bin/env bash
# 毎日の通知送信（POST /api/cron/daily-study-notifications）を Node と Go で1回ずつ流し、
# 同じ宛先・同じ本文を送るか、送り終わるまでに何秒かかるかを比べる（JUK-74）。
#
# 本物のメールは送らない。Resend の代わりに、手元に偽のサーバー（決まった時間だけ待ってから 200 を返す）を立て、
# Node（Resend の SDK は RESEND_BASE_URL を読む）と Go の両方をそこへ向ける。API キーもダミーにする。
#
# 使い方（worktree のルートから）:
#   bash apps/api-go/compare-notifications.sh
#   N=200 LATENCY_MS=400 bash apps/api-go/compare-notifications.sh
#
# ⚠️ 手元の DB の通知設定を一時的に書き換える（合成ユーザー N 人だけ朝のメールをオン、ほかは全部オフ）。
#    終わったら元に戻す（途中で止めても trap で戻す）。DB は全 worktree で共有なので、流している間は
#    ほかの worktree で通知を送らないこと。
# 前提: DB が起動していて、合成データ（pnpm db:seed:synthetic）が入っていること。
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
N=${N:-100}                  # 送る人数（合成ユーザー）
LATENCY_MS=${LATENCY_MS:-300} # 偽の Resend が1通ごとに待つ時間。本物の Resend の応答はおおむね 200〜400ms
NODE_PORT=${NODE_PORT:-18100}
GO_PORT=${GO_PORT:-18180}
FAKE_PORT=${FAKE_PORT:-18190}
SECRET=compare-notifications
WORK=$(mktemp -d)
BACKUP=_juk74_notification_backup

source "$ROOT/apps/api-go/servers.sh"

sql() { (cd "$ROOT" && bash scripts/db-shell.sh -N -B -e "$1"); }

restore() {
  # 偽のサーバーを先に止める。stop_servers の wait は残っている子プロセスすべてを待つので、
  # 後にすると偽のサーバーが終わるのを待ち続けて止まる。
  [ -n "${fake_pid:-}" ] && kill "$fake_pid" 2>/dev/null || true
  stop_servers 2>/dev/null
  if [ -n "${max_delivery_id:-}" ]; then
    sql "DELETE FROM NotificationDelivery WHERE id > $max_delivery_id" || true
  fi
  if [ "${backed_up:-}" = 1 ]; then
    sql "UPDATE NotificationPreference AS np JOIN $BACKUP AS b ON b.id = np.id
         SET np.morningEnabled = b.morningEnabled, np.eveningEnabled = b.eveningEnabled,
             np.lineMorningEnabled = b.lineMorningEnabled, np.lineEveningEnabled = b.lineEveningEnabled;
         DROP TABLE $BACKUP" && echo "通知設定を元に戻しました。"
  fi
  rm -rf "$WORK"
}
trap restore EXIT

ensure_free "$NODE_PORT" "$GO_PORT" "$FAKE_PORT"

# 前回の実行が戻しきれずに終わっていたら、上書きせずに止める（戻す元が消えるため）。
if [ -n "$(sql "SHOW TABLES LIKE '$BACKUP'")" ]; then
  echo "$BACKUP が残っています。前回の実行の戻しが終わっていません。中身を確かめてから戻してください。" >&2
  exit 1
fi

# 偽の Resend。受け取った本文を1行ずつ書き出し、LATENCY_MS 待ってから返す。同時に来ても順に書く。
cat >"$WORK/fake_resend.py" <<'PY'
import json, sys, threading, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
port, latency, out = int(sys.argv[1]), int(sys.argv[2]) / 1000, sys.argv[3]
lock = threading.Lock()
class H(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        time.sleep(latency)
        with lock, open(out, "a") as f:
            f.write(json.dumps(json.loads(body), ensure_ascii=False) + "\n")
        res = b'{"id":"fake"}'
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(res)))
        self.end_headers()
        self.wfile.write(res)
    def log_message(self, *args):
        pass
ThreadingHTTPServer(("127.0.0.1", port), H).serve_forever()
PY
python3 "$WORK/fake_resend.py" "$FAKE_PORT" "$LATENCY_MS" "$WORK/sent.jsonl" &
fake_pid=$!

# 通知設定を退避し、合成ユーザー N 人だけ朝のメールをオンにする。
sql "CREATE TABLE $BACKUP AS
     SELECT id, morningEnabled, eveningEnabled, lineMorningEnabled, lineEveningEnabled FROM NotificationPreference"
backed_up=1
max_delivery_id=$(sql "SELECT COALESCE(MAX(id), 0) FROM NotificationDelivery")
sql "UPDATE NotificationPreference
     SET morningEnabled = FALSE, eveningEnabled = FALSE, lineMorningEnabled = FALSE, lineEveningEnabled = FALSE;
     UPDATE NotificationPreference SET morningEnabled = TRUE WHERE userId IN (
       SELECT id FROM (
         SELECT u.id FROM \`user\` AS u JOIN NotificationPreference AS np ON np.userId = u.id
         WHERE u.email LIKE '%@synthetic.juken-map.invalid' ORDER BY u.id LIMIT $N
       ) AS picked)"

# 送信先を偽のサーバーへ向け、キーはダミーにする。Node は --env-file より先に入っている値を優先する。
export RESEND_BASE_URL=http://127.0.0.1:$FAKE_PORT RESEND_API_KEY=re_dummy DAILY_NOTIFICATION_SECRET=$SECRET
export LINE_CHANNEL_ACCESS_TOKEN=
# Go は .env を読み込むので、上の値を .env の後ろに足したファイルを渡す（後に書いた方が勝つ）。
{ cat "$ROOT/.env"; printf 'RESEND_BASE_URL=%s\nRESEND_API_KEY=re_dummy\nDAILY_NOTIFICATION_SECRET=%s\nLINE_CHANNEL_ACCESS_TOKEN=\n' \
  "$RESEND_BASE_URL" "$SECRET"; } >"$WORK/go.env"
GO_ENV_FILE="$WORK/go.env" start_servers "$NODE_PORT" "$GO_PORT"

# run 名前 ポート → 1回送り、かかった秒数と件数を出す。送った本文は WORK/名前.jsonl に移す。
run() {
  local name=$1 port=$2 started ended summary
  : >"$WORK/sent.jsonl"
  started=$(python3 -c 'import time; print(time.time())')
  summary=$(curl -s --max-time 120 -X POST -H "Authorization: Bearer $SECRET" -H 'Content-Type: application/json' \
    --data '{"slot":"morning"}' "localhost:$port/api/cron/daily-study-notifications")
  ended=$(python3 -c 'import time; print(time.time())')
  mv "$WORK/sent.jsonl" "$WORK/$name.jsonl"
  printf '%s\t%.1f秒\t%s\n' "$name" "$(python3 -c "print($ended - $started)")" "$summary"
  # 次の実行も同じ人へ送れるよう、今回の配信記録を消す。
  sql "DELETE FROM NotificationDelivery WHERE id > $max_delivery_id"
}

echo "合成ユーザー ${N}人に朝のメールを送ります（偽の Resend は1通 ${LATENCY_MS}ms）。"
printf 'target\ttime\tsummary\n'
run node "$NODE_PORT"
run go "$GO_PORT"

# 宛先ごとに並べて、送った中身（送り主・宛先・件名・本文）が同じかを比べる。
normalize() { jq -S -c '{from, to, subject, html, text}' "$1" | sort; }
if diff <(normalize "$WORK/node.jsonl") <(normalize "$WORK/go.jsonl") >"$WORK/diff.txt"; then
  echo "✅ $(wc -l <"$WORK/go.jsonl" | tr -d ' ')通とも、宛先・件名・本文が Node と同じ"
else
  echo "❌ Node と Go で送った中身が違う（先頭20行）:" >&2
  head -20 "$WORK/diff.txt" >&2
  exit 1
fi
