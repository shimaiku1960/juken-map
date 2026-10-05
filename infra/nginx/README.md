# nginx（リバースプロキシ）

**この内容は 2026-09-09 に SSM 経由で本番 EC2（`i-0eeb166295363e11d`）から読み取った実物である。**
設定はサーバー上に手で置かれており、これまでリポジトリ管理外だった。フロントエンド／
バックエンド分離（`docs/split-migration-plan.md` の Step 3）でこの設定を書き換えるため、
先に現状を記録する。

## 構成

```
ブラウザ
  ↓ https://juken-map.com（443）
[ nginx 1.28.3（Ubuntu）]  ← EC2 ホスト上。Docker の外
  ↓ http://localhost:3000
[ Docker コンテナ juken-map（Fastify 5＝API＋ビルド済み SPA）]
```

**2026-09-21 更新**：Cloudflare のプロキシ（オレンジ雲）は経路から外れた。DNS は EC2 の
アドレスを直接指しており、応答に `cf-*` ヘッダーは付かない。これに伴い
`cloudflare-realip.conf` を撤去した（下の「注意」を参照）。

- nginx が 80 と 443 を持ち、Docker が 3000 を持つ
- 設定は `/etc/nginx/sites-enabled/default`（`sites-available/default` へのシンボリックリンク）
- Ubuntu の既定ファイルを直接編集した形。コメントアウトされた雛形が大量に残っている
- HTTPS は Let's Encrypt。`certbot.timer` が動いており自動更新されている
  （証明書は `/etc/letsencrypt/live/juken-map.com/`）

## 現在の実質的な設定

雛形コメントを除くと、やっていることは2つだけ。

**443（本体）**

```nginx
server {
    server_name juken-map.com;   # 2026-10-04、www を外した（JUK-23。www は juken-map-www-redirect.conf が受ける）
    root /var/www/html;

    location / {
        proxy_pass http://juken_map_app;   # 2026-09-23、無停止デプロイのため upstream 経由へ
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;   # 2026-09-18 追加
        proxy_cache_bypass $http_upgrade;
    }

    listen [::]:443 ssl ipv6only=on;   # managed by Certbot
    listen 443 ssl;                    # managed by Certbot
    http2 on;                          # 2026-09-19 追加
    ssl_certificate     /etc/letsencrypt/live/juken-map.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/juken-map.com/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
}
```

**80（HTTPS へ寄せるだけ）**

```nginx
server {
    if ($host = www.juken-map.com) { return 301 https://$host$request_uri; }
    if ($host = juken-map.com)     { return 301 https://$host$request_uri; }
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name juken-map.com;   # 2026-10-04、www を外した（JUK-23）。上の www の if はもう通らない
    return 404;
}
```

## www を apex へ寄せる（2026-10-04 追加、JUK-23）

`www.juken-map.com` は、HTTP でも HTTPS でも `https://juken-map.com` へ 301 で送る（パスとクエリはそのまま、転送は1回）。
入口を1つにするのは、ログインの Cookie をホストごとに分けずに済ませるため。Cookie に `__Host-` を付けると
Domain を指定できず、www と apex でログインを共有できない（JUK-115）。

- 転送の server は **このリポジトリの `conf.d/juken-map-www-redirect.conf` が正**で、本番の
  `/etc/nginx/conf.d/juken-map-www-redirect.conf` に手で置いた。デプロイ（`deploy-ec2.sh`）は触らない
- サイト設定（`sites-available/default`）の `server_name` から www を外した（443 と 80 の2か所）。
  外さないと同じ名前の server が2つになり、nginx は警告を出して先に読んだ方（`conf.d` が先）を使う
- 証明書は apex と同じもの（SAN に www も入っている）を使う。www を証明書から外すと、
  `https://www` に来た人に転送の前で証明書のエラーが出るので、**外さない**
- HTTP の www の `return` は、server の直下ではなく `location /` に置いた。証明書の更新は
  `authenticator = nginx` で、certbot が足す `/.well-known/acme-challenge/` の location を先に通す必要がある。
  server の直下の `return` は location より先に効くので、www の確認が転送されて更新が失敗する
