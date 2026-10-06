# 外部へ送っているデータ

受験マップが外部のサービスへ送っているデータを、送信先ごとに並べる（開発基準 06 G5）。
送った先で漏れても困らないよう、送るものを先に決め、要らないものは送らない。

- **見直した日：2026-10-05**（JUK-124。コードと本番の通信を確かめた）
- **道具を足すとき・送るものを変えるとき**は、先にこの表に行を足してから実装し、見直した日を書き換える。
  プライバシーポリシー（`apps/web/src/pages/PrivacyPage.tsx` の「外部サービスの利用」）も合わせる
- 利用者を直接特定する情報＝メールアドレス・表示名・LINE のユーザー ID。学習の中身＝予定・実績・教材名・志望校

## 一覧

| 送信先 | 送る経路 | 送る項目 | 利用者を特定する情報・学習の中身 | 送る理由 |
| --- | --- | --- | --- | --- |
| Google Analytics 4 | ブラウザ（gtag.js） | ページの URL とタイトル、参照元、ブラウザ・端末・言語、GA が付ける閲覧者の ID。イベント：`signup_cta_click`（ボタンの位置）、`signup_method_submit`・`sign_up`（登録の方法）、`study_log_created`・`first_study_log_created`（記録の入口：手入力・タイマー・予定） | 送らない（利用者の ID も送らない）。URL に載るのは大学の ID（`/explore/:id`）まで | 登録と記録に至る流れの分析 |
| Grafana Cloud（Faro） | ブラウザ（`apps/web/src/lib/faro.ts`） | 画面で起きたエラー（メッセージ・スタック）、`console` の info・warn・error、Web Vitals、読み込んだ通信の URL と時間、ページの URL、ブラウザ・端末、Faro が付けるセッションの ID | 送らない（利用者の ID も送らない）。URL のトークンは送る前に伏せる | 画面のエラーと表示速度の監視 |
| Grafana Cloud（Loki） | サーバー（Alloy が Go のコンテナの標準出力を読む） | リクエストのメソッド・パス（クエリは含めない）・状態コード・時間。認証の出来事（`[auth] …`）に利用者の ID・IP アドレス・User-Agent・失敗の種類。メール送信の上限に当たった宛先は SHA-256 の先頭12文字だけ | 利用者の ID（ランダムな文字列）・IP アドレスを含む。メールアドレス・学習の中身は出さない | 障害と不正の検知・調査（06 H1・H4） |
| Grafana Cloud（Prometheus） | サーバー（Alloy が Go の `/metrics` を読む） | ルートのひな形・メソッド・状態コードごとの件数と時間、メールの送信数、Resend の送信枠の使用数 | 含まない | 監視とアラート |
| Resend | サーバー（`auth_email.go`・`internal/feature/notifications/notifications.go`） | 宛先のメールアドレス、件名、本文。本文は、確認・再設定のリンク（トークンを含む）、アカウントの変更のお知らせ、学習通知（表示名・その日の予定の内容と教材名・学習時間）。運営者への新規登録の知らせ（新しい利用者の表示名とメールアドレス。宛先は運営者） | 含む（メールを届けるのに要る） | メールの送信 |
| LINE（Messaging API） | サーバー（`internal/feature/line/api.go`・`internal/feature/notifications/notifications.go`） | 宛先の LINE のユーザー ID、本文。本文は、連携の案内と完了の知らせ、学習通知（表示名・予定の内容と教材名・学習時間） | 含む（利用者が LINE 通知を選んだときだけ） | LINE への通知 |
| LINE（LINE Login） | サーバー（`internal/feature/line/api.go`） | 認可コード・ID トークンの確認、友だち追加の状態の問い合わせ。受け取るのは LINE のユーザー ID（scope は `openid profile`） | 送るのは LINE が発行した値だけ | LINE アカウントの連携 |
| Google・GitHub（OAuth） | サーバー（`auth_oauth.go`）とブラウザのリダイレクト | 認可コード・クライアントの ID と秘密・戻り先の URL。受け取るのは ID とメールアドレス（scope は Google が `openid email`、GitHub が `user:email`） | 送るのは各社が発行した値だけ | 外部アカウントでのログイン |
| microCMS | サーバー（`blog.go`） | API キー、記事の ID と取得条件 | 含まない | ブログ記事の取得 |
| microCMS（画像の CDN）・Google Fonts | ブラウザ | 画像・フォントの取得（IP アドレス、User-Agent、Referer は自分のオリジンだけ） | 含まない | 記事の画像とフォントの表示 |
| GitHub（API） | サーバー（`microcms_webhook.go`） | デプロイのワークフローを動かす要求（ブランチ名だけ） | 含まない | 記事を公開したときの作り直し |
| Amazon Web Services（東京） | — | アプリと DB（EC2・RDS）、秘密情報（Secrets Manager）、コンテナのイメージ（ECR） | すべてのデータを置く | 本サービスの運用 |

## 伏せ方と、送らないと決めたもの

- **URL のトークン**（`?token=…`・`?linkToken=…`）。メールや LINE のリンクで開く画面は、読み込んだらすぐ
  URL から消す（`useTokenFromLink.ts`）。消す前に GA4 が URL を読まないよう、URL にトークンがあるときは
  最初の page_view を送らない（`apps/api/seo.go` の `analyticsInlineScript`）。Faro は送る前に、
  エンコードされた形（`linkToken%3D…`）も含めて伏せる（`redactSecrets`）
- **ログインの戻り先**（`/login?callbackURL=…`）にトークンを載せない。LINE 連携は同じタブの sessionStorage で
  持ち回る（`useLineLinkToken`）
- **GA4・Faro に利用者の ID を渡さない**（`setUser`・`user_id` を使わない）。分析は閲覧者の単位で足りる
- **リクエストのログにクエリを含めない**（`middleware.go` は `r.URL.Path` だけを書く）。nginx のアクセスログは
  EC2 から外へ送らない（Alloy が読むのは Go のコンテナだけ）
- **認証の失敗の記録にメールアドレスを書かない**（利用者の ID と IP アドレスで追う）

## 見直しで見つけて直したこと（2026-10-05）

- LINE 連携の画面（`/line/link?linkToken=…`）が URL からトークンを消しておらず、GA4 のページの URL に
  `linkToken` が載っていた。未ログインのときはログインの戻り先（`callbackURL`）にも載っていた。Faro も、
  読み込んだ GA4 への送信の URL の中にエンコードされた形で含んでいた。`linkToken` は LINE が発行する
  10分・1回きりの値で、GA4 を見られるのは運営者だけだが、送る理由が無いので止めた
- メールのリンクの画面（再設定・確認）は、本番の通信では GA4 に届く前に URL から消えていたが、
  順番に頼っていたので最初の page_view を送らないようにした。この2画面と LINE 連携の画面の page_view は
  GA4 に残らなくなる（登録の計測は `sign_up` で取っている）
- プライバシーポリシーに Grafana Labs が無く、Resend の用途が確認・再設定だけになっていたので直した

## 残っていること

- サーバーのエラーのログ（`err.Error()`）に、DB のエラーの文言として入力の値が入りうる（一意の制約に当たったときなど）。
  漏えい経路として 06 G4 で扱う
