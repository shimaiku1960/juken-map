# Terraform で作ったアラートと通知先は、Grafana の画面から編集できなくなる（provenance が付く）。
# 変えるときは、このファイルを直して plan → apply する。
#
# Synthetic Monitoring のフォルダにある2本（証明書の期限・チェックの失敗）は、SM のアプリが作って管理
# しているので、ここでは持たない。

resource "grafana_folder" "juken_map" {
  uid   = "ffh5ls"
  title = "juken-map"
}

resource "grafana_contact_point" "email" {
  name = "juken-map-email"

  # 画面で作った通知先は provenance が無い。これを付けようとすると provider は「作り直し」にするが、
  # ルートの通知ポリシーがこの通知先を使っているので消せない。そこで通知先だけは provenance を付けず、
  # 画面からも編集できるままにする（それでも、変えるときはこのファイルを直す決まりにする）。
  disable_provenance = true

  email {
    addresses    = split(";", var.alert_email_addresses)
    single_email = false
  }
}

resource "grafana_rule_group" "api_production" {
  name             = "api-production"
  folder_uid       = grafana_folder.juken_map.uid
  interval_seconds = 60

  rule {
    name      = "受験マップ API：5xx率が1%超過"
    condition = "C"
    for       = "1m"

    # 閾値は品質基準 05 B2 の 1%（JUK-16、2026-09-29 に 5% から下げた）。本番のリクエストは5分に10件前後
    # しかないので、5xx が1件出るだけで 1% も 5% も超える（直近7日で 5xx は1件）。「件数が少ないときは
    # 鳴らさない」条件は、めったに出ない本物の 5xx を見逃すので付けない。
    # リクエストが無い時間帯は分母が消えて No data になる。それは異常ではないので通知しない。
    no_data_state  = "OK"
    exec_err_state = "Error"

    annotations = {
      description = "本番APIの総リクエストに占める5xx応答の割合を監視します。リクエストが無い時間帯は通知しません。"
      summary     = "直近5分のAPI 5xx率が1%を超えた状態が1分続いています"
    }

    notification_settings {
      contact_point = grafana_contact_point.email.name
    }

    data {
      ref_id         = "A"
      datasource_uid = "grafanacloud-prom"

      relative_time_range {
        from = 600
        to   = 0
      }

      model = jsonencode({
        editorMode    = "code"
        expr          = "(sum(rate(http_requests_total{env=\"production\",status_code=~\"5..\"}[5m])) or vector(0))\n/(sum(rate(http_requests_total{env=\"production\"}[5m])) > 0)"
        instant       = true
        intervalMs    = 1000
        legendFormat  = "__auto"
        maxDataPoints = 43200
        range         = false
        refId         = "A"
      })
    }

    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      query_type     = "expression"

      relative_time_range {
        from = 0
        to   = 0
      }

      model = jsonencode({
        conditions = [{
          evaluator = { params = [0.01], type = "gt" }
          operator  = { type = "and" }
          query     = { params = ["C"] }
          reducer   = { params = [], type = "last" }
          type      = "query"
        }]
        datasource    = { type = "__expr__", uid = "__expr__" }
        expression    = "A"
        intervalMs    = 1000
        maxDataPoints = 43200
        refId         = "C"
        type          = "threshold"
      })
    }
  }

  # Go のコンテナ（juken-map-go）の死活。Synthetic Monitoring は無料枠（月10万回）の残りが足りず
  # /api/health/go の外形監視を足せないため、Alloy が集める up で代える（JUK-72）。
  rule {
    name      = "受験マップ API（Go）：停止"
    condition = "C"

    # デプロイでコンテナを入れ替える間の数十秒は up が落ちる。それでは鳴らさない。
    for = "3m"

    # EC2 ごと止まる・Alloy が止まると up 自体が届かなくなる。それも停止として知らせる。
    no_data_state  = "Alerting"
    exec_err_state = "Error"

    annotations = {
      description = "Alloy が juken-map-go:9464 を取りに行けたか（up）を監視します。メトリクスが届かないときも通知します。Node に戻している間もコンテナは動かしておく前提です。"
      summary     = "本番の Go の API が3分以上応答していません"
    }

    notification_settings {
      contact_point = grafana_contact_point.email.name
    }

    data {
      ref_id         = "A"
      datasource_uid = "grafanacloud-prom"

      relative_time_range {
        from = 300
        to   = 0
      }

      model = jsonencode({
        editorMode    = "code"
        expr          = "max(up{job=\"juken-map-api\",env=\"production\",runtime=\"go\"})"
        instant       = true
        intervalMs    = 1000
        maxDataPoints = 43200
        range         = false
        refId         = "A"
      })
    }

    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      query_type     = "expression"

      relative_time_range {
        from = 0
        to   = 0
      }

      model = jsonencode({
        conditions = [{
          evaluator = { params = [1], type = "lt" }
          operator  = { type = "and" }
          query     = { params = ["C"] }
          reducer   = { params = [], type = "last" }
          type      = "query"
        }]
        datasource    = { type = "__expr__", uid = "__expr__" }
        expression    = "A"
        intervalMs    = 1000
        maxDataPoints = 43200
        refId         = "C"
        type          = "threshold"
      })
    }
  }

  # Resend の送信枠（セキュリティ基準 E1、JUK-94）。枠を使い切ると、本物の利用者の確認メール・再設定メール・
  # 毎日の通知が届かなくなる。アプリは宛先ごと・全体で上限をかけているが（apps/api/src/infra/email-limits.ts）、
  # 枠は毎日の通知（Go）・シミュレーションと分け合うので、残りが減ったら人が見る。
  #
  # 使った数は、Node がメールを送ったときの応答ヘッダーを写したもの（resend_quota_used）。送らないあいだは
  # 古い値が残り続けるので、6時間以内に読んだ値だけを見る（日の枠が戻ったあとも鳴り続けないように）。
  # 分母は無料プランの枠（1日100通・月3,000通）。プランを変えたらここも直す。
  rule {
    name      = "受験マップ：Resend の送信枠の残りが2割を切った"
    condition = "C"
    for       = "0s"

    # メールを送らない時間帯は値が無い。それは異常ではないので通知しない。
    no_data_state  = "OK"
    exec_err_state = "Error"

    annotations = {
      description = "Resend の送信枠（無料プラン：1日100通・月3,000通）のうち、使った割合が8割を超えました。上限に達すると確認メールや再設定メールが届かなくなります。Resend の Usage 画面と、ログの [email-limits] を確かめてください。"
      summary     = "Resend の送信枠の8割を使いました"
    }

    notification_settings {
      contact_point = grafana_contact_point.email.name
    }

    data {
      ref_id         = "A"
      datasource_uid = "grafanacloud-prom"

      relative_time_range {
        from = 600
        to   = 0
      }

      model = jsonencode({
        editorMode    = "code"
        expr          = "max(\n  (\n    resend_quota_used{env=\"production\",period=\"daily\"} / 100\n    or resend_quota_used{env=\"production\",period=\"monthly\"} / 3000\n  )\n  and on(instance, period) (time() - resend_quota_observed_timestamp_seconds{env=\"production\"} < 21600)\n)"
        instant       = true
        intervalMs    = 1000
        maxDataPoints = 43200
        range         = false
        refId         = "A"
      })
    }

    data {
      ref_id         = "C"
      datasource_uid = "__expr__"
      query_type     = "expression"

      relative_time_range {
        from = 0
        to   = 0
      }

      model = jsonencode({
        conditions = [{
          evaluator = { params = [0.8], type = "gt" }
          operator  = { type = "and" }
          query     = { params = ["C"] }
          reducer   = { params = [], type = "last" }
          type      = "query"
        }]
        datasource    = { type = "__expr__", uid = "__expr__" }
        expression    = "A"
        intervalMs    = 1000
        maxDataPoints = 43200
        refId         = "C"
        type          = "threshold"
      })
    }
  }
}
