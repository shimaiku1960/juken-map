# nginx（リバースプロキシ）の現状

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
    server_name juken-map.com www.juken-map.com;
    root /var/www/html;

    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $remote_addr;   # 2026-09-18 追加
        proxy_cache_bypass $http_upgrade;
    }

    listen [::]:443 ssl ipv6only=on;   # managed by Certbot
    listen 443 ssl;                    # managed by Certbot
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
    server_name juken-map.com www.juken-map.com;
    return 404;
}
```

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

- **`X-Forwarded-For` は 2026-09-18 に追加した。** アプリ（Better Auth）はログインの回数制限を
  接続元 IP ごとに数えるが、IP をこのヘッダーからしか読まない。無いと全員が1つの枠で数えられる。
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
- **この README は現状の記録であって、適用される設定ではない。** 実物はサーバー上にある
- `www` → apex の寄せは nginx ではなく Certbot が入れた 301 で行われている。
  `cleanup-after-merges` にある「www→apex 一本化」の検討と関係する
- 既定ファイル（`sites-available/default`）を直接編集しているため、nginx のパッケージ更新時に
  衝突する可能性がある。Step 3 で触るときに、専用ファイルへ分ける価値がある
