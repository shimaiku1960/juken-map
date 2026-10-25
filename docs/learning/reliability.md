# 信頼性

バックエンドの学習計画（`docs/learning/README.md`）の「3. 信頼性」で学んだことのまとめ。回ごとに節を足していく。

## 3-1 タイムアウトの値の関係（2026-10-25）

題材は API の入口から外部 API までの各層のタイムアウトと、止めるときの待ち時間。実験は `apps/api/internal/app/timeout_learn_test.go`（`-tags learn` のときだけ動く）で、本物の `withDeadline` と `requestTimeout` を使い、`httptest` のサーバーに対して流した。DB と本番には触れていない。

### 学んだこと

- **内側ほど短くする。** 内側（ハンドラの ctx の上限）が先に来れば、ハンドラが自分で 500 と理由を書いて返せる。外側（`WriteTimeout`・nginx）が先に来ると、呼び出し側には何も届かず、サーバーのログにも理由が残らない（実験1・2）
- **Go の `WriteTimeout` は「その時刻を過ぎたら書き込みを失敗させる」期限で、その時刻に接続を切るものではない。** ハンドラは期限に気づかずに最後まで仕事をし、書こうとした時点で失敗して接続が閉じる。仕事は成功しているのに、呼び出し側には失敗に見える（実験2）。学習記録の作成なら、保存されたのにエラーが出て、押し直すと2件になる（2-2 の冪等性）
- **`context.WithTimeout(親, d)` の締め切りは、親の締め切りと d 後の早いほう。** 内側に外側より長い値を書いても、黙って外側の値に切り詰められる。エラーも警告も出ない（実験3）
- **`http.Server.Shutdown` が待つのは処理中のリクエスト（接続）だけで、`go f()` で始めた goroutine は待たない。** 応答を返したあとの仕事は、`Shutdown` が戻って `main` が終わった時点で、途中でも消える（実験4）
- **止めるときにも大小関係がある。** `docker stop` の猶予 > `Shutdown` で待つ時間 > 止めるときに残っている仕事にかかる時間、の順でないと、猶予を過ぎたところで SIGKILL が来るか、仕事が途中で消える

### 数字で見た結果

| 実験 | 上限（ctx） | `WriteTimeout` | 仕事にかかる時間 | 呼び出し側に届いたもの |
| -- | -- | -- | -- | -- |
| 1 今の順番 | 10秒（`requestTimeout`） | 15秒 | 12秒 | 10秒後に 500 と `context deadline exceeded` |
| 2 逆の順番 | 20秒 | 15秒 | 18秒 | 18秒後に、ステータスなしで切断（`EOF`）。15秒の時点では何も起きない |

| 実験 | 流したもの | 結果 |
| -- | -- | -- |
| 3 | 上限10秒の `r.Context()` から、AI クイック記録と同じ `parseTimeout` 20秒の ctx を作り、12秒かかる応答を待つ | ctx の締め切りは10秒先。10秒で待つのをやめた |
| 4 | 応答を返したあとに `go f()` で3秒かかる送信を始め、すぐに10秒の `Shutdown` を呼ぶ | `Shutdown` は0秒で戻った。その時点で送信は終わっていない |

### このリポジトリの今

リクエストの経路（外側から内側へ）。

| 層 | 値 | 場所 |
| -- | -- | -- |
| CloudFront → nginx | AWS の既定（応答待ち30秒） | `terraform/cloudfront.tf`（指定なし） |
| nginx → Go | 既定の60秒。毎日の通知だけ60秒と明示 | `infra/nginx/juken-map-go-routes.conf:185` |
| Go：応答を書き終えるまで | `WriteTimeout` 15秒 | `apps/api/internal/app/main.go:196` |
| Go：ヘッダーを読み終えるまで | `ReadHeaderTimeout` 5秒 | `main.go:195` |
| Go：次のリクエストを待つ間 | `IdleTimeout` 60秒 | `main.go:197` |
| Go：1リクエストの上限 | `requestTimeout` 10秒 | `apps/api/internal/app/server.go:18` |
| └ AI クイック記録の解析 | `parseTimeout` 20秒（実際は10秒） | `feature/study/study_log_parse.go:37` |
| └ 外部 API | 5秒 | `httpx/external.go:6` |
| └ ヘルスチェックの DB ping | 2秒 | `app/http.go:25` |
| 毎日の通知（リクエストの上限から切り離す） | 50秒、書き込み期限は60秒 | `feature/notifications/notifications.go:40`・`:456` |

