# 画面で作ってあったものを Terraform の管理下に取り込む（JUK-83）。
# 取り込んだ後の plan でも、すでに state にあるので何も起きない。
import {
  to = grafana_folder.juken_map
  id = "ffh5ls"
}

import {
  to = grafana_contact_point.email
  id = "juken-map-email"
}

import {
  to = grafana_rule_group.api_production
  id = "ffh5ls:api-production"
}
