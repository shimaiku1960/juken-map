# 利用者と EC2 の間に CloudFront を置き、利用者に見せる証明書を ACM のものにする（JUK-185）。
#
# 2026-10-13、会社の Wi-Fi から接続できなかった。それまでの証明書は Let's Encrypt の新しい
# チェーン（YE2 → Root YE → ISRG Root X2 → X1）で鍵は ECDSA。外から見る限り正常だったが、
# 古い検査機器はこの形を扱えないことがある。ACM の証明書（RSA）は Amazon Root CA につながり、
# その上は古くからある Starfield のルートが相互署名しているので、古い機器でも通りやすい。
# ACM の証明書は鍵を取り出せず EC2 の nginx には置けないので、CloudFront に付ける。
#
# 利用者 → CloudFront（ACM）→ HTTPS → origin.juken-map.com（EC2 の nginx、Let's Encrypt のまま）→ Go
#
# 費用は無料枠（毎月 1TB・1,000万リクエスト）に収まる見込み。

locals {
  site_domain = "juken-map.com"
  site_aliases = [
    local.site_domain,
    "www.${local.site_domain}", # www → apex の 301 は今までどおり nginx が返す
  ]
}

resource "aws_acm_certificate" "site" {
  provider = aws.us_east_1

  domain_name               = local.site_domain
  subject_alternative_names = ["www.${local.site_domain}"]
  validation_method         = "DNS"
  # 既定も RSA_2048 だが、古い機器に通すことがこの証明書の目的なので明示する。
  key_algorithm = "RSA_2048"

  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name = "juken-map"
  }
}

# DNS の確認用レコード（dns.tf）が引けるようになり、証明書が発行されるまで待つ。
resource "aws_acm_certificate_validation" "site" {
  provider = aws.us_east_1

  certificate_arn         = aws_acm_certificate.site.arn
  validation_record_fqdns = [for r in cloudflare_dns_record.acm_validation : r.name]
}

# AWS が用意している方針（managed policy）をそのまま使う。
data "aws_cloudfront_cache_policy" "disabled" {
  name = "Managed-CachingDisabled"
}

data "aws_cloudfront_cache_policy" "optimized" {
  name = "Managed-CachingOptimized"
}

# 利用者のヘッダー・Cookie・クエリをすべてオリジンへ渡す。Host も渡るので、nginx は今までどおり
# juken-map.com と www を見分けられ、CloudFront はオリジンの証明書を Host（juken-map.com）で検証する。
data "aws_cloudfront_origin_request_policy" "all_viewer" {
  name = "Managed-AllViewer"
}

resource "aws_cloudfront_distribution" "site" {
  enabled         = true
  comment         = "juken-map（JUK-185）"
  aliases         = local.site_aliases
  is_ipv6_enabled = true
  http_version    = "http2and3"
  # 日本の拠点を含む最小の区分。PriceClass_100 は北米と欧州だけで、日本からは遠い拠点を通る。
  price_class = "PriceClass_200"

  origin {
    origin_id   = "ec2"
    domain_name = cloudflare_dns_record.origin.name

    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "https-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  # 既定はキャッシュしない。API・ログイン・画面の HTML はそのままオリジンへ渡す。
  default_cache_behavior {
    target_origin_id       = "ec2"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods         = ["GET", "HEAD"]
    # 圧縮は Go が br/gzip で済ませている。
    compress                 = false
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer.id
  }

  # Vite が出す assets/* はファイル名にハッシュが入り、Go が immutable で返している
  # （apps/api/internal/spa/spa.go）ので、拠点にキャッシュしてよい。
  ordered_cache_behavior {
    path_pattern           = "/assets/*"
    target_origin_id       = "ec2"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    compress               = true
    cache_policy_id        = data.aws_cloudfront_cache_policy.optimized.id
    # ⚠️ ここにも Host を渡す。渡さないと CloudFront は origin.juken-map.com の名前でオリジンの
    # 証明書（juken-map.com と www）を検証し、一致せずに 502 になる（2026-10-13 に実際に起きた）。
    # キャッシュのキーは cache_policy が決めるので、ヘッダーを渡してもキャッシュは分かれない。
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer.id
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    acm_certificate_arn      = aws_acm_certificate_validation.site.certificate_arn
    ssl_support_method       = "sni-only"
    minimum_protocol_version = "TLSv1.2_2021"
  }

  tags = {
    Name = "juken-map"
  }
}

output "cloudfront_domain_name" {
  description = "DNS を切り替えずに CloudFront 経由を試すときの接続先（curl --connect-to juken-map.com:443:<これ>:443）"
  value       = aws_cloudfront_distribution.site.domain_name
}