- 入れたときは、まず 302 で入れて `curl` で確かめてから 301 にし、`certbot renew --dry-run` で更新が通ることを確かめた。
  変える前のサイト設定は本番の `/etc/nginx/backup-juk23/` に残してある

### 変えるとき・戻すとき

このファイルを直したら、本番へ置き直して `nginx -t` → `systemctl reload nginx` する（SSM で入る）。
戻すときは `/etc/nginx/backup-juk23/default` を `sites-available/default` へ戻し、
`conf.d/juken-map-www-redirect.conf` を消して reload する。
⚠️ 301 はブラウザが長く覚えるので、戻しても一度 www を開いたブラウザは apex へ行き続ける。

## 無停止デプロイ（2026-09-23 追加）

転送先のポートを `conf.d/juken-map-upstream.conf` に切り出し、`sites-available/default` は
その名前を見るだけにした。**このリポジトリの `conf.d/juken-map-upstream.conf` が、その形の控え**
（実際の中身は `.github/scripts/deploy-ec2.sh` がデプロイのたびに書き換える）。

```
[nginx] ──▶ upstream juken_map_app ──▶ 127.0.0.1:3000 か 3001（入れ替わる）
```

**2026-10、JUK-109 で Node のアプリのコンテナを外した。** 今は `juken_map_app` も Go と同じ
127.0.0.1:8080 か 8081 を向く（go-routes.conf の最後の受け皿が先に当たるので、普段は通らない）。
下の流れは、今は Go のコンテナ（`juken-map-go-next` → `juken-map-go`）で行う。最初の1回のデプロイで、
動いていた Node のコンテナ（`juken-map`）を止めて消した。

デプロイの流れ：

1. 空いている方のポートで新しいコンテナ（`juken-map-next`）を起こす
2. `/login` と `/api/health` が 200 を返すのを確かめる **← ここまで利用者は古い方を見ている**
3. upstream ファイルを書き換えて `nginx -t` → `systemctl reload nginx`
4. **5秒待つ**（下の「落とし穴」）
5. 古いコンテナを止め、`juken-map-next` を `juken-map` に改名する

**起動に失敗したら切り替えないだけ**で、本番には何も起きない。以前の「新しいのを入れてから
駄目なら戻す」より安全になった。

### 落とし穴（手元のリハーサルで実際に踏んだもの）

`scripts/rehearse-zero-downtime.sh` が、本番と同じ形を小さく再現して旧方式と新方式を比べる。

- **`nginx -s reload` の直後に古いコンテナを止めると、1件だけ502が出る。**
  reload はすぐ返るが、古いワーカーは処理中の接続を終えるまで「旧設定」で動き続ける＝
  まだ古いコンテナへ転送している。だから手順4の待ちが要る
- **nginx は接続に失敗した転送先を一定時間「死んでいる」と覚える**（`fail_timeout`、既定10秒）。
  実測では、アプリが復帰してから**さらに約6秒**502が続いた。
  つまり**今まで本番で見えていた502の窓は、コンテナが不在だった時間より長い**
- **初回デプロイが無言で落ちた（2026-09-23）。** upstream ファイルを読む行を
  `sed ... "$UPSTREAM_CONF" 2>/dev/null | head -1` と書いていたため、ファイルがまだ無い初回は
  sed が exit 2 を返し、`pipefail` と `set -e` でスクリプトが止まっていた。
  エラーは `2>/dev/null` で捨てていたので何も出なかった。
  **切り替え前に落ちたので本番は無傷**（コンテナも nginx 設定もそのまま）＝設計の意図どおり。
  ポートの判断だけを手元で試せるようにし（`scripts/test-deploy-ports.sh`、CIでも実行）、
  初回の状態を含めて確かめている
- 名前を `juken-map-go` に戻すのは Alloy のため。メトリクス（`juken-map-go:9464`）もログの
  絞り込み（`/juken-map-go`）もコンテナ名で引いている。`docker rename` は Docker の DNS も追随する

リハーサルの結果（起動待ち5秒で比較）：

| 方式 | 結果 |
| --- | --- |
| 旧（同じポートで stop → run） | 53件中 **44件が502** |
| 新（別ポート → 向け替え） | 100件中 **0件** ✅ |

## Go の振り分け（2026-09-29 追加、JUK-72）