止めるときの経路。

| 層 | 値 | 場所 |
| -- | -- | -- |
| `docker stop` の猶予（過ぎると SIGKILL） | 既定の10秒 | `.github/scripts/deploy-ec2.sh:369` |
| `Shutdown` で待つ時間 | 10秒 | `main.go:258` |
| 応答のあとのメール・後始末 | 1通30秒（`go f()`、待たれない） | `feature/auth/email.go:65`・`handlers.go:467`・`recovery.go:101` |

理由のある大小（守られているもの）。

- `requestTimeout` 10秒 < `WriteTimeout` 15秒：打ち切る前に 500 を返すため（`server.go:16-17` のコメント、実験1・2）
- 外部 API 5秒 < `requestTimeout` 10秒：外部 API が止まっても、ハンドラが自分でエラーを返せる
- `WriteTimeout` 15秒 < CloudFront 30秒 < nginx 60秒：Go が先に諦める
- 毎日の通知は、`SetWriteDeadline` で書き込み期限を 50 + 10 秒に延ばし、ctx をリクエストの上限から切り離している。nginx の60秒とちょうど同じなので、通知が上限いっぱいかかると nginx 側と同時になる

矛盾している組み合わせ（3-2・4 で扱う）。

| 組み合わせ | 起きること | 扱う回 |
| -- | -- | -- |
| `parseTimeout` 20秒 > 外側の `requestTimeout` 10秒 | 20秒待つつもりが10秒で 500。スキーマを変えた直後の初回（約10秒）が落ちうる | 3-2 か別の Issue（ルートごとの上限か、20秒をやめるか） |
| 応答のあとのメール 30秒、`Shutdown` は goroutine を待たない | デプロイの直前に送り始めたメールが、ログも残さず消える | 3-2（止めるときに待つ）、4（outbox） |
| `docker stop` の猶予 10秒 = `Shutdown` 10秒 | `Shutdown` が終わる前に SIGKILL が来うる。猶予を少し長くする | 3-2 |

### 変えたこと

本番のコードは変えていない。実験の道具 `timeout_learn_test.go` を足した。

### 面接で話す要点（STAR）

- **S（状況）**：API は CloudFront → nginx → Go の順に通り、Go の中にも1リクエストの上限・`WriteTimeout`・外部 API の上限がある。止めるときは `docker stop` → `Shutdown` の順で、応答のあとにメールを goroutine で送っている
- **T（課題）**：値がばらばらに決まっていて、大小に理由があるか、逆だと何が起きるかを説明できなかった
- **A（行動）**：全部の値を1枚の表にし、外側から内側へ並べた。本物の `withDeadline` を使って、上限と `WriteTimeout` の大小を入れ替えたときに呼び出し側へ何が届くか、長い上限を内側に書いたらどうなるか、`Shutdown` が応答のあとの仕事を待つかを、ローカルで試した
- **R（結果）**：今の順番では10秒で 500 と理由が返り、逆にすると仕事は成功したのに18秒後に無言で切断された。内側に書いた20秒は黙って10秒に切り詰められ、`Shutdown` は応答のあとのメールを0秒しか待たなかった。「内側ほど短く」「止めるときは猶予 > Shutdown > 残りの仕事」という原則と、その原則に反している3か所を、数字つきで説明できるようになった
