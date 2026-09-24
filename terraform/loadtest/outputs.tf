output "app_instance_id" {
  value = aws_instance.app.id
}

output "driver_instance_id" {
  value = aws_instance.driver.id
}

# 負荷をかける EC2 から叩く宛先（run-loadtest-limit.sh の BASE_URL）。
output "base_url" {
  value = "https://${aws_instance.app.private_ip}"
}

output "db_identifier" {
  value = aws_db_instance.db.identifier
}

output "results_bucket" {
  value = aws_s3_bucket.results.bucket
}