一部のパスだけを Go（`apps/api-go`）が返す。どのパスかは **このリポジトリの
`juken-map-go-routes.conf` が正**で、デプロイのたびに本番の `/etc/nginx/juken-map/go-routes.conf` へ置かれる。

```
                         ┌─ go-routes.conf のパス（業務の API・ログイン・/api/health/go）と、
[nginx] 443 の server ───┤  最後の location ~ ^/（それ以外の全部：画面・sitemap・/api/blog・/api/health・/line/settings）
                         │      → upstream juken_map_go  → 127.0.0.1:8080 か 8081（juken-map-go）
                         └─ location /（サイト設定。JUK-111 から何も届かない）
                                → upstream juken_map_app → 127.0.0.1:3000 か 3001（juken-map、Node）
```

- **JUK-111（2026-10）で画面の配信も Go へ移した。** go-routes.conf の最後の `location ~ ^/`（正規表現）が、
  ほかのどの location にも当たらないものを全部 Go へ送る。正規表現の location は、サイト設定の `location /`
  （`^~` の付かない前方一致）より優先されるので、手で置いたサイト設定には触っていない。
  正規表現どうしは書いた順に試すので、この受け皿は必ずファイルの最後に置く。
  戻すときは、この location を消してデプロイする（Node は画面の配信のコードをまだ持っている。消すのは JUK-109）

- 毎日の通知の入口（`POST /api/cron/daily-study-notifications`、JUK-74）も Go が受ける。送り終えるまで時間がかかるので、
  `proxy_read_timeout` を60秒と明示している（Go は50秒で打ち切る）
- 登録の計測（`POST /api/analytics/registration`）・CSP の違反の報告（`POST /api/csp-report`）・
  シミュレーション（`/api/sim/` で始まるパス）も Go が受ける（JUK-80）。Node の sim は JUK-84 で消したので、
  前方一致で全部 Go へ送る
- 管理画面の API（`/api/admin/` で始まるパス、JUK-78：利用者の管理とマスター編集）は書き込みも含めて全部 Go が受ける
- LINE 連携（`/api/line/` で始まるパス、JUK-79）は書き込みも含めて全部 Go が受ける。Webhook の署名は本文のバイト列で
  確かめるので、nginx では本文を書き換えない
- Go の upstream は `conf.d/juken-map-go-upstream.conf`。Node と同じく、デプロイのたびに空いている方へ入れ替わる
- 初回のデプロイで、サイト設定の `location / {` の直前に `include /etc/nginx/juken-map/go-routes.conf;` を
  1行だけ差し込む（差し込む前の設定は `sites-available/default.bak-日時` に残る）。
  `location /` が1つでなければ、どこに入れるか決められないので何もせずに止まる
- `location = /api/dashboard` の完全一致は `location /` より優先されるので、include の位置で結果は変わらない
- 書き込みも Go へ移したパス（`/api/study-logs`・`/api/study-logs/daily`・`/api/study-logs/{id}`・
  `/api/study-plans`・`/api/study-plans/{id}`・`/api/study-plans/{id}/complete`・
  `/api/notification-preferences`・`/api/profile`・`/api/goals`・`/api/goals/{id}`・`/api/goals/first-choice`・
  `/api/textbooks`・`/api/textbooks/{id}`、JUK-75）は、メソッドで分けずにすべて Go へ送る
- 読み取りだけのパス（`/api/textbook-masters`・`/api/universities`・`/api/universities/{id}`、JUK-73）も、
  メソッドで分けずに Go へ送る。GET・HEAD 以外は Go が 404 を返す。
  JUK-84 までは GET・HEAD 以外を `error_page 418 = @node` で Node へ回していたが、Node から消したので外した
- デプロイは Go の新しいコンテナを起こし（JUK-109 から Node のアプリのコンテナは起こさない）、スモークテストが通ったときだけ、
  2つの upstream（どちらも Go の新しいポート）と振り分けをまとめて書き換えて1回だけ reload する。
  `nginx -t` が通らなければ3つとも元に戻す（`scripts/test-deploy-go-routes.sh` で確かめている）

### Node に戻す

