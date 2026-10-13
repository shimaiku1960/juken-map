# Cloudflare の DNS のうち、CloudFront（cloudfront.tf）のために置くレコード（JUK-185）。
#
# ゾーンにはほかに Resend のメール（MX・DKIM・SPF）と Search Console の確認用 TXT があるが、
# terraform は書いたレコードしか触らないので、それらはここに書かない＝変わらない。
#
# どのレコードも proxied = false（グレー雲）にする。Cloudflare の中継は 2026-09-19 にやめており、
# 中継を挟むと CloudFront の手前にもう1段増えて、証明書も接続元 IP も Cloudflare のものになる。
# ttl = 1 は Cloudflare の「自動」（300秒）。

data "cloudflare_zone" "main" {
  filter = {
    name = "juken-map.com"
  }
}

# ACM が「このドメインの持ち主か」を確かめるための CNAME。更新のたびにも見に来るので、消さない。
resource "cloudflare_dns_record" "acm_validation" {
  for_each = {
    for o in aws_acm_certificate.site.domain_validation_options : o.domain_name => o
  }

  zone_id = data.cloudflare_zone.main.zone_id
  name    = trimsuffix(each.value.resource_record_name, ".")
  type    = each.value.resource_record_type
  content = trimsuffix(each.value.resource_record_value, ".")
  ttl     = 1
  proxied = false
  comment = "ACM の DNS 検証（terraform/cloudfront.tf、JUK-185）"
}

# CloudFront が転送する先（オリジン）。CloudFront にはドメイン名で渡す必要があるので、EC2 を
# 直接指す名前を1つ置く。利用者が開く名前ではない。
resource "cloudflare_dns_record" "origin" {
  zone_id = data.cloudflare_zone.main.zone_id
  name    = "origin.juken-map.com"
  type    = "A"
  content = aws_instance.app.public_ip
  ttl     = 1
  proxied = false
  comment = "CloudFront のオリジン＝EC2（terraform/dns.tf、JUK-185）"
}

# 利用者が開く名前（apex と www）は CloudFront を指す。2026-10-14 までは EC2 を直接指す A レコードを
# 手で置いていたので、import で取り込んでから CNAME に替えた。apex に CNAME は本来置けないが、
# Cloudflare は名前を辿って IP で答える（CNAME flattening）ので、CloudFront の IP が変わっても追いつく。
#
# 戻すときは type = "A"・content = aws_instance.app.public_ip にして apply する（nginx は EC2 で
# 今までどおり 443 を受けている）。
import {
  to = cloudflare_dns_record.site["juken-map.com"]
  id = "e05dbb4449d35edf9b44a421044003fb/b67a0c99a8c0e12f9d41d5f53d7a76fa"
}

import {
  to = cloudflare_dns_record.site["www.juken-map.com"]
  id = "e05dbb4449d35edf9b44a421044003fb/7af1e11c58676c39350d92627906b905"
}

resource "cloudflare_dns_record" "site" {
  for_each = toset(local.site_aliases)

  zone_id = data.cloudflare_zone.main.zone_id
  name    = each.value
  type    = "CNAME"
  content = aws_cloudfront_distribution.site.domain_name
  ttl     = 1
  proxied = false
  comment = "CloudFront（terraform/dns.tf、JUK-185）"
}
