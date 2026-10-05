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
  # 毎日の通知が届かなくなる。アプリは宛先ごと・全体で上限をかけているが（apps/api/auth_email.go）、
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

# 攻撃の兆候と監視の途絶（セキュリティ基準 H1・H3、JUK-98）。既存の api-production とはグループを分け、
# 同じ形のルールを下の表から作る。通知先は同じ juken-map-email 1つに集める。
#
# H1 のしきい値は、本番の平常時の実測（2026-09-24〜10-01 の7日）から決めた。
#   401：1時間に最大5件／403：最大7件／429：最大2件／管理画面の書き込み：0件
# どれも「15分でその何倍か」を超えたら鳴らす。攻撃が無いときに鳴らない高さで、総当たりや一括操作なら超える。
# 件数が無い時間帯はデータが無い（No data）。それは異常ではないので鳴らさない（止まったことは H3 で見る）。
#
# H3 は「データが無い」を異常として鳴らす（no_data_state = Alerting）。Node には死活監視（Synthetic
# Monitoring）が30秒ごとに来るので、10分にリクエストが19件・ログが18行を下回ったことは無い。
# 0 になったら、アプリか Alloy（集めて送る役）か Grafana Cloud までの経路のどこかが止まっている。
locals {
  security_signal_rules = {
    auth_failures = {
      name        = "受験マップ：認証失敗（401）の急増"
      summary     = "15分間の 401 が20件を超えました"
      description = "ログインの総当たりや、盗んだ Cookie の使い回しの兆候です。平常時は1時間に5件以下です。route 別の内訳と、ログの [auth] sign_in_failure（reason・ip）を確かめてください。手順は docs/incident-response.md。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(http_requests_total{env=\"production\",status_code=\"401\"}[15m]))"
      op          = "gt"
      threshold   = 20
      for         = "0s"
      no_data     = "OK"
    }
    forbidden = {
      name        = "受験マップ：権限なし（403）の急増"
      summary     = "15分間の 403 が20件を超えました"
      description = "他人のデータや管理機能を探っている兆候です。平常時は1時間に7件以下（デモアカウントの書き込み）です。route 別の内訳を確かめてください。手順は docs/incident-response.md。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(http_requests_total{env=\"production\",status_code=\"403\"}[15m]))"
      op          = "gt"
      threshold   = 20
      for         = "0s"
      no_data     = "OK"
    }
    rate_limited = {
      name        = "受験マップ：回数制限（429）の急増"
      summary     = "15分間の 429 が10件を超えました"
      description = "ログインやメール送信の連打、またはログインした人の API の叩きすぎで、回数制限に当たっています。平常時は1時間に2件以下です。ログの TOO_MANY_SIGN_IN_ATTEMPTS と IP、request limited: per user の userId を確かめてください。手順は docs/incident-response.md。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(http_requests_total{env=\"production\",status_code=\"429\"}[15m]))"
      op          = "gt"
      threshold   = 10
      for         = "0s"
      no_data     = "OK"
    }
    admin_writes = {
      name        = "受験マップ：管理画面の書き込みの急増"
      summary     = "15分間の管理画面の書き込みが15件を超えました"
      description = "管理者アカウントが乗っ取られ、一括で削除・停止されている兆候です。平常時は0件です。管理操作の監査ログを確かめてください。手順は docs/incident-response.md。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(http_requests_total{env=\"production\",route=~\"/api/admin/.*\",method!~\"GET|HEAD\"}[15m]))"
      op          = "gt"
      threshold   = 15
      for         = "0s"
      no_data     = "OK"
    }
    email_volume = {
      name        = "受験マップ：メール送信の急増"
      summary     = "1時間に送ったメールが20通を超えました"
      description = "登録や再設定の連打で、Resend の枠（1日100通）を使い切られる兆候です。アプリ全体の上限は24時間80通です。email_sends_total の kind 別の内訳を確かめてください。手順は docs/incident-response.md。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(email_sends_total{env=\"production\",result=\"sent\"}[1h]))"
      op          = "gt"
      threshold   = 20
      for         = "0s"
      no_data     = "OK"
    }
    email_blocked = {
      name        = "受験マップ：メール送信を上限で止めた"
      summary     = "宛先ごとか全体の上限で、メールを送らなかったものがあります"
      description = "同じ宛先への連続送信か、全体の上限（24時間80通）に当たりました。ログの [email-limits] で reason（recipient・global）と宛先のハッシュを確かめてください。手順は docs/incident-response.md。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(email_sends_total{env=\"production\",result=\"blocked\"}[1h]))"
      op          = "gt"
      threshold   = 0
      for         = "0s"
      no_data     = "OK"
    }
    # 監視の途絶の2本は runtime で絞らない。死活監視（30秒ごとの /api/health）を受けるのが Node から Go に変わった
    # （JUK-111）ため、片方だけを見ると、切り替えの前か後のどちらかで誤って鳴る。Go が止まったことは「受験マップ API（Go）：停止」が受け持つ。
    metrics_gap = {
      name        = "受験マップ：監視の途絶（メトリクスが届かない）"
      summary     = "API のメトリクスが10分間届いていません"
      description = "アプリ・Alloy・Grafana Cloud までの経路のどこかが止まっています。検知を黙らせるために止められた可能性もあります。EC2 で docker ps を見て、juken-map-go と alloy が動いているかを確かめてください。"
      datasource  = "grafanacloud-prom"
      expr        = "sum(increase(http_requests_total{env=\"production\"}[10m]))"
      op          = "lt"
      threshold   = 1
      for         = "5m"
      no_data     = "Alerting"
    }
    logs_gap = {
      name        = "受験マップ：監視の途絶（ログが届かない）"
      summary     = "API のログが15分間届いていません"
      description = "アプリ・Alloy・Grafana Cloud までの経路のどこかが止まっています。検知を黙らせるために止められた可能性もあります。EC2 で docker ps を見て、juken-map-go と alloy が動いているかを確かめてください。"
      datasource  = "grafanacloud-logs"
      expr        = "sum(count_over_time({job=\"juken-map-api\", env=\"production\"}[15m]))"
      op          = "lt"
      threshold   = 1
      for         = "5m"
      no_data     = "Alerting"
    }
  }
}

resource "grafana_rule_group" "security_signals" {
  name             = "security-signals"
  folder_uid       = grafana_folder.juken_map.uid
  interval_seconds = 60

  dynamic "rule" {
    for_each = local.security_signal_rules

    content {
      name      = rule.value.name
      condition = "C"
      for       = rule.value.for

      no_data_state  = rule.value.no_data
      exec_err_state = "Error"

      annotations = {
        summary     = rule.value.summary
        description = rule.value.description
      }

      notification_settings {
        contact_point = grafana_contact_point.email.name
      }

      data {
        ref_id         = "A"
        datasource_uid = rule.value.datasource
        # model の queryType から Grafana が付ける。書かないと plan に毎回消す差分が出る
        query_type = "instant"

        relative_time_range {
          from = 3600
          to   = 0
        }

        model = jsonencode({
          editorMode    = "code"
          expr          = rule.value.expr
          instant       = true
          queryType     = "instant"
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
            evaluator = { params = [rule.value.threshold], type = rule.value.op }
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
}
