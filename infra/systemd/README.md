# 本番 EC2 の systemd の定義

## 毎日の学習通知のタイマー（JUK-85）

朝7時・夜21時（日本時間）に、通知の入口（`POST /api/cron/daily-study-notifications`、Go）を呼ぶ。

| ファイル | 役割 |
| --- | --- |
| `juken-map-daily-notification-morning.timer` | 毎日 07:00（Asia/Tokyo）に `@morning` を起こす |
| `juken-map-daily-notification-evening.timer` | 毎日 21:00（Asia/Tokyo）に `@evening` を起こす |
| `juken-map-daily-notification@.service` | curl で1回呼んで終わる。`%i` が時間帯 |

- 以前は GitHub Actions の schedule で呼んでいたが、毎回2〜4時間遅れ、夜の分が翌日の未明に
  「翌日の夜」として届いていた。GitHub Actions の `daily-study-notifications.yml` は手動の送り直し用に残してある
- このディレクトリが正。デプロイ（`.github/scripts/deploy-ec2.sh`）が `/etc/systemd/system/` へ置き、
  タイマーを有効にする。本番で手で編集しない
- 共有トークンは、デプロイが `.env` の `DAILY_NOTIFICATION_SECRET` から `/etc/juken-map/daily-notification.env`
  （root だけが読める）に書く。無ければタイマーを止める
- EC2 が止まっていて時刻を過ぎた分は、あとから送らない（`Persistent` を付けない。理由はタイマーのコメント）
- 同じ日・同じ時間帯を2回呼んでも、配信記録で2回目は送らない

### 本番で様子を見る（SSM Session Manager で入って）

```sh
systemctl list-timers 'juken-map-*'                              # 次にいつ動くか・前回いつ動いたか
journalctl -u 'juken-map-daily-notification@*' --since today      # 返ってきた件数（sent・failed など）
sudo systemctl start juken-map-daily-notification@evening.service # 手で1回呼ぶ（送り済みなら skipped になる）
```

送った件数は Go のログにも出る（Grafana の `{job="juken-map-api", runtime="go"} |= "daily-notification"`）。