Go へ移したパスは、JUK-84 で2回に分けて（9/30 までの分は #317、10/1 の分はその次の PR）すべて Node から消した。
振り分けを外しても Node は 404 を返すだけなので、振り分けだけでは戻らない。
戻すには、JUK-84 のコミットを revert して main に入れ、Node のイメージをデプロイしてから
`juken-map-go-routes.conf` から該当の location を消す。

Go に問題が出たときは、Node に戻すより Go を直して出し直すほうが早い。

## 手元（開発・E2E）でも同じ振り分けを通す（2026-10-01 追加、JUK-96）

開発（`pnpm dev`）と E2E（`scripts/e2e-server.sh`）も、Docker の nginx（`nginx:1.28`）を前に置き、
本番と同じ `juken-map-go-routes.conf` をそのまま読む。それまでは `/api` を全部 Node に送っていて、
E2E が Go の API を一度も通っていなかった。

```
開発  ブラウザ → Vite :5173（画面）→ nginx :4200（/api）→ Go :4100
E2E   ブラウザ → nginx :3000 → Go :4400（SPA も配る）
```

JUK-111 から go-routes.conf の最後の受け皿が残りを全部 Go へ送るので、手元でも Node（:4000・:4300）には何も届かない。

- サイト設定は `local/default.conf.template`。本番のサイト設定と同じ形で、違うのは HTTP であることと転送先だけ
- 起動は `scripts/local-proxy.sh`、ポートは `scripts/local-ports.sh`（worktree の N 番目は上の番号に N を足す）
- コンテナから手元の Go へは `host.docker.internal` で届く（Linux の CI では `--add-host` で作る）
- 振り分けファイルを書き換えたら、開発中は `pnpm dev`（の dev:proxy）を起動し直す

## Step 3 で必要になる変更

現在は `location /` が全部 3000 番へ流している。分離後はパスで振り分ける。

```
                        ┌─ /api/*  → Fastify（:4000）
ブラウザ → nginx ───────┤
                        └─ /*      → 静的 SPA（apps/web の dist）
```

`/api` を先に書いて Fastify へ、`/` は SPA の静的ファイルを返す形になる。SPA はクライアント
ルーティングなので、`try_files $uri /index.html;` で未知のパスも `index.html` に落とす必要がある
（これが無いと `/dashboard` を直接開いたときに 404 になる）。

**切り戻しはこのファイルを元に戻して `nginx -s reload` するだけ**で済む。Step 3 の切り替えが
低リスクなのはこのため。

## 注意

- **`X-Forwarded-For` は 2026-09-18 に追加した。** アプリ（当時は Better Auth、今は Go の `auth_throttle.go`）は
  ログインの回数制限を接続元 IP ごとに数えるが、IP をこのヘッダーからしか読まない。無いと全員が1つの枠で数えられる。
  `$proxy_add_x_forwarded_for`（届いた値に追記）ではなく `$remote_addr` で上書きし、
  利用者が送ってきた値をそのまま信用しないようにしている。変更前の設定は
  `/etc/nginx/sites-available/default.bak-20260918` に残してある
- **`cloudflare-realip.conf` は 2026-09-21 に撤去した。** Cloudflare を経由していた間は、
  `$remote_addr` が利用者ではなく Cloudflare のサーバーの IP になるため、Cloudflare の IP 範囲から
  来た接続に限って `CF-Connecting-IP` を利用者の IP として扱っていた。いまは Cloudflare が経路に
  いないので `$remote_addr` がそのまま利用者の IP である。
  **中継を信頼する設定は、その中継が経路から外れたら一緒に外す。** 残しておくと、nginx が見る
  接続元と実際の接続元が食い違いうる（ログインの回数制限はこの値で数えている）。
  将来ふたたび Cloudflare や ALB を前に置くときは、そのときの経路に合わせて入れ直す
- **この README の設定は現状の記録であって、適用される設定ではない。** 実物はサーバー上にある。
  ただし `conf.d/juken-map-upstream.conf` だけは例外で、デプロイスクリプトが同じ形で書き出す
- `www` → apex の寄せは nginx ではなく Certbot が入れた 301 で行われている。
  `cleanup-after-merges` にある「www→apex 一本化」の検討と関係する
- 既定ファイル（`sites-available/default`）を直接編集しているため、nginx のパッケージ更新時に
  衝突する可能性がある。Step 3 で触るときに、専用ファイルへ分ける価値がある
