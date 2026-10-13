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
